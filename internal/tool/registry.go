package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

type ApprovalRequired struct{ ApprovalID string }

func (e *ApprovalRequired) Error() string { return "approval required: " + e.ApprovalID }

type Registry struct {
	cfg       *config.Config
	policy    *policy.Engine
	approvals *approval.Store
	events    *eventlog.Log
}

func New(cfg *config.Config, p *policy.Engine, a *approval.Store, events *eventlog.Log) *Registry {
	return &Registry{cfg: cfg, policy: p, approvals: a, events: events}
}

func (r *Registry) audit(eventType, taskID string, data map[string]any) error {
	if r.events == nil {
		return errors.New("audit log unavailable")
	}
	return r.events.Append(eventlog.Event{Type: eventType, TaskID: taskID, Data: data})
}

func (r *Registry) Definitions() []provider.ToolDef {
	return []provider.ToolDef{
		def("system_info", "Read safe local runtime information", map[string]any{"type": "object", "properties": map[string]any{}}),
		def("file_read", "Read a UTF-8 text file inside approved roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}),
		def("file_write", "Write a UTF-8 text file inside approved roots", map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}),
		def("http_get", "Perform an HTTPS GET to an approved host with DNS-rebinding/SSRF protection", map[string]any{"type": "object", "properties": map[string]any{"url": map[string]any{"type": "string"}}, "required": []string{"url"}}),
		def("shell_exec", "Execute an allow-listed binary directly without a shell", map[string]any{"type": "object", "properties": map[string]any{"argv": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 120}}, "required": []string{"argv"}}),
		def("mcp_tools_list", "List tools from an explicitly configured remote MCP server", map[string]any{"type": "object", "properties": map[string]any{"server": map[string]any{"type": "string"}}, "required": []string{"server"}}),
		def("mcp_tools_call", "Call a tool on an explicitly configured remote MCP server", map[string]any{"type": "object", "properties": map[string]any{"server": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object"}}, "required": []string{"server", "name", "arguments"}}),
		def("a2a_send", "Send a text message to an explicitly configured A2A 1.0 peer", map[string]any{"type": "object", "properties": map[string]any{"peer": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}}, "required": []string{"peer", "text"}}),
	}
}
func def(name, desc string, p map[string]any) provider.ToolDef {
	return provider.ToolDef{Type: "function", Function: provider.FunctionDef{Name: name, Description: desc, Parameters: p}}
}

func (r *Registry) Execute(ctx context.Context, taskID, name string, args json.RawMessage) (string, error) {
	if !json.Valid(args) {
		return "", errors.New("tool arguments must be valid JSON")
	}
	argHash := approval.CanonicalArgumentsHash(args)
	d := r.policy.Evaluate(name)
	if d == policy.Deny {
		if err := r.audit("tool.denied", taskID, map[string]any{"tool": name, "arguments_hash": argHash}); err != nil {
			return "", fmt.Errorf("tool denied and audit write failed: %w", err)
		}
		return "", fmt.Errorf("tool %s denied by policy", name)
	}
	if d != policy.Ask {
		if err := r.audit("tool.execution.requested", taskID, map[string]any{"tool": name, "arguments_hash": argHash, "decision": "allow"}); err != nil {
			return "", fmt.Errorf("audit unavailable; tool execution blocked: %w", err)
		}
		result, execErr := r.executeCore(ctx, name, args)
		eventType := "tool.execution.completed"
		data := map[string]any{"tool": name, "arguments_hash": argHash}
		if execErr != nil {
			eventType = "tool.execution.failed"
			data["error"] = memory.SanitizeContent(execErr.Error())
		}
		if auditErr := r.audit(eventType, taskID, data); auditErr != nil {
			return result, fmt.Errorf("tool outcome could not be audited; action may have executed and requires reconciliation: %w", auditErr)
		}
		return result, execErr
	}

	a, err := r.approvals.FindMatching(taskID, name, args)
	if errors.Is(err, os.ErrNotExist) {
		approvalID, idErr := storage.RandomID("appr")
		if idErr != nil {
			return "", idErr
		}
		a = &approval.Approval{ID: approvalID, TaskID: taskID, Tool: name, Capability: name, Arguments: args, ArgumentsHash: argHash, Status: "pending"}
		if err = r.approvals.Save(a); err != nil {
			return "", err
		}
		if err = r.audit("tool.approval.requested", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash}); err != nil {
			return "", fmt.Errorf("approval created but audit write failed; execution blocked: %w", err)
		}
		return "", &ApprovalRequired{ApprovalID: a.ID}
	}
	if err != nil {
		return "", err
	}
	switch a.Status {
	case "pending":
		return "", &ApprovalRequired{ApprovalID: a.ID}
	case "denied":
		return "", errors.New("approval denied for this exact action")
	case "approved":
	default:
		return "", errors.New("invalid approval state")
	}

	switch a.ExecutionState {
	case "completed":
		if err = r.audit("tool.execution.replayed", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "cached": true}); err != nil {
			return "", fmt.Errorf("audit unavailable; cached tool result withheld: %w", err)
		}
		return a.Result, nil
	case "failed":
		if err = r.audit("tool.execution.replayed", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "cached_failure": true}); err != nil {
			return "", fmt.Errorf("audit unavailable; cached tool failure withheld: %w", err)
		}
		return a.Result, fmt.Errorf("previous approved execution failed: %s", a.ExecutionError)
	}
	if err = r.audit("tool.execution.requested", taskID, map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash, "decision": "approved"}); err != nil {
		return "", fmt.Errorf("audit unavailable; approved tool execution blocked: %w", err)
	}
	a, err = r.approvals.BeginExecution(a.ID)
	if err != nil {
		return "", err
	}
	switch a.ExecutionState {
	case "completed":
		return a.Result, nil
	case "failed":
		return a.Result, fmt.Errorf("previous approved execution failed: %s", a.ExecutionError)
	case "executing":
	default:
		return "", errors.New("invalid approved execution state")
	}

	result, execErr := r.executeCore(ctx, name, args)
	if finishErr := r.approvals.FinishExecution(a.ID, result, execErr); finishErr != nil {
		return result, fmt.Errorf("tool outcome could not be durably recorded; action may have executed and requires reconciliation: %w", finishErr)
	}
	eventType := "tool.execution.completed"
	data := map[string]any{"approval_id": a.ID, "tool": name, "arguments_hash": argHash}
	if execErr != nil {
		eventType = "tool.execution.failed"
		data["error"] = memory.SanitizeContent(execErr.Error())
	}
	if auditErr := r.audit(eventType, taskID, data); auditErr != nil {
		return result, fmt.Errorf("tool outcome was durably recorded but audit append failed; action may have executed and requires reconciliation: %w", auditErr)
	}
	return result, execErr
}

func (r *Registry) executeCore(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "system_info":
		return r.systemInfo()
	case "file_read":
		return r.fileRead(args)
	case "file_write":
		return r.fileWrite(args)
	case "http_get":
		return r.httpGet(ctx, args)
	case "shell_exec":
		return r.shell(ctx, args)
	case "mcp_tools_list":
		return r.mcpList(ctx, args)
	case "mcp_tools_call":
		return r.mcpCall(ctx, args)
	case "a2a_send":
		return r.a2aSend(ctx, args)
	default:
		return "", errors.New("unknown tool")
	}
}

func (r *Registry) systemInfo() (string, error) {
	return fmt.Sprintf(`{"os":%q,"arch":%q,"go":%q,"workspace":%q}`, runtime.GOOS, runtime.GOARCH, runtime.Version(), r.cfg.Runtime.WorkspaceDir), nil
}

func (r *Registry) fileRead(raw json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	p, err := secureExisting(a.Path, r.cfg.Security.FileReadRoots)
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 2<<20 {
		return "", errors.New("file exceeds 2 MiB limit")
	}
	return string(b), nil
}

func (r *Registry) fileWrite(raw json.RawMessage) (string, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	if len(a.Content) > 2<<20 {
		return "", errors.New("content exceeds 2 MiB limit")
	}
	p, err := secureWritable(a.Path, r.cfg.Security.FileWriteRoots)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	p, err = secureWritable(p, r.cfg.Security.FileWriteRoots)
	if err != nil {
		return "", err
	}
	if err = storage.AtomicWriteFile(p, []byte(a.Content), 0o600); err != nil {
		return "", err
	}
	return `{"ok":true}`, nil
}

func (r *Registry) httpGet(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return "", err
	}
	if u.User != nil {
		return "", errors.New("credentials in URL are not allowed")
	}
	if u.Scheme != "https" {
		return "", errors.New("http_get requires https")
	}
	if u.Hostname() == "" {
		return "", errors.New("URL hostname required")
	}
	if !hostAllowed(u.Hostname(), r.cfg.Security.HTTPAllowedHosts) {
		return "", errors.New("host not in allowlist")
	}
	client := netguard.Client(30*time.Second, r.cfg.Security.AllowPrivateNetwork)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.User != nil {
			return errors.New("redirect URL credentials denied")
		}
		if !hostAllowed(req.URL.Hostname(), r.cfg.Security.HTTPAllowedHosts) {
			return errors.New("redirect host denied")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "KING-Agent-OS/1.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 2<<20 {
		return "", errors.New("HTTP response exceeds 2 MiB limit")
	}
	return fmt.Sprintf("status=%d\n%s", resp.StatusCode, string(b)), nil
}

func (r *Registry) shell(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Argv    []string `json:"argv"`
		Timeout int      `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	if len(a.Argv) == 0 {
		return "", errors.New("argv required")
	}
	if len(a.Argv) > 64 {
		return "", errors.New("too many argv items")
	}
	for _, arg := range a.Argv {
		if strings.IndexByte(arg, 0) >= 0 {
			return "", errors.New("NUL byte in argv denied")
		}
	}
	cmdName := filepath.Base(a.Argv[0])
	if a.Argv[0] != cmdName {
		return "", errors.New("shell executable must be an allow-listed bare command name; paths are denied")
	}
	ok := false
	for _, x := range r.cfg.Security.ShellAllowlist {
		if cmdName == x {
			ok = true
			break
		}
	}
	if !ok {
		return "", errors.New("binary not in shell allowlist")
	}
	if a.Timeout <= 0 {
		a.Timeout = 30
	}
	if a.Timeout > 120 {
		a.Timeout = 120
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, a.Argv[0], a.Argv[1:]...)
	cmd.Dir = r.cfg.Runtime.WorkspaceDir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + r.cfg.Runtime.WorkspaceDir, "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	out, err := cmd.CombinedOutput()
	if len(out) > 1<<20 {
		out = out[:1<<20]
	}
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}

func (r *Registry) endpoint(name string, list []config.RemoteEndpoint) (config.RemoteEndpoint, error) {
	for _, ep := range list {
		if ep.Enabled && ep.Name == name {
			return ep, nil
		}
	}
	return config.RemoteEndpoint{}, errors.New("remote endpoint is not configured or enabled")
}

func (r *Registry) remoteRPC(ctx context.Context, ep config.RemoteEndpoint, protocol, method, methodName string, params any) (string, error) {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": time.Now().UnixNano(), "method": method, "params": params})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	switch protocol {
	case "mcp":
		req.Header.Set("MCP-Protocol-Version", "2026-07-28")
		req.Header.Set("Mcp-Method", method)
		if methodName != "" {
			req.Header.Set("Mcp-Name", methodName)
		}
	case "a2a":
		req.Header.Set("A2A-Version", "1.0")
	}
	if ep.BearerTokenEnv != "" {
		token := os.Getenv(ep.BearerTokenEnv)
		if token == "" {
			return "", fmt.Errorf("missing bearer token env %s", ep.BearerTokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := netguard.Client(60*time.Second, ep.AllowPrivateNetwork)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 4<<20 {
		return "", errors.New("remote response exceeds 4 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("remote endpoint HTTP %d: %s", resp.StatusCode, string(b))
	}
	return string(b), nil
}

func (r *Registry) mcpList(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Server string `json:"server"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	ep, err := r.endpoint(a.Server, r.cfg.Protocols.MCPServers)
	if err != nil {
		return "", err
	}
	return r.remoteRPC(ctx, ep, "mcp", "tools/list", "", map[string]any{})
}
func (r *Registry) mcpCall(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Server    string         `json:"server"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	if a.Name == "" {
		return "", errors.New("remote tool name required")
	}
	ep, err := r.endpoint(a.Server, r.cfg.Protocols.MCPServers)
	if err != nil {
		return "", err
	}
	return r.remoteRPC(ctx, ep, "mcp", "tools/call", a.Name, map[string]any{"name": a.Name, "arguments": a.Arguments, "_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientInfo": map[string]any{"name": "KINGAIBOT", "version": r.cfg.Version}}})
}
func (r *Registry) a2aSend(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Peer string `json:"peer"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	if strings.TrimSpace(a.Text) == "" {
		return "", errors.New("text required")
	}
	ep, err := r.endpoint(a.Peer, r.cfg.Protocols.A2APeers)
	if err != nil {
		return "", err
	}
	messageID, idErr := storage.RandomID("msg")
	if idErr != nil {
		return "", idErr
	}
	params := map[string]any{"message": map[string]any{"messageId": messageID, "role": "ROLE_USER", "parts": []any{map[string]any{"text": a.Text}}}}
	return r.remoteRPC(ctx, ep, "a2a", "SendMessage", "", params)
}

func hostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(a), "."))
		if a == "*" {
			return true
		}
		if strings.HasPrefix(a, "*.") {
			suffix := strings.TrimPrefix(a, "*")
			if strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".") {
				return true
			}
		}
		if host == a {
			return true
		}
	}
	return false
}

func secureExisting(p string, roots []string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(a)
	if err != nil {
		return "", err
	}
	if !insideCanonical(real, roots) {
		return "", errors.New("path outside allowed roots")
	}
	return real, nil
}
func secureWritable(p string, roots []string) (string, error) {
	a, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	ancestor := filepath.Dir(a)
	for {
		realAncestor, er := filepath.EvalSymlinks(ancestor)
		if er == nil {
			rel, relErr := filepath.Rel(ancestor, a)
			if relErr != nil {
				return "", relErr
			}
			candidate := filepath.Clean(filepath.Join(realAncestor, rel))
			if !insideCanonical(candidate, roots) {
				return "", errors.New("path outside allowed roots after symlink resolution")
			}
			return candidate, nil
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return "", er
		}
		ancestor = next
	}
}
func insideCanonical(p string, roots []string) bool {
	for _, root := range roots {
		r := filepath.Clean(root)
		if real, err := filepath.EvalSymlinks(r); err == nil {
			r = real
		}
		if pathInside(p, r) {
			return true
		}
	}
	return false
}
func inside(p string, roots []string) bool {
	for _, r := range roots {
		if pathInside(p, filepath.Clean(r)) {
			return true
		}
	}
	return false
}
func pathInside(p, root string) bool {
	p = filepath.Clean(p)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func AllowedTools(cfg *config.Config) []string {
	var out []string
	for n, d := range cfg.Security.ToolPolicies {
		if strings.ToLower(d) != "deny" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
