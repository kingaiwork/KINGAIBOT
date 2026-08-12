package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	base := flag.String("url", env("KINGAGENT_URL", "http://127.0.0.1:18888"), "server URL")
	token := flag.String("token", os.Getenv("KINGAGENT_ADMIN_TOKEN"), "admin bearer token")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	baseURL, err := validateBaseURL(*base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	client := &apiClient{base: baseURL, token: *token, http: &http.Client{Timeout: 130 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	err = nil
	switch args[0] {
	case "health":
		err = client.print("GET", "/healthz", nil)
	case "run":
		if len(args) < 2 {
			err = fmt.Errorf("usage: kingagent run <prompt>")
			break
		}
		err = client.run(strings.Join(args[1:], " "))
	case "tasks":
		err = client.print("GET", "/v1/tasks", nil)
	case "task":
		if len(args) != 2 {
			err = fmt.Errorf("usage: kingagent task <id>")
			break
		}
		err = client.print("GET", "/v1/tasks/"+url.PathEscape(args[1]), nil)
	case "approvals":
		err = client.print("GET", "/v1/approvals", nil)
	case "approve", "deny":
		if len(args) != 2 {
			err = fmt.Errorf("usage: kingagent %s <approval-id>", args[0])
			break
		}
		status := "approved"
		if args[0] == "deny" {
			status = "denied"
		}
		err = client.print("POST", "/v1/approvals/"+url.PathEscape(args[1]), map[string]any{"status": status})
	case "evolution":
		err = client.print("GET", "/v1/evolution/proposals", nil)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`KINGAIBOT CLI

Commands:
  health
  run <prompt>
  tasks
  task <id>
  approvals
  approve <approval-id>
  deny <approval-id>
  evolution

Environment:
  KINGAGENT_URL
  KINGAGENT_ADMIN_TOKEN`)
}

type apiClient struct {
	base, token string
	http        *http.Client
}

func (c *apiClient) request(method, path string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 8<<20 {
		return nil, fmt.Errorf("server response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}
func (c *apiClient) print(method, path string, body any) error {
	b, err := c.request(method, path, body)
	if err != nil {
		return err
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Print(string(b))
	}
	return nil
}
func (c *apiClient) run(prompt string) error {
	b, err := c.request("POST", "/v1/tasks", map[string]any{"input": prompt})
	if err != nil {
		return err
	}
	var t struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(b, &t); err != nil {
		return err
	}
	if t.ID == "" {
		return fmt.Errorf("server returned no task id")
	}
	fmt.Println("task:", t.ID)
	for i := 0; i < 1200; i++ {
		b, err = c.request("GET", "/v1/tasks/"+t.ID, nil)
		if err != nil {
			return err
		}
		var cur struct {
			Status  string `json:"status"`
			Output  string `json:"output"`
			Error   string `json:"error"`
			Pending string `json:"pending_approval"`
		}
		if err = json.Unmarshal(b, &cur); err != nil {
			return err
		}
		switch cur.Status {
		case "completed":
			fmt.Println(cur.Output)
			return nil
		case "failed", "canceled":
			return fmt.Errorf("%s: %s", cur.Status, cur.Error)
		case "waiting_approval":
			fmt.Printf("waiting approval: %s\n", cur.Pending)
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("task polling timeout")
}
func validateBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("server URL requires a hostname and must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("server URL must not contain query or fragment")
	}
	if u.Scheme != "https" {
		h := u.Hostname()
		ip := net.ParseIP(h)
		if u.Scheme != "http" || !(strings.EqualFold(h, "localhost") || (ip != nil && ip.IsLoopback())) {
			return "", fmt.Errorf("remote KINGAIBOT URL must use https; http is allowed only on loopback")
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
