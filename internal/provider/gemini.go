package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/netguard"
)

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
	ID   string         `json:"id,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
	ID       string         `json:"id,omitempty"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	GenerationConfig  map[string]any  `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func geminiEndpoint(base, model string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, ":generateContent") {
		return base
	}
	if strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v1beta") {
		return base + "/models/" + url.PathEscape(model) + ":generateContent"
	}
	return base + "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
}

func geminiToolNames(messages []Message) map[string]string {
	out := map[string]string{}
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			if call.ID != "" && call.Function.Name != "" {
				out[call.ID] = call.Function.Name
			}
		}
	}
	return out
}

func geminiCallID(name string, args map[string]any, index int) string {
	b, _ := json.Marshal(args)
	h := sha256.Sum256(append(append([]byte(fmt.Sprintf("%d\x00%s\x00", index, name)), b...), 0))
	return "gem_" + hex.EncodeToString(h[:12])
}

func geminiInputMessages(messages []Message) (*geminiContent, []geminiContent, error) {
	var systemParts []geminiPart
	contents := make([]geminiContent, 0, len(messages))
	toolNames := geminiToolNames(messages)
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != nil && strings.TrimSpace(*msg.Content) != "" {
				systemParts = append(systemParts, geminiPart{Text: *msg.Content})
			}
		case "user":
			text := ""
			if msg.Content != nil {
				text = *msg.Content
			}
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: text}}})
		case "assistant":
			parts := make([]geminiPart, 0, len(msg.ToolCalls)+1)
			if msg.Content != nil && strings.TrimSpace(*msg.Content) != "" {
				parts = append(parts, geminiPart{Text: *msg.Content})
			}
			for _, call := range msg.ToolCalls {
				if call.ID == "" || call.Function.Name == "" {
					return nil, nil, errors.New("gemini conversion: invalid tool call")
				}
				var args map[string]any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
					return nil, nil, fmt.Errorf("gemini conversion: invalid tool arguments: %w", err)
				}
				if args == nil {
					args = map[string]any{}
				}
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{Name: call.Function.Name, Args: args, ID: call.ID}})
			}
			if len(parts) > 0 {
				contents = append(contents, geminiContent{Role: "model", Parts: parts})
			}
		case "tool":
			if msg.ToolCallID == "" {
				return nil, nil, errors.New("gemini conversion: tool result missing tool_call_id")
			}
			name := toolNames[msg.ToolCallID]
			if name == "" {
				return nil, nil, errors.New("gemini conversion: tool result has unknown tool_call_id")
			}
			result := ""
			if msg.Content != nil {
				result = *msg.Content
			}
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{FunctionResponse: &geminiFunctionResponse{Name: name, ID: msg.ToolCallID, Response: map[string]any{"result": result}}}}})
		default:
			return nil, nil, fmt.Errorf("gemini conversion: unsupported message role %q", msg.Role)
		}
	}
	var system *geminiContent
	if len(systemParts) > 0 {
		system = &geminiContent{Parts: systemParts}
	}
	return system, contents, nil
}

func geminiTools(tools []ToolDef) []geminiTool {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		if tool.Function.Name == "" {
			continue
		}
		schema := tool.Function.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		decls = append(decls, geminiFunctionDeclaration{Name: tool.Function.Name, Description: tool.Function.Description, Parameters: schema})
	}
	if len(decls) == 0 {
		return nil
	}
	return []geminiTool{{FunctionDeclarations: decls}}
}

func (c *Client) chatGemini(ctx context.Context, p config.Provider, key string, messages []Message, tools []ToolDef) (Message, bool, error) {
	system, converted, err := geminiInputMessages(messages)
	if err != nil {
		return Message{}, false, err
	}
	body, err := json.Marshal(geminiRequest{SystemInstruction: system, Contents: converted, Tools: geminiTools(tools), GenerationConfig: map[string]any{"temperature": 0}})
	if err != nil {
		return Message{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiEndpoint(p.BaseURL, p.Model), bytes.NewReader(body))
	if err != nil {
		return Message{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KINGAIBOT/1.3")
	if key != "" {
		req.Header.Set("x-goog-api-key", key)
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
		return Message{}, false, errors.New("gemini response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500
		return Message{}, transient, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, truncate(string(responseBody), 500))
	}
	var out geminiResponse
	if err := json.Unmarshal(responseBody, &out); err != nil {
		return Message{}, false, fmt.Errorf("invalid gemini response: %w", err)
	}
	if out.Error != nil {
		return Message{}, false, errors.New(out.Error.Message)
	}
	if len(out.Candidates) == 0 {
		return Message{}, false, errors.New("gemini returned no candidates")
	}
	msg := Message{Role: "assistant"}
	var texts []string
	for index, part := range out.Candidates[0].Content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
		if part.FunctionCall != nil {
			fc := part.FunctionCall
			if fc.Name == "" {
				return Message{}, false, errors.New("gemini returned functionCall without name")
			}
			args := fc.Args
			if args == nil {
				args = map[string]any{}
			}
			argBytes, err := json.Marshal(args)
			if err != nil {
				return Message{}, false, err
			}
			id := fc.ID
			if id == "" {
				id = geminiCallID(fc.Name, args, index)
			}
			call := ToolCall{ID: id, Type: "function"}
			call.Function.Name = fc.Name
			call.Function.Arguments = string(argBytes)
			msg.ToolCalls = append(msg.ToolCalls, call)
		}
	}
	if len(texts) > 0 {
		text := strings.Join(texts, "\n")
		msg.Content = &text
	}
	if msg.Content == nil && len(msg.ToolCalls) == 0 {
		return Message{}, false, errors.New("gemini returned no text or function calls")
	}
	return msg, false, nil
}
