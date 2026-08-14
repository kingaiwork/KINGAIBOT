package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/cluster"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
	"github.com/kingaiwork/KINGAIBOT/internal/storage"
)

var version = "1.3.0"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(strings.ToLower(item))
		if item != "" {
			*s = append(*s, item)
		}
	}
	return nil
}

type workerClient struct {
	base       string
	token      string
	workspace  string
	allowHosts map[string]struct{}
	http       *http.Client
}

type leaseResponse struct {
	Job        cluster.Job `json:"job"`
	LeaseToken string      `json:"lease_token"`
}

func main() {
	var allowHosts stringList
	server := flag.String("server", "http://127.0.0.1:18888", "KINGAIBOT coordinator base URL")
	tokenEnv := flag.String("token-env", "KINGAIBOT_WORKER_TOKEN", "environment variable containing the worker token")
	workspace := flag.String("workspace", "./worker-data", "sandboxed worker workspace")
	poll := flag.Duration("poll", 3*time.Second, "idle lease poll interval")
	lease := flag.Int("lease-seconds", 120, "requested lease duration (30-900 seconds)")
	showVersion := flag.Bool("version", false, "print version")
	flag.Var(&allowHosts, "allow-host", "HTTPS host allowed for http.get; repeat or comma-separate")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *poll < time.Second || *poll > time.Minute {
		slog.Error("invalid poll interval", "poll", *poll)
		os.Exit(2)
	}
	if *lease < 30 || *lease > 900 {
		slog.Error("invalid lease duration", "lease_seconds", *lease)
		os.Exit(2)
	}
	token := strings.TrimSpace(os.Getenv(*tokenEnv))
	if token == "" {
		slog.Error("worker token missing", "env", *tokenEnv)
		os.Exit(2)
	}
	base, err := validateCoordinator(*server)
	if err != nil {
		slog.Error("invalid coordinator", "error", err)
		os.Exit(2)
	}
	root, err := filepath.Abs(*workspace)
	if err != nil {
		slog.Error("workspace", "error", err)
		os.Exit(2)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		slog.Error("workspace", "error", err)
		os.Exit(2)
	}
	hosts := map[string]struct{}{}
	for _, h := range allowHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || strings.ContainsAny(h, "/@?#*") {
			slog.Error("invalid allow-host", "host", h)
			os.Exit(2)
		}
		hosts[h] = struct{}{}
	}
	client := &workerClient{
		base:       base,
		token:      token,
		workspace:  root,
		allowHosts: hosts,
		http:       coordinatorHTTPClient(2 * time.Minute),
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go client.heartbeatLoop(ctx)
	slog.Info("KINGAIBOT worker started", "version", version, "coordinator", base, "workspace", root, "capabilities", []string{"system.info", "file.read", "file.write", "http.get"})
	if err := client.run(ctx, *poll, *lease); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func validateCoordinator(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.User != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("coordinator URL requires host and must not contain credentials/query/fragment")
	}
	if u.Scheme == "https" {
		return strings.TrimRight(u.String(), "/"), nil
	}
	if u.Scheme != "http" {
		return "", errors.New("coordinator URL must use https; http is allowed only for loopback")
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return strings.TrimRight(u.String(), "/"), nil
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
		return strings.TrimRight(u.String(), "/"), nil
	}
	return "", errors.New("non-loopback coordinator must use https")
}

func coordinatorHTTPClient(timeout time.Duration) *http.Client {
	client := netguard.Client(timeout, true)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func (c *workerClient) request(ctx context.Context, method, path string, body any) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT-Worker/1.3")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return resp, nil, err
	}
	if len(b) > 8<<20 {
		return resp, nil, errors.New("coordinator response exceeds 8 MiB")
	}
	return resp, b, nil
}

func (c *workerClient) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			body := map[string]any{"metadata": map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "worker_version": version}}
			resp, _, err := c.request(ctx, http.MethodPost, "/v1/cluster/worker/heartbeat", body)
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				slog.Warn("heartbeat failed", "error", err, "status", statusCode(resp))
			}
		}
	}
}

func statusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func (c *workerClient) run(ctx context.Context, poll time.Duration, leaseSeconds int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, b, err := c.request(ctx, http.MethodPost, "/v1/cluster/worker/lease", map[string]any{"lease_seconds": leaseSeconds})
		if err != nil {
			slog.Warn("lease request failed", "error", err)
			if !sleepContext(ctx, poll) {
				return ctx.Err()
			}
			continue
		}
		if resp.StatusCode == http.StatusNoContent {
			if !sleepContext(ctx, poll) {
				return ctx.Err()
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			slog.Warn("lease rejected", "status", resp.StatusCode, "body", safeSnippet(b))
			if resp.StatusCode == http.StatusUnauthorized {
				return errors.New("worker authentication rejected")
			}
			if !sleepContext(ctx, poll) {
				return ctx.Err()
			}
			continue
		}
		var leased leaseResponse
		if err := json.Unmarshal(b, &leased); err != nil || leased.Job.ID == "" || leased.LeaseToken == "" {
			slog.Error("invalid lease response", "error", err)
			continue
		}
		slog.Info("job leased", "job_id", leased.Job.ID, "kind", leased.Job.Kind, "attempt", leased.Job.Attempts, "replay_policy", leased.Job.ReplayPolicy)
		result, execErr := c.execute(ctx, leased.Job)
		completion := map[string]any{"job_id": leased.Job.ID, "lease_token": leased.LeaseToken}
		if execErr != nil {
			completion["error"] = execErr.Error()
		} else {
			completion["result"] = result
		}
		completeResp, completeBody, completeErr := c.request(ctx, http.MethodPost, "/v1/cluster/worker/complete", completion)
		if completeErr != nil || completeResp.StatusCode < 200 || completeResp.StatusCode >= 300 {
			// Never blindly retry completion forever. The coordinator deliberately
			// enters reconciliation for ambiguous side-effect outcomes.
			slog.Error("job completion not confirmed; coordinator reconciliation may be required", "job_id", leased.Job.ID, "error", completeErr, "status", statusCode(completeResp), "body", safeSnippet(completeBody))
			continue
		}
		slog.Info("job completed", "job_id", leased.Job.ID, "failed", execErr != nil)
	}
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func safeSnippet(b []byte) string {
	if len(b) > 512 {
		b = b[:512]
	}
	return strings.TrimSpace(string(b))
}

func (c *workerClient) execute(ctx context.Context, job cluster.Job) (any, error) {
	switch job.Kind {
	case "system.info":
		return map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "worker_version": version, "workspace": c.workspace}, nil
	case "file.read":
		return c.fileRead(job.Payload)
	case "file.write":
		return c.fileWrite(job.Payload)
	case "http.get":
		return c.httpGet(ctx, job.Payload)
	default:
		return nil, fmt.Errorf("unsupported worker job kind %q", job.Kind)
	}
}

func (c *workerClient) fileRead(raw json.RawMessage) (any, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	f, err := storage.OpenAllowedFile(in.Path, []string{c.workspace})
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 2<<20 {
		return nil, errors.New("file exceeds 2 MiB worker limit")
	}
	return map[string]any{"content": string(b), "bytes": len(b)}, nil
}

func (c *workerClient) fileWrite(raw json.RawMessage) (any, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if len(in.Content) > 2<<20 {
		return nil, errors.New("content exceeds 2 MiB worker limit")
	}
	if err := storage.AtomicWriteAllowedFile(in.Path, []string{c.workspace}, []byte(in.Content), 0o600); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "bytes": len(in.Content)}, nil
}

func (c *workerClient) httpGet(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return nil, errors.New("worker http.get requires an HTTPS URL without embedded credentials")
	}
	host := strings.ToLower(u.Hostname())
	if _, ok := c.allowHosts[host]; !ok {
		return nil, fmt.Errorf("host %q is not in worker allow-host list", host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "KINGAIBOT-Worker/1.3")
	client := netguard.Client(60*time.Second, false)
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if next.URL.Scheme != "https" || strings.ToLower(next.URL.Hostname()) != host {
			return errors.New("cross-host or protocol-downgrade redirect blocked")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 2<<20 {
		return nil, errors.New("HTTP response exceeds 2 MiB worker limit")
	}
	headerNames := make([]string, 0, len(resp.Header))
	for name := range resp.Header {
		name = http.CanonicalHeaderKey(name)
		switch name {
		case "Content-Type", "Content-Length", "Last-Modified", "ETag":
			headerNames = append(headerNames, name)
		}
	}
	sort.Strings(headerNames)
	headers := map[string]string{}
	for _, name := range headerNames {
		headers[name] = resp.Header.Get(name)
	}
	return map[string]any{"status": resp.StatusCode, "headers": headers, "body": string(b)}, nil
}
