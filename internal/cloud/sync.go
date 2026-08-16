package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const syncStream = "memory-v1"

func syncKeyFromEnv(name string) ([]byte, string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, "", errors.New("memory sync key is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(raw)
	}
	if err != nil || len(key) != 32 {
		return nil, "", errors.New("memory sync key must be exactly 32 bytes encoded as base64")
	}
	h := sha256.Sum256(key)
	return key, hex.EncodeToString(h[:8]), nil
}

func encryptEnvelope(key, plaintext []byte, aad string) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, []byte(aad)), nil
}

func decryptEnvelope(key, nonce, ciphertext []byte, aad string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid sync nonce")
	}
	return gcm.Open(nil, nonce, ciphertext, []byte(aad))
}

func syncAAD(workspace, stream, keyID string) string {
	return "KINGAI-E2EE-SYNC-V1\n" + workspace + "\n" + stream + "\n" + keyID
}

func (m *Manager) SyncOnce(ctx context.Context) error {
	if m == nil || !m.cfg.Enabled || !m.cfg.SyncEnabled || m.export == nil {
		return nil
	}
	m.mu.RLock()
	state := m.state
	policy := m.policy
	m.mu.RUnlock()
	if !state.Enrolled || state.NodeID == "" || policy.DisableMemorySync {
		return nil
	}
	key, keyID, err := syncKeyFromEnv(m.cfg.SyncKeyEnv)
	if err != nil {
		m.setError(err)
		return err
	}
	plain, err := m.export(ctx)
	if err != nil {
		m.setError(err)
		return err
	}
	if len(plain) == 0 {
		return nil
	}
	if len(plain) > 1<<20 {
		return errors.New("memory sync snapshot exceeds 1 MiB local limit")
	}
	aad := syncAAD(state.WorkspaceID, syncStream, keyID)
	nonce, ciphertext, err := encryptEnvelope(key, plain, aad)
	if err != nil {
		return err
	}
	h := sha256.Sum256(ciphertext)
	m.mu.Lock()
	m.state.SyncSequence++
	sequence := m.state.SyncSequence
	m.mu.Unlock()
	ts := time.Now().Unix()
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	cipherB64 := base64.StdEncoding.EncodeToString(ciphertext)
	digest := hex.EncodeToString(h[:])
	message := strings.Join([]string{"KINGAI-OPS-SYNC-PUSH-V1", state.NodeID, fmt.Sprint(ts), fmt.Sprint(sequence), syncStream, keyID, digest}, "\n")
	payload := map[string]any{"node_id": state.NodeID, "timestamp": ts, "sequence": sequence, "stream": syncStream, "key_id": keyID, "nonce_b64": nonceB64, "ciphertext_b64": cipherB64, "ciphertext_sha256": digest, "signature_b64": m.sign(message)}
	if err := m.post(ctx, "/api/v1/ops/nodes/sync/push", payload, "", nil); err != nil {
		m.setError(err)
		_ = m.saveState()
		return err
	}
	if m.importer != nil {
		if err := m.pullAndImport(ctx, key, keyID, state); err != nil {
			m.setError(err)
			_ = m.saveState()
			return err
		}
	}
	m.mu.Lock()
	m.state.LastSyncAt = time.Now().UTC()
	m.state.LastError = ""
	m.mu.Unlock()
	return m.saveState()
}

func (m *Manager) pullAndImport(ctx context.Context, key []byte, keyID string, state State) error {
	n, err := nonce()
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	message := strings.Join([]string{"KINGAI-OPS-SYNC-PULL-V1", state.NodeID, fmt.Sprint(ts), n, syncStream, keyID}, "\n")
	payload := map[string]any{"node_id": state.NodeID, "timestamp": ts, "nonce": n, "stream": syncStream, "key_id": keyID, "signature_b64": m.sign(message)}
	var response struct {
		Items []struct {
			NodeID           string `json:"node_id"`
			KeyID            string `json:"key_id"`
			NonceB64         string `json:"nonce_b64"`
			CiphertextB64    string `json:"ciphertext_b64"`
			CiphertextSHA256 string `json:"ciphertext_sha256"`
		} `json:"items"`
	}
	if err := m.post(ctx, "/api/v1/ops/nodes/sync/pull", payload, "", &response); err != nil {
		return err
	}
	for _, item := range response.Items {
		if item.NodeID == state.NodeID || item.KeyID != keyID {
			continue
		}
		ciphertext, err := base64.StdEncoding.DecodeString(item.CiphertextB64)
		if err != nil {
			continue
		}
		h := sha256.Sum256(ciphertext)
		if !strings.EqualFold(hex.EncodeToString(h[:]), item.CiphertextSHA256) {
			continue
		}
		nonceRaw, err := base64.StdEncoding.DecodeString(item.NonceB64)
		if err != nil {
			continue
		}
		plain, err := decryptEnvelope(key, nonceRaw, ciphertext, syncAAD(state.WorkspaceID, syncStream, keyID))
		if err != nil {
			continue
		}
		if err := m.importer(ctx, plain); err != nil {
			return err
		}
	}
	return nil
}
