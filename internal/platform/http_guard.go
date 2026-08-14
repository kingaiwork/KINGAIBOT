package platform

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	rootRateLimit      = 600
	rootRateWindow     = time.Minute
	rootRateMaxEntries = 8192
)

type rootRateEntry struct {
	WindowStart time.Time
	Count       int
}

type rootLimiter struct {
	mu      sync.Mutex
	entries map[string]rootRateEntry
}

func newRootLimiter() *rootLimiter {
	return &rootLimiter{entries: map[string]rootRateEntry{}}
}

func remoteKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func (l *rootLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.entries[key]
	if !exists && len(l.entries) >= rootRateMaxEntries {
		cutoff := now.Add(-2 * rootRateWindow)
		for k, v := range l.entries {
			if v.WindowStart.Before(cutoff) {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= rootRateMaxEntries {
			oldestKey := ""
			var oldest time.Time
			for k, v := range l.entries {
				if oldestKey == "" || v.WindowStart.Before(oldest) {
					oldestKey = k
					oldest = v.WindowStart
				}
			}
			if oldestKey != "" {
				delete(l.entries, oldestKey)
			}
		}
	}

	if entry.WindowStart.IsZero() || now.Sub(entry.WindowStart) >= rootRateWindow {
		entry = rootRateEntry{WindowStart: now, Count: 0}
	}
	entry.Count++
	l.entries[key] = entry
	return entry.Count <= rootRateLimit
}

// HTTPGuard is the outermost daemon HTTP defense. It deliberately does not
// trust X-Forwarded-For because a deployment must explicitly configure a
// trusted reverse-proxy boundary before forwarded client identity is safe.
func HTTPGuard(next http.Handler) http.Handler {
	limiter := newRootLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if !limiter.allow(remoteKey(r), time.Now().UTC()) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "rate_limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
