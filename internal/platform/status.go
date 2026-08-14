package platform

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/task"
)

type platformTaskLister interface {
	Tasks() ([]*task.Task, error)
}

type Status struct {
	Time         time.Time      `json:"time"`
	Counts       map[string]int `json:"counts"`
	TaskStatuses map[string]int `json:"task_statuses,omitempty"`
	Healthy      bool           `json:"healthy"`
}

func (m *Manager) StatusSnapshot() (*Status, error) {
	counts := map[string]int{}
	if v, err := m.Agents(); err != nil { return nil, err } else { counts["agents"] = len(v) }
	if v, err := m.Sessions(); err != nil { return nil, err } else { counts["sessions"] = len(v) }
	if v, err := m.Schedules(); err != nil { return nil, err } else {
		counts["schedules"] = len(v)
		for _, x := range v { if x.Enabled { counts["schedules_enabled"]++ } }
	}
	if v, err := m.Workflows(); err != nil { return nil, err } else { counts["workflows"] = len(v) }
	if v, err := m.WorkflowRuns(); err != nil { return nil, err } else {
		counts["workflow_runs"] = len(v)
		for _, x := range v { if x.Status == "running" { counts["workflow_runs_running"]++ } }
	}
	if v, err := m.Nodes(); err != nil { return nil, err } else {
		counts["nodes"] = len(v)
		for _, x := range v { if x.Online { counts["nodes_online"]++ } }
	}
	if v, err := m.Plugins(); err != nil { return nil, err } else {
		counts["plugins"] = len(v)
		for _, x := range v { if x.Enabled { counts["plugins_enabled"]++ } }
	}
	if v, err := m.Channels(); err != nil { return nil, err } else {
		counts["channels"] = len(v)
		for _, x := range v { if x.Enabled { counts["channels_enabled"]++ } }
	}
	if v, err := m.Skills(); err != nil { return nil, err } else {
		counts["skills"] = len(v)
		for _, x := range v { if x.Enabled { counts["skills_enabled"]++ } }
	}
	if v, err := m.Missions(); err != nil { return nil, err } else {
		counts["missions"] = len(v)
		for _, x := range v { if x.Status == "running" { counts["missions_running"]++ } }
	}
	if v, err := m.Identities(); err == nil {
		counts["identities"] = len(v)
		for _, x := range v { if x.Enabled { counts["identities_enabled"]++ } }
	}
	if v, err := m.AccessKeys(); err == nil {
		counts["access_keys"] = len(v)
		n := now()
		for _, x := range v {
			if x.RevokedAt == nil && (x.ExpiresAt == nil || x.ExpiresAt.After(n)) { counts["access_keys_active"]++ }
		}
	}
	statuses := map[string]int{}
	if lister, ok := m.rt.(platformTaskLister); ok {
		if tasks, err := lister.Tasks(); err == nil {
			counts["runtime_tasks"] = len(tasks)
			for _, t := range tasks { statuses[string(t.Status)]++ }
		}
	}
	return &Status{Time: now(), Counts: counts, TaskStatuses: statuses, Healthy: true}, nil
}

func (m *Manager) StatusHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/platform/status", func(w http.ResponseWriter, _ *http.Request) {
		status, err := m.StatusSnapshot()
		respondPlatform(w, status, err)
	})
	mux.HandleFunc("GET /v1/platform/metrics", func(w http.ResponseWriter, _ *http.Request) {
		status, err := m.StatusSnapshot()
		if err != nil { platformProblem(w, err); return }
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		keys := make([]string, 0, len(status.Counts))
		for k := range status.Counts { keys = append(keys, k) }
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "kingaibot_platform_%s %d\n", metricName(k), status.Counts[k])
		}
		statusKeys := make([]string, 0, len(status.TaskStatuses))
		for k := range status.TaskStatuses { statusKeys = append(statusKeys, k) }
		sort.Strings(statusKeys)
		for _, k := range statusKeys {
			fmt.Fprintf(&b, "kingaibot_runtime_tasks{status=%q} %d\n", k, status.TaskStatuses[k])
		}
		_, _ = w.Write([]byte(b.String()))
	})
	return mux
}

func metricName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
