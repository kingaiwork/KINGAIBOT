package orchestration

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
)

func (b *Bridge) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/orchestration/bindings", b.httpBindings)
	mux.HandleFunc("POST /v1/orchestration/sync", b.httpSync)
	mux.HandleFunc("POST /v1/orchestration/workgraphs/{graph}/nodes/{node}/dispatch", b.httpDispatch)
	mux.HandleFunc("GET /v1/orchestration/workgraphs/{graph}/nodes/{node}/binding", b.httpBinding)
	return mux
}

func (b *Bridge) httpBindings(w http.ResponseWriter, _ *http.Request) {
	bindings, err := b.Bindings()
	orchestrationWrite(w, map[string]any{"bindings": bindings}, err, http.StatusOK)
}

func (b *Bridge) httpSync(w http.ResponseWriter, _ *http.Request) {
	err := b.Sync()
	orchestrationWrite(w, map[string]bool{"ok": err == nil}, err, http.StatusOK)
}

func (b *Bridge) httpDispatch(w http.ResponseWriter, r *http.Request) {
	binding, err := b.Dispatch(r.PathValue("graph"), r.PathValue("node"))
	orchestrationWrite(w, binding, err, http.StatusCreated)
}

func (b *Bridge) httpBinding(w http.ResponseWriter, r *http.Request) {
	binding, err := b.Binding(r.PathValue("graph"), r.PathValue("node"))
	orchestrationWrite(w, binding, err, http.StatusOK)
}

func orchestrationWrite(w http.ResponseWriter, value any, err error, success int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(success)
	_ = json.NewEncoder(w).Encode(value)
}
