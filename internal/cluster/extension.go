package cluster

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kingaiwork/KINGAIBOT/internal/provider"
)

type workerSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities,omitempty"`
	Enabled      bool     `json:"enabled"`
}

type jobSummary struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	Priority             int      `json:"priority"`
	ReplayPolicy         string   `json:"replay_policy"`
	Status               string   `json:"status"`
	LeaseOwner           string   `json:"lease_owner,omitempty"`
	Attempts             int      `json:"attempts"`
	Error                string   `json:"error,omitempty"`
}

func (c *Coordinator) ToolDefinitions() []provider.ToolDef {
	return []provider.ToolDef{
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_workers_list", Description: "List registered remote workers and their declared capabilities. Worker secrets and metadata are never returned to the model.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_job_submit", Description: "Request a durable remote-worker job. In authority-enforced deployments, direct model submission is denied unless trusted runtime context binds an authority envelope; the model cannot choose or elevate its own authority.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string"}, "payload": map[string]any{}, "required_capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "priority": map[string]any{"type": "integer"}, "replay_policy": map[string]any{"type": "string", "enum": []string{"manual", "safe"}}}, "required": []string{"kind"}}}},
		{Type: "function", Function: provider.FunctionDef{Name: "cluster_jobs_list", Description: "List remote job lifecycle summaries without exposing raw payloads, lease tokens, result data or authority identifiers", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
	}
}

func (c *Coordinator) ExecuteTool(_ context.Context, _ string, name string, raw json.RawMessage) (string, error) {
	var v any
	var err error
	switch name {
	case "cluster_workers_list":
		workers, er := c.Workers()
		if er != nil {
			return "", er
		}
		out := make([]workerSummary, 0, len(workers))
		for _, w := range workers {
			out = append(out, workerSummary{ID: w.ID, Name: w.Name, Capabilities: w.Capabilities, Enabled: w.Enabled})
		}
		v = out
	case "cluster_jobs_list":
		jobs, er := c.Jobs()
		if er != nil {
			return "", er
		}
		out := make([]jobSummary, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, jobSummary{ID: j.ID, Kind: j.Kind, RequiredCapabilities: j.RequiredCapabilities, Priority: j.Priority, ReplayPolicy: j.ReplayPolicy, Status: j.Status, LeaseOwner: j.LeaseOwner, Attempts: j.Attempts, Error: j.Error})
		}
		v = out
	case "cluster_job_submit":
		var in struct {
			Kind                 string          `json:"kind"`
			Payload              json.RawMessage `json:"payload"`
			RequiredCapabilities []string        `json:"required_capabilities"`
			Priority             int             `json:"priority"`
			ReplayPolicy         string          `json:"replay_policy"`
		}
		if er := json.Unmarshal(raw, &in); er != nil {
			return "", er
		}
		v, err = c.SubmitAuthorized(Job{Kind: in.Kind, Payload: in.Payload, RequiredCapabilities: in.RequiredCapabilities, Priority: in.Priority, ReplayPolicy: in.ReplayPolicy}, "", nil, "")
	default:
		return "", errors.New("unknown cluster tool")
	}
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
