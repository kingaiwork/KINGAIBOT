package platform

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const maxAPIRequest = 1 << 20

func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/platform/capabilities", m.httpCapabilities)
	mux.HandleFunc("GET /v1/platform/agents", m.httpAgents)
	mux.HandleFunc("POST /v1/platform/agents", m.httpAgents)
	mux.HandleFunc("POST /v1/platform/agents/{id}/enabled", m.httpAgentEnabled)
	mux.HandleFunc("GET /v1/platform/sessions", m.httpSessions)
	mux.HandleFunc("POST /v1/platform/sessions", m.httpSessions)
	mux.HandleFunc("GET /v1/platform/sessions/{id}", m.httpSession)
	mux.HandleFunc("POST /v1/platform/sessions/{id}/messages", m.httpSessionMessage)
	mux.HandleFunc("GET /v1/platform/schedules", m.httpSchedules)
	mux.HandleFunc("POST /v1/platform/schedules", m.httpSchedules)
	mux.HandleFunc("POST /v1/platform/schedules/{id}/enabled", m.httpScheduleEnabled)
	mux.HandleFunc("GET /v1/platform/workflows", m.httpWorkflows)
	mux.HandleFunc("POST /v1/platform/workflows", m.httpWorkflows)
	mux.HandleFunc("POST /v1/platform/workflows/{id}/enabled", m.httpWorkflowEnabled)
	mux.HandleFunc("POST /v1/platform/workflows/{id}/run", m.httpWorkflowRun)
	mux.HandleFunc("GET /v1/platform/workflow-runs", m.httpWorkflowRuns)
	mux.HandleFunc("GET /v1/platform/nodes", m.httpNodes)
	mux.HandleFunc("POST /v1/platform/nodes", m.httpNodes)
	mux.HandleFunc("POST /v1/platform/nodes/{id}/heartbeat", m.httpNodeHeartbeat)
	mux.HandleFunc("GET /v1/platform/plugins", m.httpPlugins)
	mux.HandleFunc("POST /v1/platform/plugins", m.httpPlugins)
	mux.HandleFunc("POST /v1/platform/plugins/{id}/enabled", m.httpPluginEnabled)
	mux.HandleFunc("GET /v1/platform/channels", m.httpChannels)
	mux.HandleFunc("POST /v1/platform/channels", m.httpChannels)
	mux.HandleFunc("POST /v1/platform/channels/{id}/enabled", m.httpChannelEnabled)
	mux.HandleFunc("GET /v1/platform/skills", m.httpSkills)
	mux.HandleFunc("POST /v1/platform/skills", m.httpSkills)
	mux.HandleFunc("POST /v1/platform/skills/{id}/enabled", m.httpSkillEnabled)
	mux.HandleFunc("GET /v1/platform/missions", m.httpMissions)
	mux.HandleFunc("POST /v1/platform/missions", m.httpMissions)
	mux.HandleFunc("GET /v1/platform/missions/{id}", m.httpMission)
	return mux
}

func (m *Manager) httpCapabilities(w http.ResponseWriter, _ *http.Request) {
	writePlatformJSON(w, http.StatusOK, map[string]any{
		"platform": "KINGAIBOT Platform Control Plane",
		"version":  "1.4",
		"capabilities": []string{
			"durable_sessions", "agent_profiles", "recurring_schedules", "durable_workflows",
			"parallel_multi_agent_missions", "device_nodes", "remote_plugins", "channel_adapters",
			"skills_with_integrity_hashes", "policy_gated_extension_tools", "restart_recovery",
			"audit_before_activation", "safe_scheduler", "node_heartbeat_gate",
		},
		"security_boundary": "all agent-triggered extension side effects remain mediated by the core tool policy, exact approvals and hash-chained audit log; trust-expanding platform activation is audit-first",
	})
}

func (m *Manager) httpAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Agents()
		respondPlatform(w, v, err)
		return
	}
	var in AgentProfile
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateAgentSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpAgentEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetAgentEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Sessions()
		respondPlatform(w, v, err)
		return
	}
	var in Session
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateSession(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpSession(w http.ResponseWriter, r *http.Request) {
	v, err := m.Session(r.PathValue("id"))
	respondPlatform(w, v, err)
}

func (m *Manager) httpSessionMessage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message string `json:"message"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.SendSessionSafe(r.PathValue("id"), in.Message)
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, v)
}

func (m *Manager) httpSchedules(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Schedules()
		respondPlatform(w, v, err)
		return
	}
	var in Schedule
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateScheduleSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpScheduleEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetScheduleEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Workflows()
		respondPlatform(w, v, err)
		return
	}
	var in Workflow
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateWorkflowSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpWorkflowEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetWorkflowEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpWorkflowRun(w http.ResponseWriter, r *http.Request) {
	v, err := m.RunWorkflowSafe(r.PathValue("id"))
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, v)
}

func (m *Manager) httpWorkflowRuns(w http.ResponseWriter, _ *http.Request) {
	v, err := m.WorkflowRuns()
	respondPlatform(w, v, err)
}

func (m *Manager) httpNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.NodesSafe()
		respondPlatform(w, v, err)
		return
	}
	var in Node
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateNodeSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Metadata map[string]any `json:"metadata"`
	}
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.HeartbeatNodeSafe(r.PathValue("id"), in.Metadata)
	respondPlatform(w, v, err)
}

func (m *Manager) httpPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Plugins()
		respondPlatform(w, v, err)
		return
	}
	var in Plugin
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreatePluginSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpPluginEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetPluginEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Channels()
		respondPlatform(w, v, err)
		return
	}
	var in Channel
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateChannelSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpChannelEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetChannelEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Skills()
		respondPlatform(w, v, err)
		return
	}
	var in Skill
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.CreateSkillSafe(in)
	respondCreated(w, v, err)
}

func (m *Manager) httpSkillEnabled(w http.ResponseWriter, r *http.Request) {
	enabled, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	v, err := m.SetSkillEnabledSafe(r.PathValue("id"), enabled)
	respondPlatform(w, v, err)
}

func (m *Manager) httpMissions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		v, err := m.Missions()
		respondPlatform(w, v, err)
		return
	}
	var in Mission
	if !decodePlatform(w, r, &in) {
		return
	}
	v, err := m.DispatchMissionSafe(in)
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusAccepted, v)
}

func (m *Manager) httpMission(w http.ResponseWriter, r *http.Request) {
	v, err := m.Mission(r.PathValue("id"))
	respondPlatform(w, v, err)
}

func decodeEnabled(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodePlatform(w, r, &in) {
		return false, false
	}
	if in.Enabled == nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "enabled_required"})
		return false, false
	}
	return *in.Enabled, true
}

func decodePlatform(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequest)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": err.Error()})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writePlatformJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "detail": "only one JSON object is allowed"})
		return false
	}
	return true
}

func respondCreated(w http.ResponseWriter, v any, err error) {
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusCreated, v)
}

func respondPlatform(w http.ResponseWriter, v any, err error) {
	if err != nil {
		platformProblem(w, err)
		return
	}
	writePlatformJSON(w, http.StatusOK, v)
}

func platformProblem(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "platform_error"
	if errors.Is(err, os.ErrNotExist) {
		status, code = http.StatusNotFound, "not_found"
	}
	if strings.Contains(strings.ToLower(err.Error()), "runtime queue") {
		status, code = http.StatusServiceUnavailable, "runtime_busy"
		w.Header().Set("Retry-After", strconv.Itoa(1))
	}
	writePlatformJSON(w, status, map[string]any{"error": code, "detail": err.Error()})
}

func writePlatformJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
