package policy

import "strings"

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

type Engine struct {
	Default Decision
	Tools   map[string]Decision
}

func New(def string, tools map[string]string) *Engine {
	e := &Engine{Default: parse(def), Tools: map[string]Decision{}}
	for k, v := range tools {
		e.Tools[k] = parse(v)
	}
	return e
}
func parse(s string) Decision {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return Allow
	case "ask":
		return Ask
	default:
		return Deny
	}
}
func (e *Engine) Evaluate(tool string) Decision {
	if d, ok := e.Tools[tool]; ok {
		return d
	}
	return e.Default
}
