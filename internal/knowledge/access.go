package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
)

var reviewGate sync.Map // map[*Store]*sync.Mutex

func (s *Store) reviewLock() *sync.Mutex {
	v, _ := reviewGate.LoadOrStore(s, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ReadHandler exposes only operator-approved knowledge. Proposed/rejected items
// are deliberately invisible even when their IDs are known.
func (s *Store) ReadHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/knowledge/items", s.readApprovedItems)
	mux.HandleFunc("GET /v1/knowledge/items/{id}", s.readApprovedItem)
	mux.HandleFunc("GET /v1/knowledge/search", s.httpSearch)
	mux.HandleFunc("GET /v1/knowledge/neighbors", s.httpNeighbors)
	return mux
}

// AdminHandler owns proposal creation, inspection and review. It is intended to
// be mounted behind platform.admin rather than a general read/write scope.
func (s *Store) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/knowledge/admin/items", s.adminItems)
	mux.HandleFunc("POST /v1/knowledge/admin/items", s.adminItems)
	mux.HandleFunc("GET /v1/knowledge/admin/items/{id}", s.adminItem)
	mux.HandleFunc("POST /v1/knowledge/admin/items/{id}/review", s.adminReview)
	return mux
}

func (s *Store) readApprovedItems(w http.ResponseWriter, _ *http.Request) {
	v, err := s.List(false)
	write(w, v, err, http.StatusOK)
}

func (s *Store) readApprovedItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.Get(r.PathValue("id"))
	if err == nil && item.Status != "approved" {
		err = os.ErrNotExist
		item = nil
	}
	write(w, item, err, http.StatusOK)
}

func (s *Store) adminItems(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := s.List(true)
		write(w, v, err, http.StatusOK)
		return
	}
	var in struct {
		Item       Item   `json:"item"`
		Approved   bool   `json:"approved"`
		ReviewNote string `json:"review_note,omitempty"`
	}
	if err := strictDecode(w, r, &in); err != nil {
		return
	}
	var v *Item
	var err error
	if in.Approved {
		// Directly creating trusted knowledge is an administrative action and still
		// follows audit-before-trust semantics inside CreateApproved.
		v, err = s.CreateApproved(in.Item, in.ReviewNote)
	} else {
		v, err = s.CreateProposal(in.Item)
	}
	write(w, v, err, http.StatusCreated)
}

func (s *Store) adminItem(w http.ResponseWriter, r *http.Request) {
	v, err := s.Get(r.PathValue("id"))
	write(w, v, err, http.StatusOK)
}

func (s *Store) adminReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note,omitempty"`
	}
	if err := strictDecode(w, r, &in); err != nil {
		return
	}
	// Review() performs audit-before-trust. Serializing reviews here removes the
	// possibility that two simultaneous admin requests review the same proposal.
	gate := s.reviewLock()
	gate.Lock()
	v, err := s.Review(r.PathValue("id"), in.Decision, in.Note)
	gate.Unlock()
	write(w, v, err, http.StatusOK)
}

func strictDecode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		write(w, nil, fmt.Errorf("invalid JSON: %w", err), http.StatusBadRequest)
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		err := errors.New("only one JSON object is allowed")
		write(w, nil, err, http.StatusBadRequest)
		return err
	}
	return nil
}
