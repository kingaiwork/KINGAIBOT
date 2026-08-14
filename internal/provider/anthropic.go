package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
)

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicResponse struct {
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func anthropicInputMessages(messages []Message) (string, []anthropicMessage, error) {
	var system []string
	out := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != nil && strings.TrimSpace(*msg.Content) != "" {
				system = append(system, *msg.Content)
			}
		case "user":
			text := ""
			if msg.Content != nil {
				text = *msg.Content
			}
			out = append(out, anthropicMessage{Role: "user", Content: text})
		case "assistant":
			if len(msg.ToolCalls) == 0 {
				text := ""
				if msg.Content != nil {
					text = *msg.Content
				}
				out = append(out, anthropicMessage{Role: "assistant", Content: text})
				continue
			}
			blocks := make([]anthropicBlock, 0, len(msg.ToolCalls)+1)
			if msg.Content != nil && strings.TrimSpace(*msg.Content) != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: *msg.Content})
			}
			for _, call := range msg.ToolCalls {
				if call.ID == "" || call.Function.Name == "" {
					return "", nil, errors.New("anthropic conversion: invalid tool call")
				}
				input := json.RawMessage(call.Function.Arguments)
				if !json.Valid(input) {
					return "", nil, errors.New("anthropic conversion: invalid tool arguments")
				}
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: input})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: blocks})
		case "tool":
			if msg.ToolCallID == "" {
				return "", nil, errors.New("anthropic conversion: tool result missing tool_call_id")
			}
			content := ""
			if msg.Content != nil {
				content = *msg.Content
			}
			out = append(out, anthropicMessage{Role: "user", Content: []anthropicBlock{{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: content}}})
		default:
			return "", nil, fmt.Errorf("anthropic conversion: unsupported message role %q", msg.Role)
		}
	}
	return strings.Join(system, "\n\n"), out, nil
}

func anthropicTools(tools []ToolDef) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Function.Name == "" {
			continue
		}
		schema := tool.Function.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, anthropicTool{Name: tool.Function.Name, Description: tool.Function.Description, InputSchema: schema})
	}
	return out
}

func anthropicEndpoint(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/messages"
	}
	return base + "/v1/messages"
}

func (c *Client) chatAnthropic(ctx context.Context, p config.Provider, key string, messages []Message, tools []ToolDef) (Message, bool, error) {
	system, converted, err := anthropicInputMessages(messages)
	if err != nil {
		return Message{}, false, err
	}
	body, err := json.Marshal(anthropicRequest{Model: p.Model, MaxTokens: 8192, System: system, Messages: converted, Tools: anthropicTools(tools)})
	if err != nil {
		return Message{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicEndpoint(p.BaseURL), bytes.NewReader(body))
	if err != nil {
		return Message{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT/1.3")
	req.Header.Set("anthropic-version", "2023-06-01")
	if key != "" {
		req.Header.Set("x-api-key", key)
	}
	client := netguard.Client(c.timeout, p.AllowPrivateNetwork)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return Message{}, true, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return Message{}, true, err
	}
	if len(responseBody) > 8<<20 {
		return Message{}, false, errors.New("anthropic response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500
		return Message{}, transient, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncate(string(responseBody), 500))
	}
	var out anthropicResponse
	if err := json.Unmarshal(responseBody, &out); err != nil {
		return Message{}, false, fmt.Errorf("invalid anthropic response: %w", err)
	}
	if out.Error != nil {
		return Message{}, false, errors.New(out.Error.Message)
	}
	var texts []string
	msg := Message{Role: "assistant"}
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			if block.ID == "" || block.Name == "" || !json.Valid(block.Input) {
				return Message{}, false, errors.New("anthropic returned invalid tool_use block")
			}
			call := ToolCall{ID: block.ID, Type: "function"}
			call.Function.Name = block.Name
			call.Function.Arguments = string(block.Input)
			msg.ToolCalls = append(msg.ToolCalls, call)
		}
	}
	if len(texts) > 0 {
		text := strings.Join(texts, "\n")
		msg.Content = &text
	}
	if msg.Content == nil && len(msg.ToolCalls) == 0 {
		return Message{}, false, errors.New("anthropic returned no text or client tool calls")
	}
	return msg, false, nil
}
