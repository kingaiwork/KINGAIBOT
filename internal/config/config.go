package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Provider struct {
	Name                string `json:"name"`
	Type                string `json:"type"`
	BaseURL             string `json:"base_url"`
	APIKeyEnv           string `json:"api_key_env"`
	Model               string `json:"model"`
	Priority            int    `json:"priority"`
	Enabled             bool   `json:"enabled"`
	AllowInsecureHTTP   bool   `json:"allow_insecure_http,omitempty"`
	AllowPrivateNetwork bool   `json:"allow_private_network,omitempty"`
}

type Server struct {
	Listen         string   `json:"listen"`
	BaseURL        string   `json:"base_url"`
	AdminTokenEnv  string   `json:"admin_token_env"`
	MCPTokenEnv    string   `json:"mcp_token_env"`
	A2ATokenEnv    string   `json:"a2a_token_env"`
	AllowedOrigins []string `json:"allowed_origins"`
}

type Runtime struct {
	DataDir                    string `json:"data_dir"`
	WorkspaceDir               string `json:"workspace_dir"`
	MaxSteps                   int    `json:"max_steps"`
	WorkerCount                int    `json:"worker_count"`
	MaxRequestBytes            int64  `json:"max_request_bytes"`
	RequestTimeoutSeconds      int    `json:"request_timeout_seconds"`
	TaskTimeoutSeconds         int    `json:"task_timeout_seconds"`
	QueueCapacity              int    `json:"queue_capacity"`
	AuditVerifyIntervalSeconds int    `json:"audit_verify_interval_seconds"`
}

type Memory struct {
	Enabled          bool `json:"enabled"`
	MaxRecords       int  `json:"max_records"`
	MaxContextChars  int  `json:"max_context_chars"`
	StoreTaskInputs  bool `json:"store_task_inputs"`
	StoreTaskOutputs bool `json:"store_task_outputs"`
}

type Security struct {
	DefaultToolPolicy   string            `json:"default_tool_policy"`
	ToolPolicies        map[string]string `json:"tool_policies"`
	ShellAllowlist      []string          `json:"shell_allowlist"`
	HTTPAllowedHosts    []string          `json:"http_allowed_hosts"`
	AllowPrivateNetwork bool              `json:"allow_private_network"`
	FileReadRoots       []string          `json:"file_read_roots"`
	FileWriteRoots      []string          `json:"file_write_roots"`
}

type Evolution struct {
	Enabled              bool   `json:"enabled"`
	Mode                 string `json:"mode"`
	Repository           string `json:"repository"`
	AllowPrerelease      bool   `json:"allow_prerelease"`
	CheckIntervalMinutes int    `json:"check_interval_minutes"`
}

type RemoteEndpoint struct {
	Name                string `json:"name"`
	URL                 string `json:"url"`
	BearerTokenEnv      string `json:"bearer_token_env"`
	Enabled             bool   `json:"enabled"`
	AllowPrivateNetwork bool   `json:"allow_private_network,omitempty"`
	AllowInsecureHTTP   bool   `json:"allow_insecure_http,omitempty"`
}

type Protocols struct {
	MCP        bool             `json:"mcp"`
	A2A        bool             `json:"a2a"`
	MCPServers []RemoteEndpoint `json:"mcp_servers"`
	A2APeers   []RemoteEndpoint `json:"a2a_peers"`
}

type Config struct {
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Server    Server     `json:"server"`
	Runtime   Runtime    `json:"runtime"`
	Memory    Memory     `json:"memory"`
	Providers []Provider `json:"providers"`
	Security  Security   `json:"security"`
	Evolution Evolution  `json:"evolution"`
	Protocols Protocols  `json:"protocols"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := Config{Memory: Memory{Enabled: true, StoreTaskOutputs: true}}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if err := c.Normalize(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Normalize(base string) error {
	if c.Name == "" {
		c.Name = "KINGAIBOT"
	}
	if c.Server.Listen == "" {
		c.Server.Listen = "127.0.0.1:18888"
	}
	if c.Server.AdminTokenEnv == "" {
		c.Server.AdminTokenEnv = "KINGAGENT_ADMIN_TOKEN"
	}
	if c.Server.MCPTokenEnv == "" {
		c.Server.MCPTokenEnv = "KINGAGENT_MCP_TOKEN"
	}
	if c.Server.A2ATokenEnv == "" {
		c.Server.A2ATokenEnv = "KINGAGENT_A2A_TOKEN"
	}
	if c.Protocols.MCP && c.Server.MCPTokenEnv == c.Server.AdminTokenEnv {
		return errors.New("server.mcp_token_env must be distinct from admin_token_env")
	}
	if c.Protocols.A2A && c.Server.A2ATokenEnv == c.Server.AdminTokenEnv {
		return errors.New("server.a2a_token_env must be distinct from admin_token_env")
	}
	if c.Protocols.MCP && c.Protocols.A2A && c.Server.MCPTokenEnv == c.Server.A2ATokenEnv {
		return errors.New("server.mcp_token_env and a2a_token_env must be distinct")
	}
	if c.Server.BaseURL != "" {
		if err := validateServerBaseURL(c.Server.BaseURL); err != nil {
			return fmt.Errorf("server.base_url: %w", err)
		}
	}
	if c.Runtime.DataDir == "" {
		c.Runtime.DataDir = "./data"
	}
	if c.Runtime.WorkspaceDir == "" {
		c.Runtime.WorkspaceDir = filepath.Join(c.Runtime.DataDir, "workspace")
	}
	c.Runtime.DataDir = abs(base, c.Runtime.DataDir)
	c.Runtime.WorkspaceDir = abs(base, c.Runtime.WorkspaceDir)
	if c.Runtime.MaxSteps <= 0 { c.Runtime.MaxSteps = 12 }
	if c.Runtime.MaxSteps > 64 { c.Runtime.MaxSteps = 64 }
	if c.Runtime.WorkerCount <= 0 { c.Runtime.WorkerCount = 2 }
	if c.Runtime.WorkerCount > 32 { c.Runtime.WorkerCount = 32 }
	if c.Runtime.MaxRequestBytes <= 0 { c.Runtime.MaxRequestBytes = 1 << 20 }
	if c.Runtime.RequestTimeoutSeconds <= 0 { c.Runtime.RequestTimeoutSeconds = 120 }
	if c.Runtime.TaskTimeoutSeconds <= 0 { c.Runtime.TaskTimeoutSeconds = 600 }
	if c.Runtime.TaskTimeoutSeconds > 86400 { c.Runtime.TaskTimeoutSeconds = 86400 }
	if c.Runtime.QueueCapacity <= 0 { c.Runtime.QueueCapacity = 1024 }
	if c.Runtime.QueueCapacity > 100000 { c.Runtime.QueueCapacity = 100000 }
	if c.Runtime.AuditVerifyIntervalSeconds <= 0 { c.Runtime.AuditVerifyIntervalSeconds = 300 }
	if c.Runtime.AuditVerifyIntervalSeconds < 30 { c.Runtime.AuditVerifyIntervalSeconds = 30 }
	if c.Runtime.AuditVerifyIntervalSeconds > 86400 { c.Runtime.AuditVerifyIntervalSeconds = 86400 }
	if c.Memory.MaxRecords <= 0 { c.Memory.MaxRecords = 5000 }
	if c.Memory.MaxRecords > 100000 { c.Memory.MaxRecords = 100000 }
	if c.Memory.MaxContextChars <= 0 { c.Memory.MaxContextChars = 8000 }
	if c.Memory.MaxContextChars > 64000 { c.Memory.MaxContextChars = 64000 }
	if c.Security.DefaultToolPolicy == "" { c.Security.DefaultToolPolicy = "deny" }
	c.Security.DefaultToolPolicy = strings.ToLower(strings.TrimSpace(c.Security.DefaultToolPolicy))
	if !validPolicy(c.Security.DefaultToolPolicy) { return errors.New("security.default_tool_policy must be allow, ask, or deny") }
	if c.Security.ToolPolicies == nil { c.Security.ToolPolicies = map[string]string{} }
	for k, v := range c.Security.ToolPolicies {
		v = strings.ToLower(strings.TrimSpace(v))
		if !validPolicy(v) { return fmt.Errorf("invalid policy %q for tool %s", v, k) }
		c.Security.ToolPolicies[k] = v
	}
	if len(c.Security.FileReadRoots) == 0 { c.Security.FileReadRoots = []string{c.Runtime.WorkspaceDir} }
	if len(c.Security.FileWriteRoots) == 0 { c.Security.FileWriteRoots = []string{c.Runtime.WorkspaceDir} }
	for i := range c.Security.FileReadRoots { c.Security.FileReadRoots[i] = abs(base, c.Security.FileReadRoots[i]) }
	for i := range c.Security.FileWriteRoots { c.Security.FileWriteRoots[i] = abs(base, c.Security.FileWriteRoots[i]) }
	if c.Evolution.Mode == "" { c.Evolution.Mode = "proposal-only" }
	if c.Evolution.Mode != "proposal-only" { return errors.New("only evolution.mode=proposal-only is allowed in this release") }
	if c.Evolution.CheckIntervalMinutes <= 0 { c.Evolution.CheckIntervalMinutes = 360 }
	if err := os.MkdirAll(c.Runtime.DataDir, 0o700); err != nil { return err }
	if err := os.MkdirAll(c.Runtime.WorkspaceDir, 0o700); err != nil { return err }
	if len(c.Providers) == 0 { return errors.New("at least one provider must be configured") }
	sort.SliceStable(c.Providers, func(i, j int) bool { return c.Providers[i].Priority < c.Providers[j].Priority })
	enabled := 0
	for _, p := range c.Providers {
		if !p.Enabled { continue }
		enabled++
		if p.Name == "" || p.BaseURL == "" || p.Model == "" { return fmt.Errorf("enabled provider requires name, base_url and model") }
		if err := validateEndpointURL(p.BaseURL, p.AllowInsecureHTTP); err != nil { return fmt.Errorf("provider %s: %w", p.Name, err) }
	}
	if enabled == 0 { return errors.New("at least one provider must be enabled") }
	for _, ep := range append(append([]RemoteEndpoint{}, c.Protocols.MCPServers...), c.Protocols.A2APeers...) {
		if ep.Enabled {
			if ep.Name == "" || ep.URL == "" { return errors.New("enabled remote endpoint requires name and url") }
			if err := validateEndpointURL(ep.URL, ep.AllowInsecureHTTP); err != nil { return fmt.Errorf("remote endpoint %s: %w", ep.Name, err) }
		}
	}
	return nil
}
func validPolicy(s string) bool { return s == "allow" || s == "ask" || s == "deny" }
func validateEndpointURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw); if err != nil { return err }
	if u.Hostname() == "" { return errors.New("URL requires a hostname") }
	if u.User != nil { return errors.New("credentials in URL are not allowed") }
	if u.Scheme == "https" { return nil }
	if u.Scheme != "http" { return errors.New("only https is allowed (http requires explicit allow_insecure_http)") }
	if !allowHTTP { return errors.New("insecure http endpoint is disabled") }
	h := u.Hostname(); ip := net.ParseIP(h)
	if ip != nil && ip.IsLoopback() { return nil }
	if strings.EqualFold(h, "localhost") { return nil }
	return errors.New("insecure http is permitted only for loopback endpoints")
}
func validateServerBaseURL(raw string) error {
	u, err := url.Parse(raw); if err != nil { return err }
	if u.Hostname() == "" || u.User != nil { return errors.New("base URL requires a hostname and must not contain credentials") }
	if u.RawQuery != "" || u.Fragment != "" { return errors.New("base URL must not contain query or fragment") }
	if u.Scheme == "https" { return nil }
	if u.Scheme == "http" { h := u.Hostname(); if strings.EqualFold(h, "localhost") { return nil }; if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() { return nil } }
	return errors.New("public base URL must use https; http is allowed only for loopback")
}
func abs(base, p string) string { if filepath.IsAbs(p) { return filepath.Clean(p) }; a, err := filepath.Abs(filepath.Join(base, p)); if err != nil { return filepath.Clean(filepath.Join(base, p)) }; return a }
