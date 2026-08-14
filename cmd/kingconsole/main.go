package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
	"github.com/kingaiwork/KINGAIBOT/internal/platform"
)

var version = "1.4.0"

func main() {
	listen := flag.String("listen", "127.0.0.1:18889", "console listen address")
	apiBase := flag.String("api", "http://127.0.0.1:18888", "KINGAIBOT API base URL")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	base, err := validateAPIBase(*apiBase)
	if err != nil {
		slog.Error("invalid API base", "error", err)
		os.Exit(2)
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           newConsoleHandler(base),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      150 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		slog.Info("KINGAIBOT console started", "listen", *listen, "api", base, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("console server", "error", err)
			os.Exit(1)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// newConsoleHandler keeps every mounted route in the same ServeMux pattern
// class. Go 1.22+ rejects ambiguous combinations such as a method-specific
// "GET /" catch-all next to method-agnostic subtree mounts like "/ui/".
// Method enforcement for the root redirect is therefore done inside the root
// handler instead of in the pattern string.
func newConsoleHandler(base string) http.Handler {
	proxy := newAPIProxy(base)
	mux := http.NewServeMux()
	mux.Handle("/ui/", platform.ControlCenterHandler())
	mux.Handle("/v1/", proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	return securityHeaders(mux)
}

func validateAPIBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("API URL requires host and must not contain credentials/query/fragment")
	}
	if u.Scheme == "https" {
		return strings.TrimRight(u.String(), "/"), nil
	}
	if u.Scheme != "http" {
		return "", errors.New("API URL must use https; http is allowed only for loopback")
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return strings.TrimRight(u.String(), "/"), nil
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		return strings.TrimRight(u.String(), "/"), nil
	}
	return "", errors.New("non-loopback API URL must use https")
}

func newAPIProxy(base string) http.Handler {
	client := netguard.Client(150*time.Second, true)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
		if err != nil {
			http.Error(w, "request read failed", http.StatusBadRequest)
			return
		}
		if len(body) > 8<<20 {
			http.Error(w, "request exceeds 8 MiB console limit", http.StatusRequestEntityTooLarge)
			return
		}
		target := base + r.URL.RequestURI()
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "proxy request failed", http.StatusBadGateway)
			return
		}
		for _, name := range []string{"Authorization", "Accept", "Content-Type", "If-None-Match"} {
			if value := r.Header.Get(name); value != "" {
				req.Header.Set(name, value)
			}
		}
		req.Header.Set("User-Agent", "KINGAIBOT-Console/1.4")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		responseBody, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
		if err != nil {
			http.Error(w, "upstream response read failed", http.StatusBadGateway)
			return
		}
		if len(responseBody) > 16<<20 {
			http.Error(w, "upstream response exceeds console limit", http.StatusBadGateway)
			return
		}
		for _, name := range []string{"Content-Type", "Cache-Control", "ETag", "Retry-After"} {
			if value := resp.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(responseBody)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
