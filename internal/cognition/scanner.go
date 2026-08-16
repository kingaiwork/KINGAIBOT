package cognition

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type TaskReader interface {
	List() ([]*task.Task, error)
}

type processedTaskState struct {
	Tasks map[string]string `json:"tasks"`
}

// AttachTasks starts a conservative learning loop over durable terminal task
// records. Learning is derived only from committed task state; it never replays
// tasks and never grants authority to the model or evolution subsystem.
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
		ticker := time.NewTicker(10 * time.Second)
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

func learnCommittedTasks(e *Engine, reader TaskReader, path string, state *processedTaskState) {
	tasks, err := reader.List()
	if err != nil {
		return
	}
	changed := false
	for _, t := range tasks {
		if t == nil || (t.Status != task.Completed && t.Status != task.Failed) {
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
	if !changed {
		return
	}
	// Keep the deduplication index bounded. task.Store.List returns newest first,
	// so retaining the newest 10k terminal tasks is sufficient for restart-safe
	// learning without allowing this sidecar to grow forever.
	if len(state.Tasks) > 20000 {
		keep := make(map[string]string, 10000)
		for _, t := range tasks {
			if len(keep) >= 10000 {
				break
			}
			if v, ok := state.Tasks[t.ID]; ok {
				keep[t.ID] = v
			}
		}
		state.Tasks = keep
	}
	if b, err := json.MarshalIndent(state, "", "  "); err == nil {
		_ = storage.AtomicWriteFile(path, b, 0o600)
	}
}
