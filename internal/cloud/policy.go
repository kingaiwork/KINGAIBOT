package cloud

import (
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
)

func ApplyRestrictions(cfg *config.Config, p Policy) {
	if cfg == nil || p.Version <= 0 {
		return
	}
	if p.MaxSteps > 0 && p.MaxSteps < cfg.Runtime.MaxSteps {
		cfg.Runtime.MaxSteps = p.MaxSteps
	}
	if p.MaxWorkerCount > 0 && p.MaxWorkerCount < cfg.Runtime.WorkerCount {
		cfg.Runtime.WorkerCount = p.MaxWorkerCount
	}
	if p.MaxTaskTimeoutSeconds > 0 && p.MaxTaskTimeoutSeconds < cfg.Runtime.TaskTimeoutSeconds {
		cfg.Runtime.TaskTimeoutSeconds = p.MaxTaskTimeoutSeconds
	}
	disabled := map[string]struct{}{}
	for _, name := range p.DisabledProviders {
		disabled[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	for i := range cfg.Providers {
		if _, ok := disabled[strings.ToLower(cfg.Providers[i].Name)]; ok {
			cfg.Providers[i].Enabled = false
		}
	}
	remoteDefault := strings.ToLower(strings.TrimSpace(p.DefaultToolPolicy))
	if tighterPolicy(remoteDefault, cfg.Security.DefaultToolPolicy) {
		cfg.Security.DefaultToolPolicy = remoteDefault
	}
	for tool, remote := range p.ToolPolicies {
		tool = strings.TrimSpace(tool)
		remote = strings.ToLower(strings.TrimSpace(remote))
		if tool == "" {
			continue
		}
		local := cfg.Security.ToolPolicies[tool]
		if local == "" {
			local = cfg.Security.DefaultToolPolicy
		}
		if tighterPolicy(remote, local) {
			cfg.Security.ToolPolicies[tool] = remote
		}
	}
}

func tighterPolicy(remote, local string) bool {
	rank := map[string]int{"allow": 1, "ask": 2, "deny": 3}
	r, rok := rank[remote]
	l, lok := rank[strings.ToLower(strings.TrimSpace(local))]
	return rok && lok && r > l
}
