package platform

import (
	"sync"
	"time"
)

const (
	nativeSenderRateLimit      = 120
	nativeSenderRateWindow     = time.Minute
	nativeSenderRateMaxEntries = 16384
)

type nativeSenderRateEntry struct {
	WindowStart time.Time
	LastSeen    time.Time
	Count       int
}

type nativeSenderLimiter struct {
	mu      sync.Mutex
	entries map[string]nativeSenderRateEntry
}

var nativeSenderLimitersV170 sync.Map

func (g *channelGatewayV170) allowNativeSender(channelID, sender string, at time.Time) bool {
	value, _ := nativeSenderLimitersV170.LoadOrStore(g.manager, &nativeSenderLimiter{entries: map[string]nativeSenderRateEntry{}})
	limiter := value.(*nativeSenderLimiter)
	key := channelID + ":" + senderDigest(sender)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	entry, exists := limiter.entries[key]
	if !exists && len(limiter.entries) >= nativeSenderRateMaxEntries {
		cutoff := at.Add(-2 * nativeSenderRateWindow)
		for k, v := range limiter.entries {
			if v.LastSeen.Before(cutoff) {
				delete(limiter.entries, k)
			}
		}
		if len(limiter.entries) >= nativeSenderRateMaxEntries {
			oldestKey := ""
			var oldest time.Time
			for k, v := range limiter.entries {
				if oldestKey == "" || v.LastSeen.Before(oldest) {
					oldestKey = k
					oldest = v.LastSeen
				}
			}
			if oldestKey != "" {
				delete(limiter.entries, oldestKey)
			}
		}
	}
	if entry.WindowStart.IsZero() || at.Sub(entry.WindowStart) >= nativeSenderRateWindow {
		entry = nativeSenderRateEntry{WindowStart: at}
	}
	entry.Count++
	entry.LastSeen = at
	limiter.entries[key] = entry
	return entry.Count <= nativeSenderRateLimit
}
