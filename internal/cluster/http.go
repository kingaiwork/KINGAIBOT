package cluster

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

func (c *Coordinator) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/cluster/workers", c.httpWorkers)
	mux.HandleFunc("POST /v1/cluster/workers", c.httpWorkers)
	mux.HandleFunc("POST /v1/cluster/workers/{id}/enabled", c.httpWorkerEnabled)
	mux.HandleFunc("GET /v1/cluster/jobs", c.httpJobs)
	mux.HandleFunc("POST /v1/cluster/jobs", c.httpJobs)
	mux.HandleFunc("POST /v1/cluster/jobs/{id}/reconcile", c.httpReconcile)
	return mux
}

func (c *Coordinator) WorkerHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/cluster/worker/heartbeat", c.httpHeartbeat)
	mux.HandleFunc("POST /v1/cluster/worker/lease", c.httpLease)
	mux.HandleFunc("POST /v1/cluster/worker/complete", c.httpComplete)
	return mux
}

func (c *Coordinator) httpWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := c.Workers()
		clusterWrite(w, v, err, http.StatusOK)
		return
	}
	var in struct {
		Name         string         `json:"name"`
		Capabilities []string       `json:"capabilities,omitempty"`
		Metadata     map[string]any `json:"metadata,omitempty"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	issued, err := c.RegisterWorker(in.Name, in.Capabilities, in.Metadata)
	if err != nil {
		clusterWrite(w, nil, err, http.StatusCreated)
		return
	}
	clusterJSON(w, http.StatusCreated, map[string]any{
		"id":           issued.ID,
		"name":         issued.Name,
		"capabilities": issued.Capabilities,
		"token_prefix": issued.TokenPrefix,
		"token":        issued.Token,
		"enabled":      issued.Enabled,
		"created_at":   issued.CreatedAt,
		"warning":      "Worker token is returned once. Store it securely; only its verifier hash is persisted.",
	})
}

func (c *Coordinator) httpWorkerEnabled(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		clusterJSON(w, http.StatusBadRequest, map[string]any{"error": "enabled_required"})
		return
	}
	v, err := c.SetWorkerEnabled(r.PathValue("id"), *in.Enabled)
	clusterWrite(w, v, err, http.StatusOK)
}

func (c *Coordinator) httpJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := c.Jobs()
		clusterWrite(w, v, err, http.StatusOK)
		return
	}
	var in AuthorizedJobRequest
	if !clusterDecode(w, r, &in) {
		return
	}
	v, err := c.SubmitAuthorized(in.Job, in.AuthorityID, in.RequiredDataScopes, in.RequiredTool)
	clusterWrite(w, v, err, http.StatusCreated)
}

func (c *Coordinator) httpReconcile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string          `json:"action"`
		Note   string          `json:"note,omitempty"`
		Result json.RawMessage `json:"result,omitempty"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	v, err := c.Reconcile(r.PathValue("id"), in.Action, in.Note, in.Result)
	clusterWrite(w, v, err, http.StatusOK)
}

func (c *Coordinator) workerFromRequest(r *http.Request) (*Worker, error) {
	token := bearer(r)
	if token == "" {
		return nil, errors.New("worker bearer token required")
	}
	return c.AuthenticateWorker(token)
}

func (c *Coordinator) httpHeartbeat(w http.ResponseWriter, r *http.Request) {
	worker, err := c.workerFromRequest(r)
	if err != nil {
		clusterJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var in struct {
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	if in.Metadata != nil {
		path, er := c.workerPath(worker.ID)
		if er != nil {
			clusterWrite(w, nil, er, http.StatusOK)
			return
		}
		c.mu.Lock()
		var current Worker
		if er = read(path, &current); er == nil {
			current.Metadata = in.Metadata
			current.LastSeenAt = time.Now().UTC()
			current.UpdatedAt = current.LastSeenAt
			er = save(path, &current)
		}
		c.mu.Unlock()
		if er != nil {
			clusterWrite(w, nil, er, http.StatusOK)
			return
		}
	}
	clusterJSON(w, http.StatusOK, map[string]any{"ok": true, "worker_id": worker.ID})
}

func (c *Coordinator) httpLease(w http.ResponseWriter, r *http.Request) {
	worker, err := c.workerFromRequest(r)
	if err != nil {
		clusterJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var in struct {
		LeaseSeconds int `json:"lease_seconds,omitempty"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	lease, err := c.LeaseJobAuthorized(worker, in.LeaseSeconds)
	if errors.Is(err, os.ErrNotExist) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	clusterWrite(w, lease, err, http.StatusOK)
}

func (c *Coordinator) httpComplete(w http.ResponseWriter, r *http.Request) {
	worker, err := c.workerFromRequest(r)
	if err != nil {
		clusterJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var in struct {
		JobID      string          `json:"job_id"`
		LeaseToken string          `json:"lease_token"`
		Result     json.RawMessage `json:"result,omitempty"`
		Error      string          `json:"error,omitempty"`
	}
	if !clusterDecode(w, r, &in) {
		return
	}
	v, err := c.CompleteAuthorized(worker, in.JobID, in.LeaseToken, in.Result, in.Error)
	clusterWrite(w, v, err, http.StatusOK)
}

func clusterDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxResultBytes+(128<<10))
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		clusterJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": err.Error()})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		clusterJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": "only one JSON object is allowed"})
		return false
	}
	return true
}

func clusterWrite(w http.ResponseWriter, v any, err error, success int) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		clusterJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	clusterJSON(w, success, v)
}

func clusterJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
