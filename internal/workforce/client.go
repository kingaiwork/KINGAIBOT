package workforce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultControlPlane = "https://api.kingai.work"

var nodeTokenPattern = regexp.MustCompile(`^knode_[0-9a-fA-F]{64}$`)

type Settings struct {
	Enabled           bool
	ControlPlaneURL   string
	NodeToken         string
	AllowInsecureHTTP bool
	HeartbeatInterval time.Duration
	SyncInterval      time.Duration
	PollInterval      time.Duration
	RequestTimeout    time.Duration
	ReportOutput      bool
	MaxReportBytes    int
}

type Employee struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Title         string   `json:"title"`
	RoleKey       string   `json:"role_key"`
	Status        string   `json:"status"`
	AutonomyLevel string   `json:"autonomy_level"`
	RiskCeiling   string   `json:"risk_ceiling"`
	Skills        []string `json:"skills"`
	Goals         []string `json:"goals"`
}

type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	TriggerType string         `json:"trigger_type"`
	Status      string         `json:"status"`
	RiskLevel   string         `json:"risk_level"`
	Definition  map[string]any `json:"definition"`
}

type SyncResponse struct {
	OK        bool       `json:"ok"`
	Schema    string     `json:"schema"`
	Employees []Employee `json:"employees"`
	Workflows []Workflow `json:"workflows"`
	Policy    struct {
		CloudNeverBypassesLocalApproval bool   `json:"cloud_never_bypasses_local_approval"`
		ArbitraryShell                  bool   `json:"arbitrary_shell"`
		ExecutionBoundary               string `json:"execution_boundary"`
	} `json:"policy"`
}

type CloudTask struct {
	ID                string `json:"id"`
	OrganizationID    string `json:"organization_id"`
	EmployeeID        string `json:"employee_id"`
	Title             string `json:"title"`
	Instructions      string `json:"instructions"`
	Priority          string `json:"priority"`
	RiskLevel         string `json:"risk_level"`
	ActionFingerprint string `json:"action_fingerprint"`
	DueAt             string `json:"due_at"`
	CreatedAt         string `json:"created_at"`
	StartedAt         string `json:"started_at"`
}

type pullResponse struct {
	OK   bool       `json:"ok"`
	Task *CloudTask `json:"task"`
}

type Client struct {
	base    *url.URL
	token   string
	version string
	http    *http.Client
}

func SettingsFromEnv() (Settings, error) {
	token := strings.TrimSpace(os.Getenv("KINGAI_WORKFORCE_NODE_TOKEN"))
	if token == "" {
		return Settings{Enabled: false}, nil
	}
	if !nodeTokenPattern.MatchString(token) {
		return Settings{}, errors.New("KINGAI_WORKFORCE_NODE_TOKEN has invalid format")
	}
	base := strings.TrimSpace(os.Getenv("KINGAI_WORKFORCE_URL"))
	if base == "" {
		base = defaultControlPlane
	}
	s := Settings{
		Enabled:           true,
		ControlPlaneURL:   base,
		NodeToken:         token,
		AllowInsecureHTTP: envBool("KINGAI_WORKFORCE_ALLOW_INSECURE_HTTP"),
		HeartbeatInterval: envDuration("KINGAI_WORKFORCE_HEARTBEAT_SECONDS", 60*time.Second, 15*time.Second, 30*time.Minute),
		SyncInterval:      envDuration("KINGAI_WORKFORCE_SYNC_SECONDS", 120*time.Second, 30*time.Second, time.Hour),
		PollInterval:      envDuration("KINGAI_WORKFORCE_POLL_SECONDS", 8*time.Second, 2*time.Second, 5*time.Minute),
		RequestTimeout:    envDuration("KINGAI_WORKFORCE_REQUEST_TIMEOUT_SECONDS", 30*time.Second, 5*time.Second, 2*time.Minute),
		ReportOutput:      envBool("KINGAI_WORKFORCE_REPORT_OUTPUT"),
		MaxReportBytes:    envInt("KINGAI_WORKFORCE_MAX_REPORT_BYTES", 8192, 256, 65536),
	}
	if err := validateControlPlaneURL(s.ControlPlaneURL, s.AllowInsecureHTTP); err != nil {
		return Settings{}, fmt.Errorf("workforce control plane: %w", err)
	}
	return s, nil
}

func NewClient(settings Settings, version string) (*Client, error) {
	if !settings.Enabled {
		return nil, errors.New("workforce client is disabled")
	}
	if !nodeTokenPattern.MatchString(settings.NodeToken) {
		return nil, errors.New("invalid node token")
	}
	if err := validateControlPlaneURL(settings.ControlPlaneURL, settings.AllowInsecureHTTP); err != nil {
		return nil, err
	}
	base, _ := url.Parse(strings.TrimRight(settings.ControlPlaneURL, "/"))
	return &Client{
		base:    base,
		token:   settings.NodeToken,
		version: version,
		http: &http.Client{
			Timeout: settings.RequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) Heartbeat(ctx context.Context, capabilities []string) error {
	var out struct {
		OK bool `json:"ok"`
	}
	return c.doJSON(ctx, http.MethodPost, "/api/workforce/runtime/heartbeat", map[string]any{"version": c.version, "capabilities": capabilities}, &out)
}

func (c *Client) Sync(ctx context.Context) (*SyncResponse, error) {
	var out SyncResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/workforce/runtime/sync", nil, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, errors.New("workforce sync was not acknowledged")
	}
	if out.Policy.ArbitraryShell || !out.Policy.CloudNeverBypassesLocalApproval {
		return nil, errors.New("unsafe workforce cloud policy rejected")
	}
	return &out, nil
}

func (c *Client) PullTask(ctx context.Context) (*CloudTask, error) {
	var out pullResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/workforce/runtime/tasks/pull", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out.Task, nil
}

func (c *Client) ReportResult(ctx context.Context, taskID, status string, output any, errorText string) error {
	if taskID == "" {
		return errors.New("cloud task id required")
	}
	if status != "succeeded" && status != "failed" {
		return errors.New("invalid cloud task result status")
	}
	body := map[string]any{"status": status}
	if output != nil {
		body["output"] = output
	}
	if errorText != "" {
		body["error"] = errorText
	}
	var out struct {
		OK bool `json:"ok"`
	}
	return c.doJSON(ctx, http.MethodPost, "/api/workforce/runtime/tasks/"+url.PathEscape(taskID)+"/result", body, &out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, input any, output any) error {
	ref, err := url.Parse(path)
	if err != nil {
		return err
	}
	target := c.base.ResolveReference(ref)
	if !strings.EqualFold(target.Hostname(), c.base.Hostname()) || target.Scheme != c.base.Scheme {
		return errors.New("workforce request escaped configured control-plane origin")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if len(encoded) > 128<<10 {
			return errors.New("workforce request body exceeds limit")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT/"+c.version+" enterprise-workforce")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 1<<20+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(payload) > 1<<20 {
		return errors.New("workforce response exceeds limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("workforce API returned HTTP %d", resp.StatusCode)
	}
	if output != nil && len(payload) != 0 {
		if err := json.Unmarshal(payload, output); err != nil {
			return fmt.Errorf("invalid workforce response: %w", err)
		}
	}
	return nil
}

func validateControlPlaneURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("URL requires a hostname and must not contain credentials, query, or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme != "http" || !allowHTTP {
		return errors.New("control plane must use https")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("insecure http is allowed only for loopback development")
}

func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envDuration(name string, def, min, max time.Duration) time.Duration {
	seconds := envInt(name, int(def/time.Second), int(min/time.Second), int(max/time.Second))
	return time.Duration(seconds) * time.Second
}

func envInt(name string, def, min, max int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
