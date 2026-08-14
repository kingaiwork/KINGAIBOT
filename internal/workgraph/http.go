package workgraph

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxWorkGraphRequestBytes = 1 << 20

type createRequest struct {
	Objective string `json:"objective"`
	Nodes     []Node `json:"nodes,omitempty"`
}

type completeRequest struct {
	Outputs  map[string]any `json:"outputs,omitempty"`
	Evidence []Evidence     `json:"evidence,omitempty"`
}

type ambiguousRequest struct {
	Reason string `json:"reason"`
}

func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := splitWorkGraphPath(r.URL.Path)

		if len(parts) == 1 && parts[0] == "workgraphs" {
			s.handleRoot(w, r)
			return
		}
		if len(parts) < 2 || parts[0] != "workgraphs" {
			http.NotFound(w, r)
			return
		}

		graphID := parts[1]
		if len(parts) == 2 {
			if r.Method != http.MethodGet {
				writeWorkGraphError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			g, err := s.Get(graphID)
			if err != nil {
				writeWorkGraphStoreError(w, err)
				return
			}
			writeWorkGraphJSON(w, http.StatusOK, g)
			return
		}

		if len(parts) == 3 && parts[2] == "refresh" {
			if r.Method != http.MethodPost {
				writeWorkGraphError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			g, err := s.Refresh(graphID)
			if err != nil {
				writeWorkGraphStoreError(w, err)
				return
			}
			writeWorkGraphJSON(w, http.StatusOK, g)
			return
		}

		if len(parts) != 5 || parts[2] != "nodes" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		nodeID, action := parts[3], parts[4]
		var (
			g   *Graph
			err error
		)
		switch action {
		case "approve":
			g, err = s.Approve(graphID, nodeID)
		case "start":
			g, err = s.Start(graphID, nodeID)
		case "complete":
			var req completeRequest
			if err = decodeWorkGraphJSON(r, &req); err == nil {
				g, err = s.Complete(graphID, nodeID, req.Outputs, req.Evidence)
			}
		case "ambiguous":
			var req ambiguousRequest
			if err = decodeWorkGraphJSON(r, &req); err == nil {
				g, err = s.Ambiguous(graphID, nodeID, req.Reason)
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeWorkGraphStoreError(w, err)
			return
		}
		writeWorkGraphJSON(w, http.StatusOK, g)
	})
}

func (s *Store) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		graphs, err := s.List()
		if err != nil {
			writeWorkGraphStoreError(w, err)
			return
		}
		writeWorkGraphJSON(w, http.StatusOK, map[string]any{"workgraphs": graphs})
	case http.MethodPost:
		var req createRequest
		if err := decodeWorkGraphJSON(r, &req); err != nil {
			writeWorkGraphError(w, http.StatusBadRequest, err.Error())
			return
		}
		g, err := s.Create(req.Objective, req.Nodes)
		if err != nil {
			writeWorkGraphStoreError(w, err)
			return
		}
		writeWorkGraphJSON(w, http.StatusCreated, g)
	default:
		writeWorkGraphError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func splitWorkGraphPath(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func decodeWorkGraphJSON(r *http.Request, dst any) error {
	if r == nil || r.Body == nil {
		return errors.New("request body required")
	}
	reader := io.LimitReader(r.Body, maxWorkGraphRequestBytes+1)
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request must contain one JSON value")
		}
		return err
	}
	return nil
}

func writeWorkGraphStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		writeWorkGraphError(w, http.StatusNotFound, "workgraph not found")
		return
	}
	message := err.Error()
	if strings.Contains(message, "invalid identifier") ||
		strings.Contains(message, "required") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "not ready") ||
		strings.Contains(message, "not awaiting") ||
		strings.Contains(message, "not running") ||
		strings.Contains(message, "unknown work node") ||
		strings.Contains(message, "dependency") ||
		strings.Contains(message, "evidence") ||
		strings.Contains(message, "exceeds") {
		writeWorkGraphError(w, http.StatusBadRequest, message)
		return
	}
	writeWorkGraphError(w, http.StatusInternalServerError, "workgraph operation failed")
}

func writeWorkGraphError(w http.ResponseWriter, status int, message string) {
	writeWorkGraphJSON(w, status, map[string]string{"error": message})
}

func writeWorkGraphJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
