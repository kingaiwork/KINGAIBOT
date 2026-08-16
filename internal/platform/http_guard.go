package platform

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	rootRateLimit         = 600
	adapterRootRateLimit  = 12000
	rootRateWindow        = time.Minute
	rootRateMaxEntries    = 8192
	adapterRateMaxEntries = 2048
)

type rootRateEntry struct {
	WindowStart time.Time
	Count       int
}

type rootLimiter struct {
	mu         sync.Mutex
	entries    map[string]rootRateEntry
	limit      int
	maxEntries int
}

func newRootLimiter() *rootLimiter {
	return newRootLimiterWith(rootRateLimit, rootRateMaxEntries)
}

func newRootLimiterWith(limit, maxEntries int) *rootLimiter {
	return &rootLimiter{entries: map[string]rootRateEntry{}, limit: limit, maxEntries: maxEntries}
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
	if !exists && len(l.entries) >= l.maxEntries {
		cutoff := now.Add(-2 * rootRateWindow)
		for k, v := range l.entries {
			if v.WindowStart.Before(cutoff) {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= l.maxEntries {
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
	return entry.Count <= l.limit
}

func probePath(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

func nativeAdapterPath(path string) bool {
	return strings.HasPrefix(path, "/v1/adapters/")
}

// HTTPGuard is the outermost daemon HTTP defense. It deliberately does not
// trust X-Forwarded-For because a deployment must explicitly configure a
// trusted reverse-proxy boundary before forwarded client identity is safe.
// Liveness/readiness probes are excluded from quota accounting so an external
// supervisor cannot accidentally create a self-induced restart loop.
//
// Native provider webhooks use a separate high-volume coarse limiter because
// many independent end-users legitimately share a small set of provider egress
// IPs. The limiter key also includes the concrete adapter path, so one busy
// Channel cannot consume the coarse ingress budget of another Channel that
// happens to share the same provider source IP.
func HTTPGuard(next http.Handler) http.Handler {
	defaultLimiter := newRootLimiter()
	adapterLimiter := newRootLimiterWith(adapterRootRateLimit, adapterRateMaxEntries)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if !probePath(r.URL.Path) {
			key := remoteKey(r)
			limiter := defaultLimiter
			if nativeAdapterPath(r.URL.Path) {
				limiter = adapterLimiter
				key += ":" + r.URL.Path
			}
			if !limiter.allow(key, time.Now().UTC()) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "rate_limited"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
