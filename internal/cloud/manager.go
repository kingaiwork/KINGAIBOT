package cloud

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const (
	defaultBaseURL = "https://api.kingai.work"
	maxCloudBody   = 2 << 20
)

type Config struct {
	Enabled             bool
	BaseURL             string
	EnrollmentTokenEnv  string
	Environment         string
	NodeClass           string
	Provider             string
	Region               string
	HeartbeatInterval   time.Duration
	SyncInterval        time.Duration
	SyncEnabled         bool
	SyncKeyEnv          string
	AllowCustomEndpoint bool
}

type Policy struct {
	Version               int               `json:"version"`
	DisabledProviders     []string          `json:"disabled_providers,omitempty"`
	DisabledChannels      []string          `json:"disabled_channels,omitempty"`
	MaxSteps              int               `json:"max_steps,omitempty"`
	MaxWorkerCount        int               `json:"max_worker_count,omitempty"`
	MaxTaskTimeoutSeconds int               `json:"max_task_timeout_seconds,omitempty"`
	DefaultToolPolicy     string            `json:"default_tool_policy,omitempty"`
	ToolPolicies          map[string]string `json:"tool_policies,omitempty"`
	DisableMemorySync     bool              `json:"disable_memory_sync,omitempty"`
}

type Telemetry struct {
	Status       string         `json:"status"`
	HealthScore  int            `json:"health_score,omitempty"`
	Uptime       time.Duration  `json:"-"`
	Extra        map[string]any `json:"extra,omitempty"`
	AgentVersion string         `json:"agent_version"`
}

type State struct {
	NodeID            string    `json:"node_id,omitempty"`
	KeyID             string    `json:"key_id,omitempty"`
	OrganizationID    string    `json:"organization_id,omitempty"`
	WorkspaceID       string    `json:"workspace_id,omitempty"`
	Sequence          int64     `json:"sequence"`
	SyncSequence      int64     `json:"sync_sequence"`
	PolicyVersion     int       `json:"policy_version"`
	LastHeartbeatAt   time.Time `json:"last_heartbeat_at,omitempty"`
	LastPolicyAt      time.Time `json:"last_policy_at,omitempty"`
	LastSyncAt        time.Time `json:"last_sync_at,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	Enrolled          bool      `json:"enrolled"`
	CloudEnabled      bool      `json:"cloud_enabled"`
	MemorySyncEnabled bool      `json:"memory_sync_enabled"`
}

type SnapshotProvider func(context.Context) ([]byte, error)
type ImportProvider func(context.Context, []byte) error
type TelemetryProvider func() Telemetry

type Manager struct {
	cfg        Config
	dir        string
	statePath  string
	keyPath    string
	client     *http.Client
	privateKey ed25519.PrivateKey
	publicSPKI string

	mu     sync.RWMutex
	state  State
	policy Policy
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	telemetry TelemetryProvider
	export    SnapshotProvider
	importer  ImportProvider
}

func New(dir string, cfg Config) (*Manager, error) {
	if !cfg.Enabled {
		return &Manager{cfg: cfg, state: State{CloudEnabled: false}}, nil
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.EnrollmentTokenEnv == "" {
		cfg.EnrollmentTokenEnv = "KINGAI_ENROLLMENT_TOKEN"
	}
	if cfg.SyncKeyEnv == "" {
		cfg.SyncKeyEnv = "KINGAI_SYNC_KEY"
	}
	if cfg.Environment == "" {
		cfg.Environment = "production"
	}
	if cfg.NodeClass == "" {
		cfg.NodeClass = "server"
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = time.Minute
	}
	if cfg.HeartbeatInterval < 30*time.Second {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 15 * time.Minute
	}
	if cfg.SyncInterval < time.Minute {
		cfg.SyncInterval = time.Minute
	}
	u, err := validateBaseURL(cfg.BaseURL, cfg.AllowCustomEndpoint)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{cfg: cfg, dir: dir, statePath: filepath.Join(dir, "state.json"), keyPath: filepath.Join(dir, "device-ed25519.pem"), ctx: ctx, cancel: cancel}
	m.client = netguard.Client(30*time.Second, false)
	m.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !strings.EqualFold(req.URL.Hostname(), u.Hostname()) || req.URL.Scheme != "https" {
			return errors.New("cloud redirect denied")
		}
		return nil
	}
	if err := m.loadState(); err != nil {
		cancel()
		return nil, err
	}
	if err := m.loadOrCreateKey(); err != nil {
		cancel()
		return nil, err
	}
	m.state.CloudEnabled = true
	m.state.MemorySyncEnabled = cfg.SyncEnabled
	if err := m.saveState(); err != nil {
		cancel()
		return nil, err
	}
	return m, nil
}

func validateBaseURL(raw string, allowCustom bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("cloud base_url must be an https origin without credentials, query, or fragment")
	}
	if !allowCustom && !strings.EqualFold(u.Hostname(), "api.kingai.work") {
		return nil, errors.New("cloud base_url must use api.kingai.work unless allow_custom_endpoint is explicitly enabled")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

func (m *Manager) loadState() error {
	b, err := os.ReadFile(m.statePath)
	if errors.Is(err, os.ErrNotExist) {
		m.state = State{CloudEnabled: true}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &m.state)
}

func (m *Manager) saveState() error {
	m.mu.RLock()
	b, err := json.MarshalIndent(m.state, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(m.statePath, b, 0o600)
}

func (m *Manager) loadOrCreateKey() error {
	b, err := os.ReadFile(m.keyPath)
	if err == nil {
		block, _ := pem.Decode(b)
		if block == nil || block.Type != "PRIVATE KEY" {
			return errors.New("invalid cloud device private key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		priv, ok := key.(ed25519.PrivateKey)
		if !ok {
			return errors.New("cloud device key is not Ed25519")
		}
		m.privateKey = priv
	} else if errors.Is(err, os.ErrNotExist) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		der, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return err
		}
		if err := storage.AtomicWriteFile(m.keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return err
		}
		m.privateKey = priv
	} else {
		return err
	}
	spki, err := x509.MarshalPKIXPublicKey(m.privateKey.Public())
	if err != nil {
		return err
	}
	m.publicSPKI = base64.StdEncoding.EncodeToString(spki)
	return nil
}

func nonce() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (m *Manager) sign(message string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(m.privateKey, []byte(message)))
}

func (m *Manager) endpoint(path string) string {
	return strings.TrimRight(m.cfg.BaseURL, "/") + path
}

func (m *Manager) post(ctx context.Context, path string, body any, bearer string, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint(path), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCloudBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxCloudBody {
		return errors.New("cloud response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var problem map[string]any
		_ = json.Unmarshal(raw, &problem)
		return fmt.Errorf("cloud status %d: %v", resp.StatusCode, problem["error"])
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Bootstrap(ctx context.Context, agentVersion string) (Policy, error) {
	if m == nil || !m.cfg.Enabled {
		return Policy{}, nil
	}
	m.mu.RLock()
	enrolled := m.state.Enrolled && m.state.NodeID != ""
	m.mu.RUnlock()
	if !enrolled {
		token := strings.TrimSpace(os.Getenv(m.cfg.EnrollmentTokenEnv))
		if token != "" {
			if err := m.enroll(ctx, token, agentVersion); err != nil {
				m.setError(err)
				return Policy{}, err
			}
		}
	}
	m.mu.RLock()
	enrolled = m.state.Enrolled && m.state.NodeID != ""
	m.mu.RUnlock()
	if enrolled {
		p, err := m.PullPolicy(ctx)
		if err != nil {
			m.setError(err)
			return Policy{}, err
		}
		return p, nil
	}
	return Policy{}, nil
}

func (m *Manager) enroll(ctx context.Context, token, agentVersion string) error {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "kingaibot"
	}
	n, err := nonce()
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	display := "KINGAIBOT@" + hostname
	message := strings.Join([]string{"KINGAI-OPS-ENROLL-V2", fmt.Sprint(ts), n, display, hostname, m.cfg.Environment, m.cfg.NodeClass, m.cfg.Provider, m.cfg.Region, runtime.GOOS, runtime.GOOS, runtime.GOARCH, agentVersion, m.publicSPKI}, "\n")
	payload := map[string]any{"timestamp": ts, "nonce": n, "display_name": display, "hostname": hostname, "environment": m.cfg.Environment, "node_class": m.cfg.NodeClass, "provider": m.cfg.Provider, "region": m.cfg.Region, "os_family": runtime.GOOS, "os_version": runtime.GOOS, "architecture": runtime.GOARCH, "agent_version": agentVersion, "public_key_spki_b64": m.publicSPKI, "signature_b64": m.sign(message)}
	var response struct {
		NodeID         string `json:"node_id"`
		KeyID          string `json:"key_id"`
		OrganizationID string `json:"organization_id"`
		WorkspaceID    string `json:"workspace_id"`
	}
	if err := m.post(ctx, "/api/v1/ops/nodes/enroll", payload, token, &response); err != nil {
		return err
	}
	if response.NodeID == "" || response.KeyID == "" {
		return errors.New("cloud enrollment response missing identity")
	}
	m.mu.Lock()
	m.state.NodeID = response.NodeID
	m.state.KeyID = response.KeyID
	m.state.OrganizationID = response.OrganizationID
	m.state.WorkspaceID = response.WorkspaceID
	m.state.Enrolled = true
	m.state.LastError = ""
	m.mu.Unlock()
	return m.saveState()
}

func (m *Manager) PullPolicy(ctx context.Context) (Policy, error) {
	if m == nil || !m.cfg.Enabled {
		return Policy{}, nil
	}
	m.mu.RLock()
	nodeID := m.state.NodeID
	m.mu.RUnlock()
	if nodeID == "" {
		return Policy{}, errors.New("cloud node is not enrolled")
	}
	n, err := nonce()
	if err != nil {
		return Policy{}, err
	}
	ts := time.Now().Unix()
	message := strings.Join([]string{"KINGAI-OPS-CLOUD-PULL-V1", nodeID, fmt.Sprint(ts), n}, "\n")
	payload := map[string]any{"node_id": nodeID, "timestamp": ts, "nonce": n, "signature_b64": m.sign(message)}
	var response struct {
		Policy Policy `json:"policy"`
	}
	if err := m.post(ctx, "/api/v1/ops/nodes/cloud/pull", payload, "", &response); err != nil {
		return Policy{}, err
	}
	m.mu.Lock()
	m.policy = response.Policy
	m.state.PolicyVersion = response.Policy.Version
	m.state.LastPolicyAt = time.Now().UTC()
	m.state.LastError = ""
	m.mu.Unlock()
	if err := m.saveState(); err != nil {
		return Policy{}, err
	}
	return response.Policy, nil
}

func (m *Manager) heartbeat(ctx context.Context, t Telemetry) error {
	m.mu.Lock()
	if !m.state.Enrolled || m.state.NodeID == "" {
		m.mu.Unlock()
		return nil
	}
	m.state.Sequence++
	sequence := m.state.Sequence
	nodeID := m.state.NodeID
	m.mu.Unlock()
	ts := time.Now().Unix()
	status := strings.ToLower(strings.TrimSpace(t.Status))
	if status != "healthy" && status != "warning" && status != "critical" {
		status = "unknown"
	}
	health := t.HealthScore
	if health < 0 || health > 100 {
		health = 0
	}
	message := strings.Join([]string{"KINGAI-OPS-HEARTBEAT-V1", nodeID, fmt.Sprint(ts), fmt.Sprint(sequence), status, fmt.Sprint(health), "", "", "", "", "", "", fmt.Sprint(int64(t.Uptime.Seconds())), t.AgentVersion}, "\n")
	payload := map[string]any{"node_id": nodeID, "timestamp": ts, "sequence": sequence, "status": status, "health_score": health, "uptime_seconds": int64(t.Uptime.Seconds()), "agent_version": t.AgentVersion, "signature_b64": m.sign(message)}
	if err := m.post(ctx, "/api/v1/ops/nodes/heartbeat", payload, "", nil); err != nil {
		m.setError(err)
		_ = m.saveState()
		return err
	}
	m.mu.Lock()
	m.state.LastHeartbeatAt = time.Now().UTC()
	m.state.LastError = ""
	m.mu.Unlock()
	return m.saveState()
}

func (m *Manager) Start(telemetry TelemetryProvider, export SnapshotProvider, importer ImportProvider) {
	if m == nil || !m.cfg.Enabled {
		return
	}
	m.telemetry, m.export, m.importer = telemetry, export, importer
	m.wg.Add(1)
	go m.loop()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	heartbeat := time.NewTicker(m.cfg.HeartbeatInterval)
	defer heartbeat.Stop()
	syncTicker := time.NewTicker(m.cfg.SyncInterval)
	defer syncTicker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-heartbeat.C:
			if m.telemetry != nil {
				ctx, cancel := context.WithTimeout(m.ctx, 25*time.Second)
				_ = m.heartbeat(ctx, m.telemetry())
				_, _ = m.PullPolicy(ctx)
				cancel()
			}
		case <-syncTicker.C:
			ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
			_ = m.SyncOnce(ctx)
			cancel()
		}
	}
}

func (m *Manager) Close() {
	if m == nil || m.cancel == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.state.LastError = ""
		return
	}
	s := strings.TrimSpace(err.Error())
	if len(s) > 512 {
		s = s[:512]
	}
	m.state.LastError = s
}

func (m *Manager) Snapshot() State {
	if m == nil {
		return State{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) Policy() Policy {
	if m == nil {
		return Policy{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policy
}
