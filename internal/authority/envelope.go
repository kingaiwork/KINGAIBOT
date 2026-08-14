package authority

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Envelope is KINGAIBOT's model-independent authority boundary for an agent,
// workflow, mission or delegated worker. Authority is data owned by the
// platform; it is never inferred from model output.
type Envelope struct {
	ID                string     `json:"id"`
	SubjectID         string     `json:"subject_id"`
	Capabilities      []string   `json:"capabilities,omitempty"`
	DataScopes        []string   `json:"data_scopes,omitempty"`
	ToolScopes        []string   `json:"tool_scopes,omitempty"`
	MaxConcurrentWork int        `json:"max_concurrent_work,omitempty"`
	MaxCostUnits      int64      `json:"max_cost_units,omitempty"`
	AllowDelegation   bool       `json:"allow_delegation"`
	DelegationDepth   int        `json:"delegation_depth,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

func (e Envelope) Validate(now time.Time) error {
	if strings.TrimSpace(e.SubjectID) == "" {
		return errors.New("authority envelope requires subject_id")
	}
	if e.MaxConcurrentWork < 0 {
		return errors.New("max_concurrent_work cannot be negative")
	}
	if e.MaxCostUnits < 0 {
		return errors.New("max_cost_units cannot be negative")
	}
	if e.DelegationDepth < 0 {
		return errors.New("delegation_depth cannot be negative")
	}
	if !e.AllowDelegation && e.DelegationDepth != 0 {
		return errors.New("delegation_depth must be zero when delegation is disabled")
	}
	if e.ExpiresAt != nil && !e.ExpiresAt.After(now) {
		return errors.New("authority envelope is expired")
	}
	if err := validateScopes("capability", e.Capabilities); err != nil {
		return err
	}
	if err := validateScopes("data scope", e.DataScopes); err != nil {
		return err
	}
	if err := validateScopes("tool scope", e.ToolScopes); err != nil {
		return err
	}
	return nil
}

func validateScopes(kind string, values []string) error {
	seen := map[string]struct{}{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			return fmt.Errorf("%s cannot be empty", kind)
		}
		if _, ok := seen[v]; ok {
			return fmt.Errorf("duplicate %s %q", kind, v)
		}
		seen[v] = struct{}{}
	}
	return nil
}

// Allows reports whether a requested value is inside a scope set. KING scopes
// support exact values and namespace wildcards ending in .* only.
func Allows(scopes []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == requested || scope == "*" {
			return true
		}
		if strings.HasSuffix(scope, ".*") {
			prefix := strings.TrimSuffix(scope, "*")
			if strings.HasPrefix(requested, prefix) {
				return true
			}
		}
	}
	return false
}

func (e Envelope) AllowsCapability(capability string) bool { return Allows(e.Capabilities, capability) }
func (e Envelope) AllowsDataScope(scope string) bool       { return Allows(e.DataScopes, scope) }
func (e Envelope) AllowsTool(tool string) bool             { return Allows(e.ToolScopes, tool) }

// Derive creates a strictly equal-or-narrower delegated envelope. It is the
// only supported way to derive authority. A child cannot widen permissions,
// budgets, lifetime or delegation depth beyond its parent.
func (e Envelope) Derive(child Envelope, now time.Time) (Envelope, error) {
	if err := e.Validate(now); err != nil {
		return Envelope{}, fmt.Errorf("parent authority invalid: %w", err)
	}
	if !e.AllowDelegation || e.DelegationDepth <= 0 {
		return Envelope{}, errors.New("parent authority does not allow delegation")
	}
	if err := child.Validate(now); err != nil {
		return Envelope{}, fmt.Errorf("child authority invalid: %w", err)
	}
	if strings.TrimSpace(child.SubjectID) == strings.TrimSpace(e.SubjectID) {
		return Envelope{}, errors.New("delegation requires a different subject")
	}
	if !subset(child.Capabilities, e.Capabilities) {
		return Envelope{}, errors.New("child capabilities exceed parent authority")
	}
	if !subset(child.DataScopes, e.DataScopes) {
		return Envelope{}, errors.New("child data scopes exceed parent authority")
	}
	if !subset(child.ToolScopes, e.ToolScopes) {
		return Envelope{}, errors.New("child tool scopes exceed parent authority")
	}
	if exceedsInt(child.MaxConcurrentWork, e.MaxConcurrentWork) {
		return Envelope{}, errors.New("child concurrency budget exceeds parent authority")
	}
	if exceedsInt64(child.MaxCostUnits, e.MaxCostUnits) {
		return Envelope{}, errors.New("child cost budget exceeds parent authority")
	}
	if child.AllowDelegation {
		if child.DelegationDepth >= e.DelegationDepth {
			return Envelope{}, errors.New("child delegation depth must be lower than parent")
		}
	} else if child.DelegationDepth != 0 {
		return Envelope{}, errors.New("child delegation depth must be zero when delegation is disabled")
	}
	if e.ExpiresAt != nil {
		if child.ExpiresAt == nil || child.ExpiresAt.After(*e.ExpiresAt) {
			return Envelope{}, errors.New("child lifetime exceeds parent authority")
		}
	}

	child.Capabilities = canonical(child.Capabilities)
	child.DataScopes = canonical(child.DataScopes)
	child.ToolScopes = canonical(child.ToolScopes)
	return child, nil
}

func subset(child, parent []string) bool {
	for _, value := range child {
		if !Allows(parent, value) {
			return false
		}
	}
	return true
}

// A parent budget of zero means unlimited. A child may choose zero only when
// the parent is also unlimited; otherwise zero would accidentally widen it.
func exceedsInt(child, parent int) bool {
	if parent == 0 {
		return false
	}
	return child == 0 || child > parent
}

func exceedsInt64(child, parent int64) bool {
	if parent == 0 {
		return false
	}
	return child == 0 || child > parent
}

func canonical(values []string) []string {
	out := append([]string(nil), values...)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	sort.Strings(out)
	return out
}
