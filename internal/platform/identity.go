package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const accessKeySecretBytes = 32

type Identity struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Roles       []string   `json:"roles,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
}

type AccessKey struct {
	ID         string     `json:"id"`
	IdentityID string     `json:"identity_id"`
	Prefix     string     `json:"prefix"`
	TokenHash  string     `json:"token_hash"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type IssuedAccessKey struct {
	AccessKey
	Token string `json:"token"`
}

func (m *Manager) ensureIdentityDirs() error {
	for _, name := range []string{"identities", "access-keys"} {
		if err := os.MkdirAll(filepath.Join(m.dir, name), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRole(role string) (string, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "viewer", "operator", "automation", "admin":
		return role, nil
	default:
		return "", fmt.Errorf("unsupported role %q", role)
	}
}

func rolePermissions(role string) []string {
	switch role {
	case "viewer":
		return []string{"platform.read"}
	case "operator":
		return []string{"platform.read", "platform.write"}
	case "automation":
		return []string{"platform.read", "platform.write", "platform.automation"}
	case "admin":
		return []string{"platform.*"}
	default:
		return nil
	}
}

func validPermission(p string) bool {
	switch p {
	case "platform.read", "platform.write", "platform.automation", "platform.admin", "platform.*":
		return true
	default:
		return false
	}
}

func effectivePermissions(id *Identity) map[string]struct{} {
	out := map[string]struct{}{}
	if id == nil {
		return out
	}
	for _, role := range id.Roles {
		for _, p := range rolePermissions(role) {
			out[p] = struct{}{}
		}
	}
	for _, p := range id.Permissions {
		out[p] = struct{}{}
	}
	return out
}

func permissionAllows(grants map[string]struct{}, need string) bool {
	if _, ok := grants["platform.*"]; ok {
		return true
	}
	_, ok := grants[need]
	return ok
}

// CreateIdentity persists an inert disabled identity first. The identity is
// enabled only after its creation audit is durable. Holding the Manager lock
// across the transition prevents a concurrent enable from racing creation.
func (m *Manager) CreateIdentity(in Identity) (*Identity, error) {
	if err := m.ensureIdentityDirs(); err != nil {
		return nil, err
	}
	name, err := cleanText(in.Name, maxNameLen, "name")
	if err != nil {
		return nil, err
	}
	seenRoles := map[string]struct{}{}
	roles := make([]string, 0, len(in.Roles))
	for _, r := range in.Roles {
		r, err = normalizeRole(r)
		if err != nil {
			return nil, err
		}
		if _, ok := seenRoles[r]; !ok {
			seenRoles[r] = struct{}{}
			roles = append(roles, r)
		}
	}
	seenPerms := map[string]struct{}{}
	perms := make([]string, 0, len(in.Permissions))
	for _, p := range in.Permissions {
		p = strings.ToLower(strings.TrimSpace(p))
		if !validPermission(p) {
			return nil, fmt.Errorf("unsupported permission %q", p)
		}
		if _, ok := seenPerms[p]; !ok {
			seenPerms[p] = struct{}{}
			perms = append(perms, p)
		}
	}
	if len(roles) == 0 && len(perms) == 0 {
		roles = []string{"viewer"}
	}
	sort.Strings(roles)
	sort.Strings(perms)
	id, err := storage.RandomID("ident")
	if err != nil {
		return nil, err
	}
	t := now()
	disabledAt := t
	in.ID, in.Name, in.Roles, in.Permissions, in.Enabled = id, name, roles, perms, false
	in.CreatedAt, in.UpdatedAt, in.DisabledAt = t, t, &disabledAt
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("identities", id, &in); err != nil {
		return nil, err
	}
	if err := m.audit("identity.created", map[string]any{"identity_id": id, "roles": roles, "permissions": perms}); err != nil {
		return nil, fmt.Errorf("identity remains disabled because creation audit failed: %w", err)
	}
	in.Enabled = true
	in.DisabledAt = nil
	in.UpdatedAt = now()
	if err := m.save("identities", id, &in); err != nil {
		return nil, fmt.Errorf("identity creation was audited but activation persistence failed: %w", err)
	}
	return &in, nil
}

func (m *Manager) Identity(id string) (*Identity, error) {
	if err := m.ensureIdentityDirs(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var v Identity
	if err := m.read("identities", id, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (m *Manager) Identities() ([]*Identity, error) {
	if err := m.ensureIdentityDirs(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out, err := listJSON[Identity](filepath.Join(m.dir, "identities"))
	if err == nil {
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	}
	return out, err
}

// SetIdentityEnabled is directional: enabling is audited before persistence;
// disabling is persisted first and never rolled back on audit failure.
func (m *Manager) SetIdentityEnabled(id string, enabled bool) (*Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var v Identity
	if err := m.read("identities", id, &v); err != nil {
		return nil, err
	}
	if v.Enabled == enabled {
		return &v, nil
	}
	if enabled {
		if err := m.audit("identity.enabled", map[string]any{"identity_id": id, "enabled": true}); err != nil {
			return nil, fmt.Errorf("identity remains disabled because enable audit failed: %w", err)
		}
		v.Enabled = true
		v.DisabledAt = nil
		v.UpdatedAt = now()
		if err := m.save("identities", id, &v); err != nil {
			return nil, fmt.Errorf("identity enable was audited but persistence failed: %w", err)
		}
		return &v, nil
	}
	v.Enabled = false
	t := now()
	v.DisabledAt = &t
	v.UpdatedAt = t
	if err := m.save("identities", id, &v); err != nil {
		return nil, err
	}
	if err := m.audit("identity.enabled", map[string]any{"identity_id": id, "enabled": false}); err != nil {
		return nil, fmt.Errorf("identity remains disabled but disable audit failed: %w", err)
	}
	return &v, nil
}

func generateAccessToken(keyID string) (token string, prefix string, digest string, err error) {
	secret := make([]byte, accessKeySecretBytes)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", errors.New("secure random generator unavailable")
	}
	encoded := hex.EncodeToString(secret)
	prefix = encoded[:12]
	token = "kai_" + keyID + "_" + encoded
	h := sha256.Sum256([]byte(token))
	return token, prefix, hex.EncodeToString(h[:]), nil
}

// IssueAccessKey stores the key initially revoked. The secret is returned only
// after the issuance audit is durable and the revocation marker is atomically
// cleared. A crash at any earlier point leaves an unusable credential.
func (m *Manager) IssueAccessKey(identityID string, ttlSeconds int64) (*IssuedAccessKey, error) {
	if err := m.ensureIdentityDirs(); err != nil {
		return nil, err
	}
	identity, err := m.Identity(identityID)
	if err != nil {
		return nil, err
	}
	if !identity.Enabled {
		return nil, errors.New("identity disabled")
	}
	if ttlSeconds < 0 || ttlSeconds > int64((365*24*time.Hour)/time.Second) {
		return nil, errors.New("ttl_seconds must be between 0 and 31536000")
	}
	keyID, err := storage.RandomID("key")
	if err != nil {
		return nil, err
	}
	token, prefix, digest, err := generateAccessToken(keyID)
	if err != nil {
		return nil, err
	}
	created := now()
	stagedAt := created
	key := AccessKey{ID: keyID, IdentityID: identityID, Prefix: prefix, TokenHash: digest, CreatedAt: created, RevokedAt: &stagedAt}
	if ttlSeconds > 0 {
		expires := created.Add(time.Duration(ttlSeconds) * time.Second)
		key.ExpiresAt = &expires
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save("access-keys", keyID, &key); err != nil {
		return nil, err
	}
	if err := m.audit("access_key.issued", map[string]any{"key_id": keyID, "identity_id": identityID, "expires_at": key.ExpiresAt}); err != nil {
		return nil, fmt.Errorf("access key remains revoked because issuance audit failed: %w", err)
	}
	key.RevokedAt = nil
	if err := m.save("access-keys", keyID, &key); err != nil {
		return nil, fmt.Errorf("access key issuance was audited but activation persistence failed: %w", err)
	}
	return &IssuedAccessKey{AccessKey: key, Token: token}, nil
}

func parseAccessKeyID(token string) (string, error) {
	parts := strings.Split(token, "_")
	if len(parts) != 4 || parts[0] != "kai" || parts[1] != "key" {
		return "", errors.New("invalid access token")
	}
	id := parts[1] + "_" + parts[2]
	if err := storage.ValidateID(id); err != nil {
		return "", errors.New("invalid access token")
	}
	return id, nil
}

func (m *Manager) AccessKeys() ([]*AccessKey, error) {
	if err := m.ensureIdentityDirs(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out, err := listJSON[AccessKey](filepath.Join(m.dir, "access-keys"))
	if err == nil {
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	}
	return out, err
}

func (m *Manager) RevokeAccessKey(id string) (*AccessKey, error) {
	m.mu.Lock()
	var k AccessKey
	if err := m.read("access-keys", id, &k); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if k.RevokedAt == nil {
		t := now()
		k.RevokedAt = &t
	}
	if err := m.save("access-keys", id, &k); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()
	if err := m.audit("access_key.revoked", map[string]any{"key_id": id, "identity_id": k.IdentityID}); err != nil {
		return nil, fmt.Errorf("access key revoked but audit append failed: %w", err)
	}
	return &k, nil
}

func (m *Manager) AuthenticateAccessToken(token, permission string) (*Identity, error) {
	keyID, err := parseAccessKeyID(token)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	var key AccessKey
	err = m.read("access-keys", keyID, &key)
	m.mu.RUnlock()
	if err != nil {
		return nil, errors.New("invalid access token")
	}
	if key.RevokedAt != nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(now())) {
		return nil, errors.New("access token expired or revoked")
	}
	h := sha256.Sum256([]byte(token))
	got := hex.EncodeToString(h[:])
	if len(got) != len(key.TokenHash) || subtle.ConstantTimeCompare([]byte(got), []byte(key.TokenHash)) != 1 {
		return nil, errors.New("invalid access token")
	}
	identity, err := m.Identity(key.IdentityID)
	if err != nil || !identity.Enabled {
		return nil, errors.New("identity disabled or unavailable")
	}
	if !permissionAllows(effectivePermissions(identity), permission) {
		return nil, errors.New("permission denied")
	}
	t := now()
	key.LastUsedAt = &t
	m.mu.Lock()
	_ = m.save("access-keys", key.ID, &key)
	m.mu.Unlock()
	return identity, nil
}

func requestPermission(r *http.Request) string {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return "platform.read"
	}
	return "platform.write"
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func constantTokenEqual(a, b string) bool {
	return len(a) == len(b) && len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (m *Manager) authWithPermission(adminTokenEnv, permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "bearer token required", http.StatusUnauthorized)
			return
		}
		admin := os.Getenv(adminTokenEnv)
		if len(admin) >= 32 && constantTokenEqual(token, admin) {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := m.AuthenticateAccessToken(token, permission); err != nil {
			http.Error(w, "valid scoped platform token required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ScopedAuthHandler accepts the existing environment admin token or a scoped
// durable platform access key. Existing deployments remain backward compatible.
func (m *Manager) ScopedAuthHandler(adminTokenEnv string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.authWithPermission(adminTokenEnv, requestPermission(r), next).ServeHTTP(w, r)
	})
}

// AdminAuthHandler protects identity/key lifecycle operations. Only the legacy
// environment admin token or an identity with platform.admin/platform.* may use it.
func (m *Manager) AdminAuthHandler(adminTokenEnv string, next http.Handler) http.Handler {
	return m.authWithPermission(adminTokenEnv, "platform.admin", next)
}
