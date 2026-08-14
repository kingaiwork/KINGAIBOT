package authority

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxAuthorityRequestBytes = 256 << 10

type checkRequest struct {
	Capability string `json:"capability,omitempty"`
	DataScope  string `json:"data_scope,omitempty"`
	Tool       string `json:"tool,omitempty"`
}

func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := splitAuthorityPath(r.URL.Path)
		if len(parts) == 2 && parts[0] == "authority" && parts[1] == "envelopes" {
			s.handleAuthorityRoot(w, r)
			return
		}
		if len(parts) < 3 || parts[0] != "authority" || parts[1] != "envelopes" {
			http.NotFound(w, r)
			return
		}

		id := parts[2]
		if len(parts) == 3 {
			if r.Method != http.MethodGet {
				writeAuthorityError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			grant, err := s.Get(id)
			if err != nil {
				writeAuthorityStoreError(w, err)
				return
			}
			writeAuthorityJSON(w, http.StatusOK, grant)
			return
		}
		if len(parts) != 4 || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		var (
			grant *Grant
			err   error
		)
		switch parts[3] {
		case "derive":
			var child Envelope
			if err = decodeAuthorityJSON(r, &child); err == nil {
				grant, err = s.Derive(id, child)
			}
		case "revoke":
			grant, err = s.Revoke(id)
		case "check":
			var req checkRequest
			if err = decodeAuthorityJSON(r, &req); err == nil {
				err = s.Check(id, req.Capability, req.DataScope, req.Tool)
				if err == nil {
					writeAuthorityJSON(w, http.StatusOK, map[string]bool{"allowed": true})
					return
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeAuthorityStoreError(w, err)
			return
		}
		writeAuthorityJSON(w, http.StatusOK, grant)
	})
}

func (s *Store) handleAuthorityRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		grants, err := s.List()
		if err != nil {
			writeAuthorityStoreError(w, err)
			return
		}
		writeAuthorityJSON(w, http.StatusOK, map[string]any{"envelopes": grants})
	case http.MethodPost:
		var envelope Envelope
		if err := decodeAuthorityJSON(r, &envelope); err != nil {
			writeAuthorityError(w, http.StatusBadRequest, err.Error())
			return
		}
		grant, err := s.CreateRoot(envelope)
		if err != nil {
			writeAuthorityStoreError(w, err)
			return
		}
		writeAuthorityJSON(w, http.StatusCreated, grant)
	default:
		writeAuthorityError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func splitAuthorityPath(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if strings.HasPrefix(path, "v1/") {
		path = strings.TrimPrefix(path, "v1/")
	}
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func decodeAuthorityJSON(r *http.Request, dst any) error {
	if r == nil || r.Body == nil {
		return errors.New("request body required")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxAuthorityRequestBytes+1))
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

func writeAuthorityStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		writeAuthorityError(w, http.StatusNotFound, "authority envelope not found")
		return
	}
	message := err.Error()
	if strings.Contains(message, "requires") ||
		strings.Contains(message, "cannot") ||
		strings.Contains(message, "invalid") ||
		strings.Contains(message, "denied") ||
		strings.Contains(message, "exceed") ||
		strings.Contains(message, "expired") ||
		strings.Contains(message, "revoked") ||
		strings.Contains(message, "delegation") ||
		strings.Contains(message, "effective") {
		writeAuthorityError(w, http.StatusBadRequest, message)
		return
	}
	writeAuthorityError(w, http.StatusInternalServerError, "authority operation failed")
}

func writeAuthorityError(w http.ResponseWriter, status int, message string) {
	writeAuthorityJSON(w, status, map[string]string{"error": message})
}

func writeAuthorityJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
