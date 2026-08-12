package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

const maxRecordContent = 32 << 10

type Record struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Content    string     `json:"content"`
	Source     string     `json:"source"`
	Importance float64    `json:"importance"`
	Confidence float64    `json:"confidence"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type Store struct {
	path       string
	mu         sync.RWMutex
	maxRecords int
	count      int
}

var secretPatterns = []struct {
	re *regexp.Regexp
	r  string
}{
	{regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer)\s+[^\s,;]+`), `$1 [REDACTED]`},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`), `[REDACTED_OPENAI_KEY]`},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`), `[REDACTED_GITHUB_TOKEN]`},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), `[REDACTED_JWT]`},
	{regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|token|secret|password|passwd)\b\s*[:=]\s*["']?[A-Za-z0-9_./+=:@-]{8,}["']?`), `$1=[REDACTED]`},
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), `[REDACTED_PRIVATE_KEY]`},
}

func SanitizeContent(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.r)
	}
	return s
}

func New(dir string, maxRecords ...int) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := 5000
	if len(maxRecords) > 0 && maxRecords[0] > 0 {
		m = maxRecords[0]
	}
	s := &Store{path: filepath.Join(dir, "memory.jsonl"), maxRecords: m}
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		s.count++
	}
	scanErr := sc.Err()
	closeErr := f.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return s, nil
}

func (s *Store) Add(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == "" {
		id, err := storage.RandomID("mem")
		if err != nil {
			return err
		}
		r.ID = id
	}
	if err := storage.ValidateID(r.ID); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	r.Content = SanitizeContent(r.Content)
	if len(r.Content) > maxRecordContent {
		r.Content = r.Content[:maxRecordContent]
	}
	r.Importance = clamp01(r.Importance)
	r.Confidence = clamp01(r.Confidence)
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.count++
	if s.count > s.maxRecords+s.maxRecords/10 {
		return s.compactLocked()
	}
	return nil
}

func (s *Store) compactLocked() error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		cp := append([]byte(nil), sc.Bytes()...)
		lines = append(lines, cp)
		if len(lines) > s.maxRecords {
			lines = lines[1:]
		}
	}
	scanErr := sc.Err()
	closeErr := f.Close()
	if scanErr != nil {
		return scanErr
	}
	if closeErr != nil {
		return closeErr
	}
	var out bytes.Buffer
	for _, line := range lines {
		out.Write(line)
		out.WriteByte('\n')
	}
	if err := storage.AtomicWriteFile(s.path, out.Bytes(), 0o600); err != nil {
		return err
	}
	s.count = len(lines)
	return nil
}

func (s *Store) Search(q string, limit int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	type scored struct {
		r Record
		s float64
	}
	var all []scored
	qs := tokens(q)
	now := time.Now().UTC()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if r.ExpiresAt != nil && !r.ExpiresAt.After(now) {
			continue
		}
		rs := tokens(r.Content)
		score := overlap(qs, rs) * (0.5 + 0.5*clamp01(r.Importance)) * (0.5 + 0.5*clamp01(r.Confidence))
		if score > 0 {
			all = append(all, scored{r, score})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].s == all[j].s {
			return all[i].r.CreatedAt.After(all[j].r.CreatedAt)
		}
		return all[i].s > all[j].s
	})
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	out := make([]Record, 0, limit)
	for _, x := range all[:limit] {
		out = append(out, x.r)
	}
	return out, nil
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func tokens(s string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, p := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x4e00 && r <= 0x9fff)
	}) {
		if len([]rune(p)) > 1 {
			m[p] = struct{}{}
		}
	}
	return m
}

func overlap(a, b map[string]struct{}) float64 {
	if len(a) == 0 {
		return 0
	}
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return float64(n) / float64(len(a))
}
