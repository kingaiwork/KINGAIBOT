package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/device"
	karuntime "github.com/kingaiwork/KINGAIBOT/internal/runtime"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
)

type Server struct {
	cfg     *config.Config
	rt      *karuntime.Runtime
	tools   *tool.Registry
	devices *device.Store
	limiter *limiter
}

func New(cfg *config.Config, rt *karuntime.Runtime, tools *tool.Registry, deviceStores ...*device.Store) *Server {
	var devices *device.Store
	if len(deviceStores) > 0 {
		devices = deviceStores[0]
	}
	return &Server{cfg: cfg, rt: rt, tools: tools, devices: devices, limiter: newLimiter(120, time.Minute)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /.well-known/agent-card.json", s.agentCard)
	mux.Handle("POST /a2a", s.authEnv(s.cfg.Server.A2ATokenEnv, http.HandlerFunc(s.a2a)))
	mux.Handle("POST /mcp", s.authEnv(s.cfg.Server.MCPTokenEnv, http.HandlerFunc(s.mcp)))
	admin := func(h http.Handler) http.Handler { return s.authEnv(s.cfg.Server.AdminTokenEnv, h) }
	control := func(scope string, h http.Handler) http.Handler { return s.authScoped(scope, h) }
	mux.Handle("POST /v1/device-pair", http.HandlerFunc(s.pairDevice))
	mux.Handle("POST /v1/devices/pairings", admin(http.HandlerFunc(s.createPairing)))
	mux.Handle("GET /v1/devices", admin(http.HandlerFunc(s.listDevices)))
	mux.Handle("POST /v1/devices/{id}/revoke", admin(http.HandlerFunc(s.revokeDevice)))
	mux.Handle("POST /v1/tasks", control(device.ScopeTasksCreate, http.HandlerFunc(s.createTask)))
	mux.Handle("GET /v1/tasks", control(device.ScopeTasksRead, http.HandlerFunc(s.listTasks)))
	mux.Handle("GET /v1/tasks/{id}", control(device.ScopeTasksRead, http.HandlerFunc(s.getTask)))
	mux.Handle("POST /v1/tasks/{id}/cancel", control(device.ScopeTasksCancel, http.HandlerFunc(s.cancelTask)))
	mux.Handle("GET /v1/approvals", control(device.ScopeApprovalsRead, http.HandlerFunc(s.listApprovals)))
	mux.Handle("POST /v1/approvals/{id}", control(device.ScopeApprovalsDecide, http.HandlerFunc(s.decideApproval)))
	mux.Handle("GET /v1/evolution/proposals", control(device.ScopeEvolutionRead, http.HandlerFunc(s.listEvolution)))
	return s.security(s.cors(s.limit(mux)))
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "name": s.cfg.Name, "version": s.cfg.Version, "time": time.Now().UTC()})
}
func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if s.rt != nil {
		if err := s.rt.Healthy(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "runtime_integrity"})
			return
		}
	}
	adminToken := os.Getenv(s.cfg.Server.AdminTokenEnv)
	if len(adminToken) < 32 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "auth_secret_strength"})
		return
	}
	protocolTokens := []string{adminToken}
	if s.cfg.Protocols.MCP {
		mcpToken := os.Getenv(s.cfg.Server.MCPTokenEnv)
		if len(mcpToken) < 32 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "auth_secret_strength"})
			return
		}
		protocolTokens = append(protocolTokens, mcpToken)
	}
	if s.cfg.Protocols.A2A {
		a2aToken := os.Getenv(s.cfg.Server.A2ATokenEnv)
		if len(a2aToken) < 32 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "auth_secret_strength"})
			return
		}
		protocolTokens = append(protocolTokens, a2aToken)
	}
	for i := range protocolTokens {
		for j := i + 1; j < len(protocolTokens); j++ {
			if subtle.ConstantTimeCompare([]byte(protocolTokens[i]), []byte(protocolTokens[j])) == 1 {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "auth_secret_reuse"})
				return
			}
		}
	}
	providerReady := false
	for _, p := range s.cfg.Providers {
		if !p.Enabled {
			continue
		}
		if p.APIKeyEnv == "" || os.Getenv(p.APIKeyEnv) != "" {
			providerReady = true
			break
		}
	}
	if !providerReady {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
func (s *Server) authEnv(envName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv(envName)
		if len(expected) < 32 {
			problem(w, 500, "server_misconfigured", "required authentication secret is missing or too short")
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			problem(w, 401, "unauthorized", "bearer token required")
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if len(got) != len(expected) || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			problem(w, 401, "unauthorized", "valid bearer token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authScoped(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv(s.cfg.Server.AdminTokenEnv)
		if len(expected) < 32 {
			problem(w, 500, "server_misconfigured", "required authentication secret is missing or too short")
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			problem(w, 401, "unauthorized", "bearer token required")
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if len(got) == len(expected) && subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		if s.devices != nil {
			if _, err := s.devices.Authorize(got, scope); err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}
		problem(w, http.StatusForbidden, "forbidden", "credential is invalid or lacks the required scope")
	})
}

func (s *Server) createPairing(w http.ResponseWriter, r *http.Request) {
	if s.devices == nil {
		problem(w, http.StatusServiceUnavailable, "device_identity_unavailable", "device identity store is unavailable")
		return
	}
	var in struct {
		Scopes           []string `json:"scopes"`
		ExpiresInSeconds int      `json:"expires_in_seconds"`
	}
	if decode(r, w, 64<<10, &in) != nil {
		return
	}
	if in.ExpiresInSeconds < 0 {
		problem(w, http.StatusBadRequest, "invalid_pairing_ttl", "expires_in_seconds cannot be negative")
		return
	}
	pairing, secret, err := s.devices.CreatePairing(in.Scopes, time.Duration(in.ExpiresInSeconds)*time.Second)
	if err != nil {
		problem(w, http.StatusBadRequest, "pairing_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"pairing_id":     pairing.ID,
		"pairing_secret": secret,
		"scopes":         pairing.Scopes,
		"expires_at":     pairing.ExpiresAt,
	})
}

func (s *Server) pairDevice(w http.ResponseWriter, r *http.Request) {
	if s.devices == nil {
		problem(w, http.StatusServiceUnavailable, "device_identity_unavailable", "device identity store is unavailable")
		return
	}
	var in struct {
		PairingID     string `json:"pairing_id"`
		PairingSecret string `json:"pairing_secret"`
		DeviceName    string `json:"device_name"`
		Platform      string `json:"platform"`
	}
	if decode(r, w, 64<<10, &in) != nil {
		return
	}
	d, token, err := s.devices.ConsumePairing(in.PairingID, in.PairingSecret, in.DeviceName, in.Platform)
	if err != nil {
		problem(w, http.StatusUnauthorized, "pairing_rejected", "pairing is invalid, expired, or already consumed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"device":       d,
		"device_token": token,
		"token_type":   "Bearer",
	})
}

func (s *Server) listDevices(w http.ResponseWriter, _ *http.Request) {
	if s.devices == nil {
		problem(w, http.StatusServiceUnavailable, "device_identity_unavailable", "device identity store is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": s.devices.List()})
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	if s.devices == nil {
		problem(w, http.StatusServiceUnavailable, "device_identity_unavailable", "device identity store is unavailable")
		return
	}
	if err := s.devices.Revoke(r.PathValue("id")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			problem(w, http.StatusNotFound, "not_found", "device not found")
			return
		}
		problem(w, http.StatusBadRequest, "revoke_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input    string         `json:"input"`
		Metadata map[string]any `json:"metadata"`
	}
	if decode(r, w, s.cfg.Runtime.MaxRequestBytes, &in) != nil {
		return
	}
	t, err := s.rt.Create(strings.TrimSpace(in.Input), in.Metadata)
	if err != nil {
		problem(w, 400, "invalid_task", err.Error())
		return
	}
	writeJSON(w, 202, t)
}
func (s *Server) listTasks(w http.ResponseWriter, _ *http.Request) {
	ts, err := s.rt.Tasks()
	if err != nil {
		problem(w, 500, "store_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tasks": ts})
}
func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.rt.Task(r.PathValue("id"))
	if err != nil {
		problem(w, 404, "not_found", "task not found")
		return
	}
	writeJSON(w, 200, t)
}
func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	if err := s.rt.Cancel(r.PathValue("id")); err != nil {
		problem(w, 400, "cancel_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) listEvolution(w http.ResponseWriter, _ *http.Request) {
	p, err := s.rt.Evolutions()
	if err != nil {
		problem(w, 500, "store_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"mode": s.cfg.Evolution.Mode, "enabled": s.cfg.Evolution.Enabled, "proposals": p})
}
func (s *Server) listApprovals(w http.ResponseWriter, _ *http.Request) {
	a, err := s.rt.Approvals()
	if err != nil {
		problem(w, 500, "store_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"approvals": a})
}
func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if decode(r, w, 64<<10, &in) != nil {
		return
	}
	if err := s.rt.DecideApproval(r.PathValue("id"), strings.ToLower(strings.TrimSpace(in.Status))); err != nil {
		problem(w, 400, "approval_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) agentCard(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Protocols.A2A {
		problem(w, 404, "disabled", "A2A disabled")
		return
	}
	base := strings.TrimRight(s.cfg.Server.BaseURL, "/")
	if base == "" {
		_, port, err := net.SplitHostPort(s.cfg.Server.Listen)
		if err != nil || port == "" {
			port = "18888"
		}
		base = "http://127.0.0.1:" + port
	}
	card := map[string]any{"name": s.cfg.Name, "description": "Secure, durable, model-agnostic autonomous agent runtime", "version": s.cfg.Version, "supportedInterfaces": []any{map[string]any{"url": base + "/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"}}, "capabilities": map[string]any{"streaming": false, "pushNotifications": false, "extendedAgentCard": false}, "securitySchemes": map[string]any{"bearer": map[string]any{"httpAuthSecurityScheme": map[string]any{"scheme": "Bearer", "bearerFormat": "opaque API token"}}}, "security": []any{map[string]any{"bearer": []string{}}}, "defaultInputModes": []string{"text/plain"}, "defaultOutputModes": []string{"text/plain"}, "skills": []any{map[string]any{"id": "general-agent", "name": "General secure agent", "description": "Plans and performs approved digital work with durable tasks and policy controls", "tags": []string{"agent", "automation", "tools", "memory"}, "examples": []string{"Summarize this task and save the result in the workspace."}}}}
	writeJSON(w, 200, card)
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func rpcOK(w http.ResponseWriter, id, result any) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func rpcErr(w http.ResponseWriter, id any, code int, msg string) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Protocols.MCP {
		rpcErr(w, nil, -32601, "MCP disabled")
		return
	}
	var q rpcReq
	if decode(r, w, s.cfg.Runtime.MaxRequestBytes, &q) != nil {
		return
	}
	if q.JSONRPC != "2.0" || q.Method == "" {
		rpcErr(w, q.ID, -32600, "invalid JSON-RPC request")
		return
	}
	version := r.Header.Get("MCP-Protocol-Version")
	if q.Method != "server/discover" && version != "2026-07-28" {
		rpcErr(w, q.ID, -32600, "MCP-Protocol-Version 2026-07-28 required")
		return
	}
	if h := r.Header.Get("Mcp-Method"); q.Method != "server/discover" && h != q.Method {
		rpcErr(w, q.ID, -32020, "Mcp-Method header mismatch")
		return
	}
	switch q.Method {
	case "server/discover":
		rpcOK(w, q.ID, map[string]any{"protocolVersion": "2026-07-28", "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]any{"name": s.cfg.Name, "version": s.cfg.Version}, "instructions": "KINGAIBOT tools are policy controlled; side effects can require local human approval."})
	case "tools/list":
		rpcOK(w, q.ID, map[string]any{"tools": s.tools.Definitions()})
	case "tools/call":
		var p struct {
			Name           string                     `json:"name"`
			Arguments      json.RawMessage            `json:"arguments"`
			InputResponses map[string]json.RawMessage `json:"inputResponses,omitempty"`
			RequestState   json.RawMessage            `json:"requestState,omitempty"`
		}
		if json.Unmarshal(q.Params, &p) != nil || p.Name == "" || !json.Valid(p.Arguments) {
			rpcErr(w, q.ID, -32602, "invalid params")
			return
		}
		if h := r.Header.Get("Mcp-Name"); h != "" && h != p.Name {
			rpcErr(w, q.ID, -32020, "Mcp-Name header mismatch")
			return
		}
		taskID := mcpActionID(r.Header.Get("Authorization"), p.Name, p.Arguments)
		res, err := s.tools.Execute(r.Context(), taskID, p.Name, p.Arguments)
		if err != nil {
			var ar *tool.ApprovalRequired
			if errors.As(err, &ar) {
				rpcOK(w, q.ID, map[string]any{"resultType": "input_required", "inputRequests": map[string]any{"approval": map[string]any{"method": "elicitation/create", "params": map[string]any{"mode": "form", "message": "Approval required for tool " + p.Name + ". Approve only after reviewing the exact arguments in the KINGAIBOT approval console.", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"approved": map[string]any{"type": "boolean"}}, "required": []string{"approved"}}}}}, "requestState": map[string]any{"approvalId": ar.ApprovalID, "actionId": taskID}})
				return
			}
			rpcOK(w, q.ID, map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true})
			return
		}
		rpcOK(w, q.ID, map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": res}}, "isError": false})
	default:
		rpcErr(w, q.ID, -32601, "method not found")
	}
}
func mcpActionID(auth, name string, args json.RawMessage) string {
	h := sha256.Sum256(append(append([]byte(auth+"\x00"+name+"\x00"), args...), 0))
	return "mcp_" + hex.EncodeToString(h[:16])
}

func (s *Server) a2a(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Protocols.A2A {
		rpcErr(w, nil, -32601, "A2A disabled")
		return
	}
	if r.Header.Get("A2A-Version") != "1.0" {
		rpcErr(w, nil, -32600, "A2A-Version 1.0 required")
		return
	}
	var q rpcReq
	if decode(r, w, s.cfg.Runtime.MaxRequestBytes, &q) != nil {
		return
	}
	if q.JSONRPC != "2.0" || q.Method == "" {
		rpcErr(w, q.ID, -32600, "invalid JSON-RPC request")
		return
	}
	switch q.Method {
	case "SendMessage":
		text, err := extractA2AText(q.Params)
		if err != nil {
			rpcErr(w, q.ID, -32602, err.Error())
			return
		}
		t, err := s.rt.Create(text, map[string]any{"source": "a2a"})
		if err != nil {
			rpcErr(w, q.ID, -32000, err.Error())
			return
		}
		rpcOK(w, q.ID, map[string]any{"task": a2aTask(t)})
	case "GetTask":
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(q.Params, &p) != nil || p.ID == "" {
			rpcErr(w, q.ID, -32602, "task id required")
			return
		}
		t, err := s.rt.Task(p.ID)
		if err != nil {
			rpcErr(w, q.ID, -32001, "task not found")
			return
		}
		rpcOK(w, q.ID, tA2AResponse(t))
	case "ListTasks":
		ts, err := s.rt.Tasks()
		if err != nil {
			rpcErr(w, q.ID, -32000, err.Error())
			return
		}
		items := make([]any, 0, len(ts))
		for _, t := range ts {
			items = append(items, a2aTask(t))
		}
		rpcOK(w, q.ID, map[string]any{"tasks": items})
	case "CancelTask":
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(q.Params, &p) != nil || p.ID == "" {
			rpcErr(w, q.ID, -32602, "task id required")
			return
		}
		if err := s.rt.Cancel(p.ID); err != nil {
			rpcErr(w, q.ID, -32000, err.Error())
			return
		}
		t, _ := s.rt.Task(p.ID)
		rpcOK(w, q.ID, tA2AResponse(t))
	default:
		rpcErr(w, q.ID, -32601, "method not found")
	}
}
func tA2AResponse(t *task.Task) map[string]any { return map[string]any{"task": a2aTask(t)} }
func extractA2AText(raw json.RawMessage) (string, error) {
	var p struct {
		Message struct {
			Parts []map[string]any `json:"parts"`
			Role  string           `json:"role"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	if p.Message.Role != "" && p.Message.Role != "ROLE_USER" {
		return "", errors.New("message role must be ROLE_USER")
	}
	var b strings.Builder
	for _, part := range p.Message.Parts {
		if t, ok := part["text"].(string); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t)
		}
	}
	if strings.TrimSpace(b.String()) == "" {
		return "", errors.New("text part required")
	}
	return b.String(), nil
}
func a2aTask(t *task.Task) any {
	if t == nil {
		return nil
	}
	state := "TASK_STATE_UNSPECIFIED"
	switch t.Status {
	case task.Queued:
		state = "TASK_STATE_SUBMITTED"
	case task.Running:
		state = "TASK_STATE_WORKING"
	case task.WaitingApproval:
		state = "TASK_STATE_INPUT_REQUIRED"
	case task.Completed:
		state = "TASK_STATE_COMPLETED"
	case task.Failed:
		state = "TASK_STATE_FAILED"
	case task.Canceled:
		state = "TASK_STATE_CANCELED"
	}
	artifacts := []any{}
	if t.Output != "" {
		artifacts = append(artifacts, map[string]any{"artifactId": "result", "name": "result", "parts": []any{map[string]any{"text": t.Output}}})
	}
	status := map[string]any{"state": state, "timestamp": t.UpdatedAt.Format(time.RFC3339Nano)}
	if t.Status == task.WaitingApproval {
		status["message"] = map[string]any{"messageId": "approval_" + t.ID, "role": "ROLE_AGENT", "taskId": t.ID, "contextId": t.ID, "parts": []any{map[string]any{"text": "Human approval is required before this task can continue."}}}
	}
	return map[string]any{"id": t.ID, "contextId": t.ID, "status": status, "artifacts": artifacts, "createdAt": t.CreatedAt.Format(time.RFC3339Nano), "lastModified": t.UpdatedAt.Format(time.RFC3339Nano)}
}
func decode(r *http.Request, w http.ResponseWriter, max int64, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, "invalid_json", err.Error())
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		problem(w, 400, "invalid_json", "multiple JSON values are not allowed")
		return errors.New("multiple json values")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg}})
}
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, s.cfg.Server.AllowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version, Mcp-Method, Mcp-Name, A2A-Version")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func originAllowed(o string, allowed []string) bool {
	for _, a := range allowed {
		if a == o {
			return true
		}
	}
	return false
}
func (s *Server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !s.limiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			problem(w, 429, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type bucket struct {
	start time.Time
	n     int
}
type limiter struct {
	mu        sync.Mutex
	max       int
	window    time.Duration
	m         map[string]bucket
	lastSweep time.Time
}

func newLimiter(max int, w time.Duration) *limiter {
	return &limiter{max: max, window: w, m: map[string]bucket{}}
}
func (l *limiter) Allow(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) > l.window {
		for key, b := range l.m {
			if now.Sub(b.start) > 2*l.window {
				delete(l.m, key)
			}
		}
		l.lastSweep = now
	}
	b := l.m[k]
	if b.start.IsZero() || now.Sub(b.start) > l.window {
		l.m[k] = bucket{start: now, n: 1}
		return true
	}
	if b.n >= l.max {
		return false
	}
	b.n++
	l.m[k] = b
	return true
}
