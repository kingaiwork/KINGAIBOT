package cognition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

const (
	learningScanInterval  = 15 * time.Second
	learningScanBatch     = 2048
	learningCursorOverlap = 2 * time.Second
)

type TaskReader interface {
	List() ([]*task.Task, error)
}

type incrementalTaskReader interface {
	ListUpdatedSince(time.Time, int) ([]*task.Task, error)
}

type processedTaskState struct {
	Tasks  map[string]string `json:"tasks"`
	Cursor time.Time         `json:"cursor,omitempty"`
}

// AttachTasks starts a conservative learning loop over durable terminal task
// records. Learning is derived only from committed task state; it never replays
// tasks and never grants authority to the model or evolution subsystem.
//
// When the task store exposes ListUpdatedSince, the scanner uses a bounded
// incremental cursor with a small overlap. This keeps steady-state disk I/O
// proportional to recently changed tasks instead of total historical volume.
func (e *Engine) AttachTasks(reader TaskReader) error {
	if e == nil || !e.cfg.Enabled || reader == nil {
		return nil
	}
	path := filepath.Join(e.dir, "processed-tasks.json")
	state := processedTaskState{Tasks: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &state)
		if state.Tasks == nil {
			state.Tasks = map[string]string{}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// Reconcile one batch immediately at boot, then poll at a bounded cadence.
		learnCommittedTasks(e, reader, path, &state)
		ticker := time.NewTicker(learningScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-e.ctx.Done():
				return
			case <-ticker.C:
				learnCommittedTasks(e, reader, path, &state)
			}
		}
	}()
	return nil
}

func readLearningBatch(reader TaskReader, state *processedTaskState) ([]*task.Task, error) {
	if incremental, ok := reader.(incrementalTaskReader); ok {
		since := state.Cursor
		if !since.IsZero() {
			since = since.Add(-learningCursorOverlap)
		}
		return incremental.ListUpdatedSince(since, learningScanBatch)
	}
	return reader.List()
}

func learnCommittedTasks(e *Engine, reader TaskReader, path string, state *processedTaskState) {
	tasks, err := readLearningBatch(reader, state)
	if err != nil {
		return
	}
	changed := false
	cursorChanged := false
	maxUpdated := state.Cursor
	for _, t := range tasks {
		if t == nil {
			continue
		}
		if t.UpdatedAt.After(maxUpdated) {
			maxUpdated = t.UpdatedAt
			cursorChanged = true
		}
		if t.Status != task.Completed && t.Status != task.Failed {
			continue
		}
		fingerprint := string(t.Status) + ":" + t.UpdatedAt.UTC().Format(time.RFC3339Nano) + ":" + digest(t.Output+"\n"+t.Error)
		if state.Tasks[t.ID] == fingerprint {
			continue
		}
		var learnErr error
		if t.Status == task.Completed {
			learnErr = e.ObserveSuccess(t.ID, t.Input, t.Output, t.Provider)
		} else {
			learnErr = e.ObserveFailure(t.ID, t.Input, t.Provider, t.Error)
		}
		if learnErr != nil {
			continue
		}
		state.Tasks[t.ID] = fingerprint
		changed = true
	}
	if cursorChanged {
		state.Cursor = maxUpdated
	}
	if !changed && !cursorChanged {
		return
	}

	// Keep the deduplication index bounded. Incremental scanning only revisits a
	// short overlap window, so arbitrary eviction of old fingerprints is safe:
	// evicted ancient tasks are behind the persisted cursor and are not rescanned.
	if len(state.Tasks) > 20000 {
		remove := len(state.Tasks) - 10000
		for id := range state.Tasks {
			delete(state.Tasks, id)
			remove--
			if remove <= 0 {
				break
			}
		}
	}
	if b, err := json.MarshalIndent(state, "", "  "); err == nil {
		_ = storage.AtomicWriteFile(path, b, 0o600)
	}
}
