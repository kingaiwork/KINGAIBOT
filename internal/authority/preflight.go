package authority

import (
	"fmt"
	"sort"
	"time"
)

const maxPreflightCostUnits int64 = 1_000_000_000_000

type BudgetLevel struct {
	AuthorityID             string `json:"authority_id"`
	SubjectID               string `json:"subject_id"`
	ActiveWork              int    `json:"active_work"`
	MaxConcurrentWork       int    `json:"max_concurrent_work"`
	RemainingConcurrentWork int    `json:"remaining_concurrent_work"`
	ConsumedCostUnits       int64  `json:"consumed_cost_units"`
	MaxCostUnits            int64  `json:"max_cost_units"`
	RemainingCostUnits      int64  `json:"remaining_cost_units"`
	UnlimitedConcurrency    bool   `json:"unlimited_concurrency"`
	UnlimitedCost           bool   `json:"unlimited_cost"`
}

type BudgetPreflight struct {
	AuthorityID string        `json:"authority_id"`
	CostUnits   int64         `json:"cost_units"`
	Allowed     bool          `json:"allowed"`
	Reasons     []string      `json:"reasons,omitempty"`
	Lineage     []BudgetLevel `json:"lineage"`
	CheckedAt   time.Time     `json:"checked_at"`
	Advisory    string        `json:"advisory"`
}

// Preflight performs a read-only budget projection across the leaf authority
// and every ancestor. It is intentionally advisory: execution still performs
// atomic reservation and per-attempt charging immediately before work becomes
// leaseable/deliverable, so callers cannot use preflight as a TOCTOU bypass.
func (s *Store) Preflight(authorityID string, costUnits int64) (*BudgetPreflight, error) {
	if costUnits < 0 || costUnits > maxPreflightCostUnits {
		return nil, fmt.Errorf("cost_units must be between 0 and %d", maxPreflightCostUnits)
	}
	now := time.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	lineage, err := s.lineageLocked(authorityID, now, true)
	if err != nil {
		return nil, err
	}
	result := &BudgetPreflight{
		AuthorityID: authorityID,
		CostUnits:   costUnits,
		Allowed:     true,
		CheckedAt:   now,
		Advisory:    "Preflight is advisory only; execution rechecks authority and budgets at reservation and lease time.",
		Lineage:     make([]BudgetLevel, 0, len(lineage)),
	}
	for _, grant := range lineage {
		activeDir, err := s.usageCollectionDir(grant.Envelope.ID, "active")
		if err != nil {
			return nil, err
		}
		chargeDir, err := s.usageCollectionDir(grant.Envelope.ID, "charges")
		if err != nil {
			return nil, err
		}
		active, err := usageFileCount(activeDir)
		if err != nil {
			return nil, err
		}
		consumed, err := consumedCost(chargeDir)
		if err != nil {
			return nil, err
		}
		level := BudgetLevel{
			AuthorityID:          grant.Envelope.ID,
			SubjectID:            grant.Envelope.SubjectID,
			ActiveWork:           active,
			MaxConcurrentWork:    grant.Envelope.MaxConcurrentWork,
			ConsumedCostUnits:    consumed,
			MaxCostUnits:         grant.Envelope.MaxCostUnits,
			UnlimitedConcurrency: grant.Envelope.MaxConcurrentWork == 0,
			UnlimitedCost:        grant.Envelope.MaxCostUnits == 0,
		}
		if level.MaxConcurrentWork > 0 {
			remaining := level.MaxConcurrentWork - active
			if remaining < 0 {
				remaining = 0
			}
			level.RemainingConcurrentWork = remaining
			if active >= level.MaxConcurrentWork {
				result.Allowed = false
				result.Reasons = append(result.Reasons, fmt.Sprintf("authority %s has no concurrent-work capacity", level.AuthorityID))
			}
		}
		if level.MaxCostUnits > 0 {
			remaining := level.MaxCostUnits - consumed
			if remaining < 0 {
				remaining = 0
			}
			level.RemainingCostUnits = remaining
			if costUnits == 0 {
				result.Allowed = false
				result.Reasons = append(result.Reasons, fmt.Sprintf("authority %s requires a positive trusted cost estimate", level.AuthorityID))
			} else if consumed > level.MaxCostUnits-costUnits {
				result.Allowed = false
				result.Reasons = append(result.Reasons, fmt.Sprintf("authority %s lacks %d requested cost units", level.AuthorityID, costUnits))
			}
		}
		result.Lineage = append(result.Lineage, level)
	}
	return result, nil
}

// UsageOverview returns current accounting snapshots for every persisted grant.
// Pending/revoked grants remain visible to administrators for governance and
// forensic accounting even though they cannot authorize new execution.
func (s *Store) UsageOverview() ([]*UsageSnapshot, error) {
	grants, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]*UsageSnapshot, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		usage, err := s.Usage(grant.Envelope.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, usage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AuthorityID < out[j].AuthorityID })
	return out, nil
}
