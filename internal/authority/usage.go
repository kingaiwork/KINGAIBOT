package authority

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const (
	maxUsageReferenceBytes = 512
	maxUsageRecords        = 200000
)

type UsageSnapshot struct {
	AuthorityID          string    `json:"authority_id"`
	ActiveWork           int       `json:"active_work"`
	MaxConcurrentWork    int       `json:"max_concurrent_work"`
	ConsumedCostUnits    int64     `json:"consumed_cost_units"`
	MaxCostUnits         int64     `json:"max_cost_units"`
	UnlimitedConcurrency bool      `json:"unlimited_concurrency"`
	UnlimitedCost        bool      `json:"unlimited_cost"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type workUsageMarker struct {
	BudgetAuthorityID string    `json:"budget_authority_id"`
	WorkAuthorityID   string    `json:"work_authority_id"`
	WorkID            string    `json:"work_id"`
	CreatedAt         time.Time `json:"created_at"`
}

type costUsageMarker struct {
	BudgetAuthorityID string    `json:"budget_authority_id"`
	WorkAuthorityID   string    `json:"work_authority_id"`
	ChargeID          string    `json:"charge_id"`
	Units             int64     `json:"units"`
	CreatedAt         time.Time `json:"created_at"`
}

func validateUsageReference(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxUsageReferenceBytes {
		return "", fmt.Errorf("%s required and must be <= %d bytes", kind, maxUsageReferenceBytes)
	}
	return value, nil
}

func usageKey(workAuthorityID, reference string) string {
	sum := sha256.Sum256([]byte(workAuthorityID + "\x00" + reference))
	return hex.EncodeToString(sum[:])
}

func (s *Store) usageAuthorityDir(authorityID string) (string, error) {
	if err := storage.ValidateID(authorityID); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, "usage", authorityID), nil
}

func (s *Store) usageCollectionDir(authorityID, collection string) (string, error) {
	base, err := s.usageAuthorityDir(authorityID)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, collection), nil
}

func (s *Store) usageMarkerPath(budgetAuthorityID, collection, workAuthorityID, reference string) (string, error) {
	dir, err := s.usageCollectionDir(budgetAuthorityID, collection)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, usageKey(workAuthorityID, reference)+".json"), nil
}

func saveUsageMarker(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, data, 0o600)
}

// lineageLocked returns the leaf grant followed by every ancestor. When
// requireEffective is true, every grant in the chain must be active, unexpired
// and valid. Hierarchical accounting against every returned envelope prevents
// descendants from multiplying a parent's budget through delegation.
func (s *Store) lineageLocked(authorityID string, now time.Time, requireEffective bool) ([]*Grant, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	current := strings.TrimSpace(authorityID)
	if err := storage.ValidateID(current); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	lineage := make([]*Grant, 0, 4)
	for current != "" {
		if _, ok := seen[current]; ok {
			return nil, errors.New("authority parent cycle detected")
		}
		seen[current] = struct{}{}
		grant, err := s.loadLocked(current)
		if err != nil {
			return nil, err
		}
		if requireEffective {
			if grant.Status != "active" {
				return nil, errors.New("authority grant is not active")
			}
			if grant.RevokedAt != nil {
				return nil, errors.New("authority grant is revoked")
			}
			if err := grant.Envelope.Validate(now); err != nil {
				return nil, err
			}
		}
		lineage = append(lineage, grant)
		current = strings.TrimSpace(grant.ParentID)
	}
	return lineage, nil
}

func usageFileCount(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count, nil
}

func consumedCost(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if len(entries) > maxUsageRecords {
		return 0, errors.New("authority cost usage record limit exceeded")
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return 0, readErr
		}
		var marker costUsageMarker
		if err := json.Unmarshal(data, &marker); err != nil {
			return 0, fmt.Errorf("invalid authority cost marker %s: %w", entry.Name(), err)
		}
		if marker.Units <= 0 || total > int64(^uint64(0)>>1)-marker.Units {
			return 0, errors.New("invalid or overflowing authority cost usage")
		}
		total += marker.Units
	}
	return total, nil
}

// ReserveWork consumes one concurrent-work slot from the leaf authority and
// every ancestor. The operation is idempotent for the same authority/work ID.
func (s *Store) ReserveWork(authorityID, workID string) error {
	workID, err := validateUsageReference("work_id", workID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	lineage, err := s.lineageLocked(authorityID, now, true)
	if err != nil {
		return err
	}
	type target struct {
		path   string
		marker workUsageMarker
		exists bool
	}
	targets := make([]target, 0, len(lineage))
	for _, grant := range lineage {
		dir, err := s.usageCollectionDir(grant.Envelope.ID, "active")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		path, err := s.usageMarkerPath(grant.Envelope.ID, "active", authorityID, workID)
		if err != nil {
			return err
		}
		if data, readErr := os.ReadFile(path); readErr == nil {
			var marker workUsageMarker
			if json.Unmarshal(data, &marker) != nil || marker.BudgetAuthorityID != grant.Envelope.ID || marker.WorkAuthorityID != authorityID || marker.WorkID != workID {
				return errors.New("authority work reservation marker mismatch")
			}
			targets = append(targets, target{path: path, marker: marker, exists: true})
			continue
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		count, err := usageFileCount(dir)
		if err != nil {
			return err
		}
		if count >= maxUsageRecords {
			return errors.New("authority active-work record limit exceeded")
		}
		if max := grant.Envelope.MaxConcurrentWork; max > 0 && count >= max {
			return fmt.Errorf("concurrency budget exhausted for authority %s", grant.Envelope.ID)
		}
		targets = append(targets, target{path: path, marker: workUsageMarker{
			BudgetAuthorityID: grant.Envelope.ID,
			WorkAuthorityID:   authorityID,
			WorkID:            workID,
			CreatedAt:         now,
		}})
	}
	created := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.exists {
			continue
		}
		if err := saveUsageMarker(target.path, &target.marker); err != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
			return err
		}
		created = append(created, target.path)
	}
	if len(created) == 0 {
		return nil
	}
	if err := s.events.Append(eventlog.Event{Type: "authority.work.reserved", Data: map[string]any{
		"authority_id": authorityID,
		"work_id":      workID,
		"lineage_size": len(lineage),
	}}); err != nil {
		for _, path := range created {
			_ = os.Remove(path)
		}
		return fmt.Errorf("authority work reservation rolled back because audit failed: %w", err)
	}
	return nil
}

// ReleaseWork frees a concurrent-work slot across the full lineage. Audit is
// written before deletion because releasing capacity expands what may execute.
// If cleanup cannot finish, stale markers remain fail-closed and retry safely.
func (s *Store) ReleaseWork(authorityID, workID string) error {
	workID, err := validateUsageReference("work_id", workID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lineage, err := s.lineageLocked(authorityID, time.Now().UTC(), false)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(lineage))
	for _, grant := range lineage {
		path, err := s.usageMarkerPath(grant.Envelope.ID, "active", authorityID, workID)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(path); statErr == nil {
			paths = append(paths, path)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	if len(paths) == 0 {
		return nil
	}
	if err := s.events.Append(eventlog.Event{Type: "authority.work.release_authorized", Data: map[string]any{
		"authority_id": authorityID,
		"work_id":      workID,
		"lineage_size": len(paths),
	}}); err != nil {
		return fmt.Errorf("authority work release blocked because audit failed: %w", err)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("authority work release cleanup incomplete: %w", err)
		}
	}
	return nil
}

// ChargeCost permanently consumes cost units from the leaf authority and every
// ancestor. chargeID makes the operation idempotent. A bounded authority may
// not execute work with an unspecified (zero) cost estimate.
func (s *Store) ChargeCost(authorityID, chargeID string, units int64) error {
	chargeID, err := validateUsageReference("charge_id", chargeID)
	if err != nil {
		return err
	}
	if units < 0 {
		return errors.New("cost units cannot be negative")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	lineage, err := s.lineageLocked(authorityID, now, true)
	if err != nil {
		return err
	}
	if units == 0 {
		for _, grant := range lineage {
			if grant.Envelope.MaxCostUnits > 0 {
				return fmt.Errorf("positive cost_units required by bounded authority %s", grant.Envelope.ID)
			}
		}
		return nil
	}
	type target struct {
		path   string
		marker costUsageMarker
		exists bool
	}
	targets := make([]target, 0, len(lineage))
	for _, grant := range lineage {
		dir, err := s.usageCollectionDir(grant.Envelope.ID, "charges")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		path, err := s.usageMarkerPath(grant.Envelope.ID, "charges", authorityID, chargeID)
		if err != nil {
			return err
		}
		if data, readErr := os.ReadFile(path); readErr == nil {
			var marker costUsageMarker
			if json.Unmarshal(data, &marker) != nil || marker.BudgetAuthorityID != grant.Envelope.ID || marker.WorkAuthorityID != authorityID || marker.ChargeID != chargeID || marker.Units != units {
				return errors.New("authority cost charge marker mismatch")
			}
			targets = append(targets, target{path: path, marker: marker, exists: true})
			continue
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		count, err := usageFileCount(dir)
		if err != nil {
			return err
		}
		if count >= maxUsageRecords {
			return errors.New("authority cost usage record limit exceeded")
		}
		consumed, err := consumedCost(dir)
		if err != nil {
			return err
		}
		if max := grant.Envelope.MaxCostUnits; max > 0 && (consumed > max-units) {
			return fmt.Errorf("cost budget exhausted for authority %s", grant.Envelope.ID)
		}
		targets = append(targets, target{path: path, marker: costUsageMarker{
			BudgetAuthorityID: grant.Envelope.ID,
			WorkAuthorityID:   authorityID,
			ChargeID:          chargeID,
			Units:             units,
			CreatedAt:         now,
		}})
	}
	created := 0
	for _, target := range targets {
		if target.exists {
			continue
		}
		if err := saveUsageMarker(target.path, &target.marker); err != nil {
			// Cost consumption is monotonic/fail-closed. Already-written markers
			// intentionally remain so a partial write cannot create extra budget.
			return err
		}
		created++
	}
	if created == 0 {
		return nil
	}
	if err := s.events.Append(eventlog.Event{Type: "authority.cost.charged", Data: map[string]any{
		"authority_id": authorityID,
		"charge_id":    chargeID,
		"cost_units":   units,
		"lineage_size": len(lineage),
	}}); err != nil {
		// Keep charge markers on audit failure. Consuming budget without executing
		// is conservative; restoring budget without a durable audit could widen
		// authority after an ambiguous failure.
		return fmt.Errorf("authority cost charge persisted but audit failed: %w", err)
	}
	return nil
}

func (s *Store) HasWorkReservation(authorityID, workID string) error {
	workID, err := validateUsageReference("work_id", workID)
	if err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	lineage, err := s.lineageLocked(authorityID, time.Now().UTC(), true)
	if err != nil {
		return err
	}
	for _, grant := range lineage {
		path, err := s.usageMarkerPath(grant.Envelope.ID, "active", authorityID, workID)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("authority work reservation missing")
		}
		var marker workUsageMarker
		if json.Unmarshal(data, &marker) != nil || marker.BudgetAuthorityID != grant.Envelope.ID || marker.WorkAuthorityID != authorityID || marker.WorkID != workID {
			return errors.New("authority work reservation marker mismatch")
		}
	}
	return nil
}

func (s *Store) Usage(authorityID string) (*UsageSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, err := s.loadLocked(strings.TrimSpace(authorityID))
	if err != nil {
		return nil, err
	}
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
	cost, err := consumedCost(chargeDir)
	if err != nil {
		return nil, err
	}
	return &UsageSnapshot{
		AuthorityID:          grant.Envelope.ID,
		ActiveWork:           active,
		MaxConcurrentWork:    grant.Envelope.MaxConcurrentWork,
		ConsumedCostUnits:    cost,
		MaxCostUnits:         grant.Envelope.MaxCostUnits,
		UnlimitedConcurrency: grant.Envelope.MaxConcurrentWork == 0,
		UnlimitedCost:        grant.Envelope.MaxCostUnits == 0,
		UpdatedAt:            time.Now().UTC(),
	}, nil
}
