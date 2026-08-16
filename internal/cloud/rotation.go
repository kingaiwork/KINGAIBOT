package cloud

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

type RotationState struct {
	RotationID string    `json:"rotation_id,omitempty"`
	NewKeyID   string    `json:"new_key_id,omitempty"`
	Status     string    `json:"status"`
	PreparedAt time.Time `json:"prepared_at,omitempty"`
}

type rotationPrepareResponse struct {
	RotationID string `json:"rotation_id"`
	NewKeyID   string `json:"new_key_id"`
	Phase      string `json:"phase"`
}

type rotationCommitResponse struct {
	RotationID string `json:"rotation_id"`
	NewKeyID   string `json:"new_key_id"`
	Phase      string `json:"phase"`
	Idempotent bool   `json:"idempotent"`
}

func (m *Manager) rotationPath() string {
	return filepath.Join(m.dir, "key-rotation.json")
}

func (m *Manager) pendingKeyPath() string {
	return filepath.Join(m.dir, "device-ed25519.pending.pem")
}

func encodePrivateKey(priv ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parsePrivateKey(raw []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("invalid pending Ed25519 private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("pending device key is not Ed25519")
	}
	return priv, nil
}

func publicSPKIForKey(priv ed25519.PrivateKey) (string, error) {
	spki, err := x509.MarshalPKIXPublicKey(priv.Public())
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(spki), nil
}

func signWith(priv ed25519.PrivateKey, message string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))
}

func (m *Manager) readRotationState() (RotationState, error) {
	b, err := os.ReadFile(m.rotationPath())
	if err != nil {
		return RotationState{}, err
	}
	var state RotationState
	if err := json.Unmarshal(b, &state); err != nil {
		return RotationState{}, err
	}
	if state.RotationID == "" || state.NewKeyID == "" || state.Status != "prepared" {
		return RotationState{}, errors.New("invalid local key rotation state")
	}
	return state, nil
}

func (m *Manager) saveRotationState(state RotationState) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(m.rotationPath(), b, 0o600)
}

func (m *Manager) RotationStatus() RotationState {
	if m == nil || !m.cfg.Enabled {
		return RotationState{Status: "disabled"}
	}
	state, err := m.readRotationState()
	if errors.Is(err, os.ErrNotExist) {
		return RotationState{Status: "idle"}
	}
	if err != nil {
		return RotationState{Status: "reconciliation_required"}
	}
	return state
}

func (m *Manager) RotateKey(ctx context.Context) (RotationState, error) {
	if m == nil || !m.cfg.Enabled {
		return RotationState{}, errors.New("cloud device management is disabled")
	}
	m.rotationMu.Lock()
	defer m.rotationMu.Unlock()

	if existing, err := m.readRotationState(); err == nil {
		if err := m.commitRotationLocked(ctx, existing); err != nil {
			return existing, err
		}
		return RotationState{Status: "committed"}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return RotationState{}, err
	}
	_ = os.Remove(m.pendingKeyPath())

	m.mu.RLock()
	nodeID := m.state.NodeID
	currentKeyID := m.state.KeyID
	enrolled := m.state.Enrolled
	m.mu.RUnlock()
	if !enrolled || nodeID == "" || currentKeyID == "" {
		return RotationState{}, errors.New("cloud node is not enrolled")
	}

	_, pendingPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return RotationState{}, err
	}
	pendingPEM, err := encodePrivateKey(pendingPriv)
	if err != nil {
		return RotationState{}, err
	}
	if err := storage.AtomicWriteFile(m.pendingKeyPath(), pendingPEM, 0o600); err != nil {
		return RotationState{}, err
	}
	newPublic, err := publicSPKIForKey(pendingPriv)
	if err != nil {
		_ = os.Remove(m.pendingKeyPath())
		return RotationState{}, err
	}
	n, err := nonce()
	if err != nil {
		_ = os.Remove(m.pendingKeyPath())
		return RotationState{}, err
	}
	ts := time.Now().Unix()
	message := strings.Join([]string{"KINGAI-OPS-KEY-ROTATE-PREPARE-V1", nodeID, currentKeyID, fmt.Sprint(ts), n, newPublic}, "\n")
	payload := map[string]any{
		"node_id":                 nodeID,
		"current_key_id":          currentKeyID,
		"timestamp":               ts,
		"nonce":                   n,
		"new_public_key_spki_b64": newPublic,
		"old_signature_b64":       m.sign(message),
		"new_signature_b64":       signWith(pendingPriv, message),
	}
	var prepared rotationPrepareResponse
	if err := m.post(ctx, "/api/v1/ops/nodes/key-rotation/prepare", payload, "", &prepared); err != nil {
		_ = os.Remove(m.pendingKeyPath())
		return RotationState{}, err
	}
	if prepared.RotationID == "" || prepared.NewKeyID == "" || prepared.Phase != "prepared" {
		_ = os.Remove(m.pendingKeyPath())
		return RotationState{}, errors.New("cloud returned invalid key rotation prepare state")
	}
	state := RotationState{RotationID: prepared.RotationID, NewKeyID: prepared.NewKeyID, Status: "prepared", PreparedAt: time.Now().UTC()}
	if err := m.saveRotationState(state); err != nil {
		_ = os.Remove(m.pendingKeyPath())
		return RotationState{}, err
	}
	if err := m.commitRotationLocked(ctx, state); err != nil {
		return state, err
	}
	return RotationState{RotationID: state.RotationID, NewKeyID: state.NewKeyID, Status: "committed", PreparedAt: state.PreparedAt}, nil
}

func (m *Manager) RecoverKeyRotation(ctx context.Context) error {
	if m == nil || !m.cfg.Enabled {
		return nil
	}
	m.rotationMu.Lock()
	defer m.rotationMu.Unlock()
	state, err := m.readRotationState()
	if errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(m.pendingKeyPath())
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(m.pendingKeyPath()); errors.Is(err, os.ErrNotExist) {
		// A missing pending key means the server cannot have received a valid
		// commit from this node unless the active key was already replaced. In
		// either case the stale local marker is safe to discard.
		_ = os.Remove(m.rotationPath())
		return nil
	} else if err != nil {
		return err
	}
	return m.commitRotationLocked(ctx, state)
}

func (m *Manager) commitRotationLocked(ctx context.Context, state RotationState) error {
	pendingPEM, err := os.ReadFile(m.pendingKeyPath())
	if err != nil {
		return err
	}
	pendingPriv, err := parsePrivateKey(pendingPEM)
	if err != nil {
		return err
	}
	m.mu.RLock()
	nodeID := m.state.NodeID
	m.mu.RUnlock()
	if nodeID == "" {
		return errors.New("cloud node identity missing during key rotation")
	}
	n, err := nonce()
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	message := strings.Join([]string{"KINGAI-OPS-KEY-ROTATE-COMMIT-V1", nodeID, state.RotationID, fmt.Sprint(ts), n}, "\n")
	payload := map[string]any{"node_id": nodeID, "rotation_id": state.RotationID, "timestamp": ts, "nonce": n, "signature_b64": signWith(pendingPriv, message)}
	var committed rotationCommitResponse
	if err := m.post(ctx, "/api/v1/ops/nodes/key-rotation/commit", payload, "", &committed); err != nil {
		// Do not remove the pending key or rotation marker. A network error can be
		// ambiguous: the cloud may already have committed. The next local retry or
		// restart can safely repeat the commit because the server is idempotent.
		return err
	}
	if committed.Phase != "committed" || committed.NewKeyID != state.NewKeyID {
		return errors.New("cloud returned inconsistent key rotation commit state")
	}

	// Persist the active private key before switching in-memory signing. If the
	// process crashes after the cloud commit, rotation.json + pending PEM remain
	// and Bootstrap will idempotently finish this exact transition.
	if err := storage.AtomicWriteFile(m.keyPath, pendingPEM, 0o600); err != nil {
		return err
	}
	newPublic, err := publicSPKIForKey(pendingPriv)
	if err != nil {
		return err
	}
	m.keyMu.Lock()
	m.privateKey = pendingPriv
	m.publicSPKI = newPublic
	m.keyMu.Unlock()

	m.mu.Lock()
	m.state.KeyID = state.NewKeyID
	m.state.LastKeyRotationAt = time.Now().UTC()
	m.state.LastError = ""
	m.mu.Unlock()
	if err := m.saveState(); err != nil {
		return err
	}
	_ = os.Remove(m.pendingKeyPath())
	_ = os.Remove(m.rotationPath())
	return nil
}
