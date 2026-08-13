#!/usr/bin/env python3
from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"{label} patch target not found")
    return text.replace(old, new, 1)


p = Path("internal/api/server.go")
s = p.read_text()
import_anchor = '"github.com/kingaiwork/KINGAIBOT/internal/config"\n'
if "internal/device" not in s:
    s = s.replace(import_anchor, import_anchor + '\t"github.com/kingaiwork/KINGAIBOT/internal/device"\n', 1)

s = replace_once(
    s,
    '''type Server struct {
\tcfg     *config.Config
\trt      *karuntime.Runtime
\ttools   *tool.Registry
\tlimiter *limiter
}

func New(cfg *config.Config, rt *karuntime.Runtime, tools *tool.Registry) *Server {
\treturn &Server{cfg: cfg, rt: rt, tools: tools, limiter: newLimiter(120, time.Minute)}
}
''',
    '''type Server struct {
\tcfg     *config.Config
\trt      *karuntime.Runtime
\ttools   *tool.Registry
\tdevices *device.Store
\tlimiter *limiter
}

func New(cfg *config.Config, rt *karuntime.Runtime, tools *tool.Registry, deviceStores ...*device.Store) *Server {
\tvar devices *device.Store
\tif len(deviceStores) > 0 {
\t\tdevices = deviceStores[0]
\t}
\treturn &Server{cfg: cfg, rt: rt, tools: tools, devices: devices, limiter: newLimiter(120, time.Minute)}
}
''',
    "Server constructor",
)

s = replace_once(
    s,
    '''\tadmin := func(h http.Handler) http.Handler { return s.authEnv(s.cfg.Server.AdminTokenEnv, h) }
\tmux.Handle("POST /v1/tasks", admin(http.HandlerFunc(s.createTask)))
\tmux.Handle("GET /v1/tasks", admin(http.HandlerFunc(s.listTasks)))
\tmux.Handle("GET /v1/tasks/{id}", admin(http.HandlerFunc(s.getTask)))
\tmux.Handle("POST /v1/tasks/{id}/cancel", admin(http.HandlerFunc(s.cancelTask)))
\tmux.Handle("GET /v1/approvals", admin(http.HandlerFunc(s.listApprovals)))
\tmux.Handle("POST /v1/approvals/{id}", admin(http.HandlerFunc(s.decideApproval)))
\tmux.Handle("GET /v1/evolution/proposals", admin(http.HandlerFunc(s.listEvolution)))
''',
    '''\tadmin := func(h http.Handler) http.Handler { return s.authEnv(s.cfg.Server.AdminTokenEnv, h) }
\tcontrol := func(scope string, h http.Handler) http.Handler { return s.authScoped(scope, h) }
\tmux.Handle("POST /v1/device-pair", http.HandlerFunc(s.pairDevice))
\tmux.Handle("POST /v1/devices/pairings", admin(http.HandlerFunc(s.createPairing)))
\tmux.Handle("GET /v1/devices", admin(http.HandlerFunc(s.listDevices)))
\tmux.Handle("POST /v1/devices/{id}/revoke", admin(http.HandlerFunc(s.revokeDevice)))
\tmux.Handle("POST /v1/tasks", control(device.ScopeTasksCreate, http.HandlerFunc(s.createTask)))
\tmux.Handle("GET /v1/tasks", control(device.ScopeTasksRead, http.HandlerFunc(s.listTasks)))
\tmux.Handle("GET /v1/tasks/{id}", control(device.ScopeTasksRead, http.HandlerFunc(s.getTask)))
\tmux.Handle("POST /v1/tasks/{id}/cancel", control(device.ScopeTasksCancel, http.HandlerFunc(s.cancelTask)))
\tmux.Handle("GET /v1/approvals", control(device.ScopeApprovalsRead, http.HandlerFunc(s.listApprovals)))
\tmux.Handle("POST /v1/approvals/{id}", control(device.ScopeApprovalsDecide, http.HandlerFunc(s.decideApproval)))
\tmux.Handle("GET /v1/evolution/proposals", control(device.ScopeEvolutionRead, http.HandlerFunc(s.listEvolution)))
''',
    "API routes",
)

marker = "\nfunc (s *Server) createTask(w http.ResponseWriter, r *http.Request) {"
if marker not in s:
    raise SystemExit("createTask insertion marker not found")

insert = r'''

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
		"pairing_id": pairing.ID,
		"pairing_secret": secret,
		"scopes": pairing.Scopes,
		"expires_at": pairing.ExpiresAt,
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
		"device": d,
		"device_token": token,
		"token_type": "Bearer",
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
'''
s = s.replace(marker, insert + marker, 1)
p.write_text(s)

p = Path("cmd/kingagentd/main.go")
s = p.read_text()
if "internal/device" not in s:
    s = s.replace(
        '\t"github.com/kingaiwork/KINGAIBOT/internal/config"\n',
        '\t"github.com/kingaiwork/KINGAIBOT/internal/config"\n\t"github.com/kingaiwork/KINGAIBOT/internal/device"\n',
        1,
    )
s = s.replace('var version = "1.2.0"', 'var version = "1.3.0"', 1)
anchor = '''\tes, mustErr := evolution.New(filepath.Join(cfg.Runtime.DataDir, "evolution"))
\tmust(mustErr)
'''
replacement = anchor + '''\tds, mustErr := device.New(filepath.Join(cfg.Runtime.DataDir, "devices"))
\tmust(mustErr)
'''
s = replace_once(s, anchor, replacement, "device store startup")
s = replace_once(s, "Handler: api.New(cfg, rt, tr).Handler()", "Handler: api.New(cfg, rt, tr, ds).Handler()", "API constructor startup")
p.write_text(s)

p = Path("configs/config.example.json")
s = p.read_text().replace('"version": "1.2.0"', '"version": "1.3.0"', 1)
p.write_text(s)
