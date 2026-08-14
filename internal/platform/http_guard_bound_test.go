package platform

import (
	"fmt"
	"testing"
	"time"
)

func TestRootLimiterBoundsFreshEntryGrowth(t *testing.T) {
	l := newRootLimiter()
	now := time.Now().UTC()
	for i := 0; i < 9000; i++ {
		l.allow(fmt.Sprintf("198.51.100.%d", i), now)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) > 8192 {
		t.Fatalf("rate limiter map grew beyond hard ceiling: %d", len(l.entries))
	}
}
