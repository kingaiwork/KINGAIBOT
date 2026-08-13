package device

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const (
	ScopeTasksRead       = "tasks:read"
	ScopeTasksCreate     = "tasks:create"
	ScopeTasksCancel     = "tasks:cancel"
	ScopeApprovalsRead   = "approvals:read"
	ScopeApprovalsDecide = "approvals:decide"
	ScopeEvolutionRead   = "evolution:read"
)

var allowedScopes = map[string]struct{}{
	ScopeTasksRead:       {},
	ScopeTasksCreate:     {},
	ScopeTasksCancel:     {},
	ScopeApprovalsRead:   {},
	ScopeApprovalsDecide: {},
	ScopeEvolutionRead:   {},
}

var DefaultControlScopes = []string{
	ScopeTasksRead,
	ScopeTasksCreate,
	ScopeTasksCancel,
	ScopeApprovalsRead,
	ScopeApprovalsDecide,
	ScopeEvolutionRead,
}

type Pairing struct {
	ID         string     `json:"id"`
	SecretHash string     `json:"secret_hash"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

type Device struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Platform  string     `json:"platform"`
	TokenHash string     `json:"token_hash,omitempty"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type persistedState struct {
	Pairings map[string]*Pairing `json:"pairings"`
	Devices  map[string]*Device  `json:"devices"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	data persistedState
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dir, "devices.json"),
		data: persistedState{Pairings: map[string]*Pairing{}, Devices: map[string]*Device{}},
	}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("load device identity store: %w", err)
	}
	if s.data.Pairings == nil {
		s.data.Pairings = map[string]*Pairing{}
	}
	if s.data.Devices == nil {
		s.data.Devices = map[string]*Device{}
	}
	return s, nil
}

func normalizeScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		scopes = DefaultControlScopes
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if _, ok := allowedScopes[scope]; !ok {
			return nil, fmt.Errorf("unsupported device scope %q", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out, nil
}

func randomSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("secure random generator unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func validateDeviceLabel(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 80 {
		return "", fmt.Errorf("%s must contain 1-80 characters", field)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%s contains control characters", field)
		}
	}
	return value, nil
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(s.path, b, 0o600)
}

func (s *Store) CreatePairing(scopes []string, ttl time.Duration) (*Pairing, string, error) {
	cleanScopes, err := normalizeScopes(scopes)
	if err != nil {
		return nil, "", err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if ttl > 15*time.Minute {
		return nil, "", errors.New("pairing lifetime cannot exceed 15 minutes")
	}
	id, err := storage.RandomID("pair")
	if err != nil {
		return nil, "", err
	}
	secret, err := randomSecret(32)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	p := &Pairing{
		ID:         id,
		SecretHash: hashSecret(secret),
		Scopes:     cleanScopes,
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prunePairingsLocked(now)
	s.data.Pairings[p.ID] = p
	if err := s.saveLocked(); err != nil {
		delete(s.data.Pairings, p.ID)
		return nil, "", err
	}
	return clonePairing(p), secret, nil
}

func (s *Store) ConsumePairing(id, secret, name, platform string) (*Device, string, error) {
	if err := storage.ValidateID(id); err != nil {
		return nil, "", errors.New("invalid pairing identifier")
	}
	if len(secret) < 32 || len(secret) > 256 {
		return nil, "", errors.New("invalid pairing secret")
	}
	name, err := validateDeviceLabel(name, "device name")
	if err != nil {
		return nil, "", err
	}
	platform, err = validateDeviceLabel(platform, "platform")
	if err != nil {
		return nil, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.data.Pairings[id]
	if !ok {
		return nil, "", errors.New("pairing not found")
	}
	now := time.Now().UTC()
	if p.ConsumedAt != nil {
		return nil, "", errors.New("pairing has already been consumed")
	}
	if !p.ExpiresAt.After(now) {
		return nil, "", errors.New("pairing has expired")
	}
	gotHash := hashSecret(secret)
	if subtle.ConstantTimeCompare([]byte(gotHash), []byte(p.SecretHash)) != 1 {
		return nil, "", errors.New("invalid pairing secret")
	}

	deviceID, err := storage.RandomID("dev")
	if err != nil {
		return nil, "", err
	}
	deviceToken, err := randomSecret(32)
	if err != nil {
		return nil, "", err
	}

	// Consume before issuing the credential. If persistence later fails the
	// pairing remains unusable and an administrator must create a new one.
	// This fail-safe ordering prevents crash-time replay of one-time pairings.
	p.ConsumedAt = &now
	if err := s.saveLocked(); err != nil {
		p.ConsumedAt = nil
		return nil, "", err
	}

	d := &Device{
		ID:        deviceID,
		Name:      name,
		Platform:  platform,
		TokenHash: hashSecret(deviceToken),
		Scopes:    append([]string(nil), p.Scopes...),
		CreatedAt: now,
	}
	s.data.Devices[d.ID] = d
	if err := s.saveLocked(); err != nil {
		delete(s.data.Devices, d.ID)
		return nil, "", err
	}
	return publicDevice(d), deviceToken, nil
}

func (s *Store) Authorize(token, scope string) (*Device, error) {
	if len(token) < 32 || len(token) > 256 {
		return nil, errors.New("invalid device credential")
	}
	if _, ok := allowedScopes[scope]; !ok {
		return nil, errors.New("invalid authorization scope")
	}
	gotHash := hashSecret(token)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.data.Devices {
		if d.RevokedAt != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(gotHash), []byte(d.TokenHash)) != 1 {
			continue
		}
		for _, granted := range d.Scopes {
			if granted == scope {
				return publicDevice(d), nil
			}
		}
		return nil, errors.New("device credential lacks required scope")
	}
	return nil, errors.New("invalid device credential")
}

func (s *Store) List() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Device, 0, len(s.data.Devices))
	for _, d := range s.data.Devices {
		out = append(out, publicDevice(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Store) Revoke(id string) error {
	if err := storage.ValidateID(id); err != nil {
		return errors.New("invalid device identifier")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data.Devices[id]
	if !ok {
		return os.ErrNotExist
	}
	if d.RevokedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	d.RevokedAt = &now
	return s.saveLocked()
}

func (s *Store) prunePairingsLocked(now time.Time) {
	for id, p := range s.data.Pairings {
		if p.ConsumedAt != nil && now.Sub(*p.ConsumedAt) > 24*time.Hour {
			delete(s.data.Pairings, id)
			continue
		}
		if p.ConsumedAt == nil && now.Sub(p.ExpiresAt) > 24*time.Hour {
			delete(s.data.Pairings, id)
		}
	}
}

func clonePairing(p *Pairing) *Pairing {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Scopes = append([]string(nil), p.Scopes...)
	return &cp
}

func publicDevice(d *Device) *Device {
	if d == nil {
		return nil
	}
	cp := *d
	cp.TokenHash = ""
	cp.Scopes = append([]string(nil), d.Scopes...)
	return &cp
}
