package cognition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const schemaVersion = 1

type Config struct {
	Enabled                      bool
	ReflectionInterval           time.Duration
	MaxPrinciples                int
	AutoProposalFailureThreshold int
	StoreTaskInputs              bool
}

type ProviderStats struct {
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

type Principle struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Confidence float64   `json:"confidence"`
	Evidence   int       `json:"evidence"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type State struct {
	SchemaVersion       int                       `json:"schema_version"`
	Identity            string                    `json:"identity"`
	Mode                string                    `json:"mode"`
	BootID              string                    `json:"boot_id"`
	BootCount           int                       `json:"boot_count"`
	LastBootAt          time.Time                 `json:"last_boot_at"`
	LastReflectionAt    time.Time                 `json:"last_reflection_at,omitempty"`
	Episodes            int                       `json:"episodes"`
	Successes           int                       `json:"successes"`
	Failures            int                       `json:"failures"`
	ProviderStats       map[string]*ProviderStats `json:"provider_stats"`
	FailurePatterns     map[string]int            `json:"failure_patterns"`
	FailureProposalMark map[string]int            `json:"failure_proposal_mark"`
	Principles          []Principle               `json:"principles"`
}

type Snapshot struct {
	State       State     `json:"state"`
	GeneratedAt time.Time `json:"generated_at"`
	Disclaimer  string    `json:"disclaimer"`
}

type EvolutionProposer interface {
	Propose(evolution.Proposal) (*evolution.Proposal, error)
}

type Engine struct {
	dir      string
	path     string
	cfg      Config
	memory   *memory.Store
	proposer EvolutionProposer
	events   *eventlog.Log

	mu     sync.RWMutex
	state  State
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(dir string, ms *memory.Store, proposer EvolutionProposer, events *eventlog.Log, cfg Config) (*Engine, error) {
	if events == nil {
		return nil, errors.New("cognition requires audit log")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if cfg.ReflectionInterval <= 0 {
		cfg.ReflectionInterval = 30 * time.Minute
	}
	if cfg.MaxPrinciples <= 0 {
		cfg.MaxPrinciples = 32
	}
	if cfg.MaxPrinciples > 256 {
		cfg.MaxPrinciples = 256
	}
	if cfg.AutoProposalFailureThreshold <= 0 {
		cfg.AutoProposalFailureThreshold = 3
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{dir: dir, path: filepath.Join(dir, "self-model.json"), cfg: cfg, memory: ms, proposer: proposer, events: events, ctx: ctx, cancel: cancel}
	if err := e.load(); err != nil {
		cancel()
		return nil, err
	}
	if e.state.ProviderStats == nil {
		e.state.ProviderStats = map[string]*ProviderStats{}
	}
	if e.state.FailurePatterns == nil {
		e.state.FailurePatterns = map[string]int{}
	}
	if e.state.FailureProposalMark == nil {
		e.state.FailureProposalMark = map[string]int{}
	}
	bootID, err := storage.RandomID("boot")
	if err != nil {
		cancel()
		return nil, err
	}
	e.state.SchemaVersion = schemaVersion
	e.state.Identity = "KINGAIBOT"
	e.state.Mode = "operational-self-model"
	e.state.BootID = bootID
	e.state.BootCount++
	e.state.LastBootAt = time.Now().UTC()
	if err := e.saveLocked(); err != nil {
		cancel()
		return nil, err
	}
	if err := events.Append(eventlog.Event{Type: "cognition.boot", Data: map[string]any{"boot_id": bootID, "boot_count": e.state.BootCount, "enabled": cfg.Enabled}}); err != nil {
		cancel()
		return nil, err
	}
	if cfg.Enabled {
		e.wg.Add(1)
		go e.reflectionLoop()
	}
	return e, nil
}

func (e *Engine) Close() {
	if e == nil {
		return
	}
	e.cancel()
	e.wg.Wait()
}

func (e *Engine) load() error {
	b, err := os.ReadFile(e.path)
	if os.IsNotExist(err) {
		e.state = State{SchemaVersion: schemaVersion, Identity: "KINGAIBOT", Mode: "operational-self-model", ProviderStats: map[string]*ProviderStats{}, FailurePatterns: map[string]int{}, FailureProposalMark: map[string]int{}}
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &e.state); err != nil {
		return err
	}
	return nil
}

func (e *Engine) saveLocked() error {
	b, err := json.MarshalIndent(&e.state, "", "  ")
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(e.path, b, 0o600)
}

func (e *Engine) reflectionLoop() {
	defer e.wg.Done()
	t := time.NewTicker(e.cfg.ReflectionInterval)
	defer t.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-t.C:
			_ = e.Reflect("scheduled")
		}
	}
}

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func clip(s string, n int) string {
	s = strings.TrimSpace(memory.SanitizeContent(s))
	if len(s) > n {
		return s[:n]
	}
	return s
}

func providerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unknown"
	}
	if len(name) > 128 {
		return name[:128]
	}
	return name
}

func classifyFailure(errText string) string {
	s := strings.ToLower(errText)
	switch {
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "timeout"
	case strings.Contains(s, "approval"):
		return "approval"
	case strings.Contains(s, "policy") || strings.Contains(s, "denied") || strings.Contains(s, "forbidden"):
		return "policy"
	case strings.Contains(s, "connection") || strings.Contains(s, "network") || strings.Contains(s, "dns") || strings.Contains(s, "http"):
		return "network-or-provider"
	case strings.Contains(s, "provider") || strings.Contains(s, "model") || strings.Contains(s, "rate limit"):
		return "provider"
	default:
		return "runtime"
	}
}

func (e *Engine) ObserveSuccess(taskID, input, output, provider string) error {
	if e == nil || !e.cfg.Enabled {
		return nil
	}
	provider = providerName(provider)
	now := time.Now().UTC()
	e.mu.Lock()
	e.state.Episodes++
	e.state.Successes++
	ps := e.state.ProviderStats[provider]
	if ps == nil {
		ps = &ProviderStats{}
		e.state.ProviderStats[provider] = ps
	}
	ps.Successes++
	ps.LastUsed = now
	err := e.saveLocked()
	e.mu.Unlock()
	if err != nil {
		return err
	}
	if e.memory != nil {
		content := "Task succeeded. provider=" + provider + " input_sha256=" + digest(input) + " output_sha256=" + digest(output)
		if e.cfg.StoreTaskInputs {
			content += "\nInput: " + clip(input, 2048)
		}
		content += "\nOutcome: " + clip(output, 2048)
		_ = e.memory.Add(memory.Record{Kind: "experience", Content: content, Source: "cognition:task:" + taskID, Importance: 0.65, Confidence: 0.9})
	}
	if err := e.events.Append(eventlog.Event{Type: "cognition.learned", TaskID: taskID, Data: map[string]any{"outcome": "success", "provider": provider}}); err != nil {
		return err
	}
	return e.reflectIfDue("success")
}

func (e *Engine) ObserveFailure(taskID, input, provider, errText string) error {
	if e == nil || !e.cfg.Enabled {
		return nil
	}
	provider = providerName(provider)
	category := classifyFailure(errText)
	now := time.Now().UTC()
	e.mu.Lock()
	e.state.Episodes++
	e.state.Failures++
	e.state.FailurePatterns[category]++
	count := e.state.FailurePatterns[category]
	mark := e.state.FailureProposalMark[category]
	ps := e.state.ProviderStats[provider]
	if ps == nil {
		ps = &ProviderStats{}
		e.state.ProviderStats[provider] = ps
	}
	ps.Failures++
	ps.LastUsed = now
	err := e.saveLocked()
	e.mu.Unlock()
	if err != nil {
		return err
	}
	if e.memory != nil {
		content := "Task failed. provider=" + provider + " category=" + category + " input_sha256=" + digest(input) + " error=" + clip(errText, 2048)
		if e.cfg.StoreTaskInputs {
			content += "\nInput: " + clip(input, 2048)
		}
		_ = e.memory.Add(memory.Record{Kind: "experience", Content: content, Source: "cognition:task:" + taskID, Importance: 0.8, Confidence: 0.95})
	}
	if err := e.events.Append(eventlog.Event{Type: "cognition.learned", TaskID: taskID, Data: map[string]any{"outcome": "failure", "provider": provider, "category": category, "pattern_count": count}}); err != nil {
		return err
	}
	threshold := e.cfg.AutoProposalFailureThreshold
	if e.proposer != nil && count >= threshold && count-mark >= threshold {
		p, pErr := e.proposer.Propose(evolution.Proposal{Kind: "learned-runtime-pattern", Title: "Improve repeated " + category + " failures", Rationale: "The cognitive runtime observed a repeated production failure pattern. This is a review-only proposal and has no authority to edit or release production code.", Evidence: map[string]any{"category": category, "observations": count, "last_task_id": taskID, "provider": provider, "error_sha256": digest(errText)}, Risk: "medium", Status: "proposed"})
		if pErr == nil && p != nil {
			e.mu.Lock()
			e.state.FailureProposalMark[category] = count
			_ = e.saveLocked()
			e.mu.Unlock()
		}
	}
	return e.reflectIfDue("failure")
}

func (e *Engine) reflectIfDue(reason string) error {
	e.mu.RLock()
	last := e.state.LastReflectionAt
	episodes := e.state.Episodes
	e.mu.RUnlock()
	if episodes%10 == 0 || last.IsZero() || time.Since(last) >= e.cfg.ReflectionInterval {
		return e.Reflect(reason)
	}
	return nil
}

func (e *Engine) upsertPrincipleLocked(id, text string, confidence float64, evidence int, now time.Time) {
	for i := range e.state.Principles {
		if e.state.Principles[i].ID == id {
			e.state.Principles[i].Text = text
			e.state.Principles[i].Confidence = confidence
			e.state.Principles[i].Evidence = evidence
			e.state.Principles[i].UpdatedAt = now
			return
		}
	}
	e.state.Principles = append(e.state.Principles, Principle{ID: id, Text: text, Confidence: confidence, Evidence: evidence, UpdatedAt: now})
}

func (e *Engine) Reflect(reason string) error {
	if e == nil || !e.cfg.Enabled {
		return nil
	}
	now := time.Now().UTC()
	e.mu.Lock()
	if e.state.Failures > 0 {
		e.upsertPrincipleLocked("reliability.reconcile", "On ambiguous failures or restarts, preserve evidence and require reconciliation instead of assuming success or blindly replaying side effects.", 0.99, e.state.Failures, now)
	}
	for name, ps := range e.state.ProviderStats {
		total := ps.Successes + ps.Failures
		if total < 3 {
			continue
		}
		rate := float64(ps.Successes) / float64(total)
		confidence := 0.5 + float64(total)/100
		if confidence > 0.95 {
			confidence = 0.95
		}
		text := "Provider " + name + " has recent observed success=" + itoa(ps.Successes) + " failure=" + itoa(ps.Failures) + ". Treat this as routing evidence, not permanent truth."
		if rate < 0.5 {
			text = "Provider " + name + " is currently failure-prone (success=" + itoa(ps.Successes) + ", failure=" + itoa(ps.Failures) + "); prefer a healthier configured fallback when policy and task requirements permit."
		}
		e.upsertPrincipleLocked("provider."+sanitizeID(name), text, confidence, total, now)
	}
	for category, count := range e.state.FailurePatterns {
		if count >= 3 {
			e.upsertPrincipleLocked("failure."+sanitizeID(category), "Repeated "+category+" failures require evidence-preserving diagnosis before increasing autonomy or retry scope.", 0.9, count, now)
		}
	}
	sort.Slice(e.state.Principles, func(i, j int) bool {
		if e.state.Principles[i].Confidence == e.state.Principles[j].Confidence {
			return e.state.Principles[i].UpdatedAt.After(e.state.Principles[j].UpdatedAt)
		}
		return e.state.Principles[i].Confidence > e.state.Principles[j].Confidence
	})
	if len(e.state.Principles) > e.cfg.MaxPrinciples {
		e.state.Principles = e.state.Principles[:e.cfg.MaxPrinciples]
	}
	e.state.LastReflectionAt = now
	err := e.saveLocked()
	principleCount := len(e.state.Principles)
	e.mu.Unlock()
	if err != nil {
		return err
	}
	return e.events.Append(eventlog.Event{Type: "cognition.reflected", Data: map[string]any{"reason": reason, "principles": principleCount}})
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	if len(out) > 64 {
		return out[:64]
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	b, _ := json.Marshal(e.state)
	var cp State
	_ = json.Unmarshal(b, &cp)
	return Snapshot{State: cp, GeneratedAt: time.Now().UTC(), Disclaimer: "Operational self-model for continuity, reflection and controlled learning. It is not a claim of subjective consciousness."}
}

func (e *Engine) Context(limit int) string {
	if e == nil || !e.cfg.Enabled || limit <= 0 {
		return ""
	}
	s := e.Snapshot().State
	var b strings.Builder
	b.WriteString("Operational self-model (not subjective consciousness): identity=KINGAIBOT; episodes=")
	b.WriteString(itoa(s.Episodes))
	b.WriteString("; successes=")
	b.WriteString(itoa(s.Successes))
	b.WriteString("; failures=")
	b.WriteString(itoa(s.Failures))
	b.WriteString(". Learned principles are advisory and never override user intent, policy, approvals, or authority:\n")
	for _, p := range s.Principles {
		line := "- " + p.Text + "\n"
		if b.Len()+len(line) > limit {
			break
		}
		b.WriteString(line)
	}
	out := b.String()
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/cognition/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e.Snapshot())
	})
	mux.HandleFunc("POST /v1/cognition/reflect", func(w http.ResponseWriter, _ *http.Request) {
		if err := e.Reflect("operator"); err != nil {
			http.Error(w, "reflection failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e.Snapshot())
	})
	return mux
}
