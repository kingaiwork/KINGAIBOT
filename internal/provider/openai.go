package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
)

type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type Message struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}
type ChatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type circuit struct {
	Failures  int
	OpenUntil time.Time
}
type Client struct {
	providers []config.Provider
	timeout   time.Duration
	mu        sync.Mutex
	circuits  map[string]circuit
}

func New(providers []config.Provider, timeout time.Duration) *Client {
	return &Client{providers: providers, timeout: timeout, circuits: map[string]circuit{}}
}

func (c *Client) available(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.circuits[name]
	return st.OpenUntil.IsZero() || time.Now().After(st.OpenUntil)
}
func (c *Client) success(name string) { c.mu.Lock(); delete(c.circuits, name); c.mu.Unlock() }
func (c *Client) transientFailure(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.circuits[name]
	st.Failures++
	if st.Failures >= 3 {
		st.OpenUntil = time.Now().Add(30 * time.Second)
		st.Failures = 0
	}
	c.circuits[name] = st
}

func (c *Client) Chat(ctx context.Context, messages []Message, tools []ToolDef) (Message, string, error) {
	var errs []string
	for _, p := range c.providers {
		if !p.Enabled {
			continue
		}
		if !c.available(p.Name) {
			errs = append(errs, p.Name+": circuit open")
			continue
		}
		key := ""
		if p.APIKeyEnv != "" {
			key = os.Getenv(p.APIKeyEnv)
		}
		if p.APIKeyEnv != "" && key == "" {
			errs = append(errs, p.Name+": missing API key env "+p.APIKeyEnv)
			continue
		}
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			msg, transient, err := c.chatOne(ctx, p, key, messages, tools)
			if err == nil {
				c.success(p.Name)
				return msg, p.Name, nil
			}
			lastErr = err
			if !transient {
				break
			}
			c.transientFailure(p.Name)
			if attempt == 0 {
				select {
				case <-ctx.Done():
					return Message{}, "", ctx.Err()
				case <-time.After(250 * time.Millisecond):
				}
			}
		}
		if lastErr != nil {
			errs = append(errs, p.Name+": "+lastErr.Error())
		}
	}
	if len(errs) == 0 {
		return Message{}, "", errors.New("no enabled provider")
	}
	return Message{}, "", errors.New(strings.Join(errs, "; "))
}

func (c *Client) chatOne(ctx context.Context, p config.Provider, key string, messages []Message, tools []ToolDef) (Message, bool, error) {
	reqBody := ChatRequest{Model: p.Model, Messages: messages, Tools: tools, Temperature: 0}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, false, err
	}
	u := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return Message{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT/1.3")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := netguard.Client(c.timeout, p.AllowPrivateNetwork)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return Message{}, true, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if readErr != nil {
		return Message{}, true, readErr
	}
	if len(body) > 8<<20 {
		return Message{}, false, errors.New("provider response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 408 || resp.StatusCode >= 500
		retry := ""
		if v := resp.Header.Get("Retry-After"); v != "" {
			if sec, e := strconv.Atoi(v); e == nil && sec > 0 && sec <= 5 {
				retry = fmt.Sprintf(" retry-after=%ds", sec)
			}
		}
		return Message{}, transient, fmt.Errorf("HTTP %d%s: %s", resp.StatusCode, retry, truncate(string(body), 500))
	}
	var out ChatResponse
	if err = json.Unmarshal(body, &out); err != nil {
		return Message{}, false, fmt.Errorf("invalid response: %w", err)
	}
	if out.Error != nil {
		return Message{}, false, errors.New(out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return Message{}, false, errors.New("empty choices")
	}
	return out.Choices[0].Message, false, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
