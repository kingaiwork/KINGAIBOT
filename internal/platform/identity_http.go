package platform

import (
	"net/http"
)

func (m *Manager) IdentityHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/platform/identities", m.httpIdentities)
	mux.HandleFunc("POST /v1/platform/identities", m.httpIdentities)
	mux.HandleFunc("POST /v1/platform/identities/{id}/enabled", m.httpIdentityEnabled)
	mux.HandleFunc("GET /v1/platform/access-keys", m.httpAccessKeys)
	mux.HandleFunc("POST /v1/platform/access-keys", m.httpAccessKeys)
	mux.HandleFunc("POST /v1/platform/access-keys/{id}/revoke", m.httpAccessKeyRevoke)
	return mux
}

func (m *Manager) httpIdentities(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Identities()
		respondPlatform(w, v, err)
		return
	}
	var in Identity
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateIdentity(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpIdentityEnabled(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "enabled_required"})
		return
	}
	v, err := m.SetIdentityEnabled(r.PathValue("id"), *in.Enabled)
	respondPlatform(w, v, err)
}

type accessKeyView struct {
	ID         string     `json:"id"`
	IdentityID string     `json:"identity_id"`
	Prefix     string     `json:"prefix"`
	CreatedAt  any        `json:"created_at"`
	ExpiresAt  any        `json:"expires_at,omitempty"`
	RevokedAt  any        `json:"revoked_at,omitempty"`
	LastUsedAt any        `json:"last_used_at,omitempty"`
}

func viewAccessKey(k *AccessKey) map[string]any {
	out := map[string]any{
		"id":          k.ID,
		"identity_id": k.IdentityID,
		"prefix":      k.Prefix,
		"created_at":  k.CreatedAt,
	}
	if k.ExpiresAt != nil {
		out["expires_at"] = k.ExpiresAt
	}
	if k.RevokedAt != nil {
		out["revoked_at"] = k.RevokedAt
	}
	if k.LastUsedAt != nil {
		out["last_used_at"] = k.LastUsedAt
	}
	return out
}

func (m *Manager) httpAccessKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		keys, err := m.AccessKeys()
		if err != nil {
			platformProblem(w, err)
			return
		}
		out := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, viewAccessKey(k))
		}
		writePlatformJSON(w, http.StatusOK, map[string]any{"access_keys": out})
		return
	}
	var in struct {
		IdentityID string `json:"identity_id"`
		TTLSeconds int64  `json:"ttl_seconds,omitempty"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	issued, err := m.IssueAccessKey(in.IdentityID, in.TTLSeconds)
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusCreated, map[string]any{
		"id":          issued.ID,
		"identity_id": issued.IdentityID,
		"prefix":      issued.Prefix,
		"token":       issued.Token,
		"created_at":  issued.CreatedAt,
		"expires_at":  issued.ExpiresAt,
		"warning":     "This token is returned once. Store it securely; KINGAIBOT persists only its SHA-256 verifier.",
	})
}

func (m *Manager) httpAccessKeyRevoke(w http.ResponseWriter, r *http.Request) {
	v, err := m.RevokeAccessKey(r.PathValue("id"))
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusOK, viewAccessKey(v))
}
