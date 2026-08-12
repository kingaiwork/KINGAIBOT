package eventlog

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time     time.Time      `json:"time"`
	Type     string         `json:"type"`
	TaskID   string         `json:"task_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	Sequence uint64         `json:"sequence,omitempty"`
	PrevHash string         `json:"prev_hash,omitempty"`
	Hash     string         `json:"hash,omitempty"`
}

type Log struct {
	path     string
	mu       sync.Mutex
	lastHash string
	sequence uint64
	broken   error
}

func New(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	l := &Log{path: filepath.Join(dir, "events.jsonl")}
	if err := l.verifyAndLoad(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) verifyAndLoad() error {
	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 4<<20)
	last := ""
	var seq uint64
	hashedStarted := false
	line := 0
	for s.Scan() {
		line++
		var e Event
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			return fmt.Errorf("audit log line %d invalid JSON: %w", line, err)
		}
		if e.Hash == "" {
			if hashedStarted {
				return fmt.Errorf("audit log line %d is unhashed after hash chain started", line)
			}
			continue
		}
		hashedStarted = true
		if e.PrevHash != last {
			return fmt.Errorf("audit log chain mismatch at line %d", line)
		}
		expected, err := hashEvent(e)
		if err != nil {
			return err
		}
		if e.Hash != expected {
			return fmt.Errorf("audit log hash mismatch at line %d", line)
		}
		if e.Sequence <= seq {
			return fmt.Errorf("audit log sequence not monotonic at line %d", line)
		}
		seq = e.Sequence
		last = e.Hash
	}
	if err := s.Err(); err != nil {
		return err
	}
	l.lastHash = last
	l.sequence = seq
	return nil
}

func hashEvent(e Event) (string, error) {
	e.Hash = ""
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func (l *Log) Append(e Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.broken != nil {
		return fmt.Errorf("audit log is in failed state: %w", l.broken)
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Type == "" {
		return errors.New("event type required")
	}
	l.sequence++
	e.Sequence = l.sequence
	e.PrevHash = l.lastHash
	h, err := hashEvent(e)
	if err != nil {
		l.sequence--
		return err
	}
	e.Hash = h
	b, err := json.Marshal(e)
	if err != nil {
		l.sequence--
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		l.sequence--
		l.broken = err
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		l.sequence--
		l.broken = err
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		l.sequence--
		l.broken = err
		return err
	}
	if err = f.Close(); err != nil {
		l.sequence--
		l.broken = err
		return err
	}
	l.lastHash = h
	return nil
}

func (l *Log) Verify() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	oldHash, oldSeq := l.lastHash, l.sequence
	l.lastHash = ""
	l.sequence = 0
	err := l.verifyAndLoad()
	if err != nil {
		l.lastHash, l.sequence = oldHash, oldSeq
		l.broken = err
		return err
	}
	l.broken = nil
	return nil
}

func (l *Log) Healthy() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.broken != nil {
		return fmt.Errorf("audit log unhealthy: %w", l.broken)
	}
	return nil
}
