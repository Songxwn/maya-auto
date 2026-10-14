package newapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func Resolve(base, suffix string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" {
		return ""
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	if strings.HasSuffix(b, "/v1") {
		return b + suffix
	}
	return b + "/v1" + suffix
}

type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"ownedBy"`
}

type ModelsResult struct {
	Models    []Model `json:"models"`
	LatencyMS int64   `json:"latencyMs"`
	Status    int     `json:"status"`
}

func (c *Client) ListModels(ctx context.Context) (ModelsResult, error) {
	var out ModelsResult
	url := Resolve(c.BaseURL, "/models")
	if url == "" {
		return out, fmt.Errorf("未填写 NewAPI 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, err
	}
	c.decorate(req)
	start := time.Now()
	resp, err := c.http().Do(req)
	out.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	out.Status = resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("拉取模型失败（HTTP %d）: %s", resp.StatusCode, trim(string(body), 400))
	}
	out.Models, err = parseModels(body)
	return out, err
}

func parseModels(body []byte) ([]Model, error) {
	var wrapped struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err == nil && wrapped.Data != nil {
		return uniqueModels(wrapped.Data), nil
	}
	var bare []struct {
		ID      string `json:"id"`
		OwnedBy string `json:"owned_by"`
	}
	if err := json.Unmarshal(body, &bare); err != nil {
		return nil, fmt.Errorf("无法解析模型列表: %s", trim(string(body), 240))
	}
	return uniqueModels(bare), nil
}

func uniqueModels(in []struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}) []Model {
	seen := map[string]bool{}
	var out []Model
	for _, item := range in {
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, Model{ID: item.ID, OwnedBy: item.OwnedBy})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type ProbeResult struct {
	Model     string `json:"model"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latencyMs"`
	Reply     string `json:"reply"`
	Error     string `json:"error,omitempty"`
	Class     string `json:"class"`
}

func (c *Client) Probe(ctx context.Context, model string) ProbeResult {
	res := ProbeResult{Model: model, Class: "unreachable"}
	if strings.TrimSpace(model) == "" {
		res.Class = "bad_request"
		res.Error = "模型名为空"
		return res
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with the single word OK"},
		},
		"max_tokens":  16,
		"temperature": 0,
	}
	status, raw, latency, err := c.chatRaw(ctx, body)
	res.Status = status
	res.LatencyMS = latency
	if err != nil && status == 0 {
		res.Error = err.Error()
		if ctx.Err() != nil {
			res.Class = "timeout"
		}
		return res
	}
	if status == 400 && (strings.Contains(string(raw), "max_tokens") || strings.Contains(string(raw), "temperature")) {
		delete(body, "temperature")
		delete(body, "max_tokens")
		body["max_completion_tokens"] = 16
		status, raw, latency, err = c.chatRaw(ctx, body)
		res.Status = status
		res.LatencyMS += latency
	}
	res.Class = classify(status, raw)
	if status < 200 || status >= 300 {
		res.Error = trim(extractAPIError(raw), 500)
		if res.Error == "" && err != nil {
			res.Error = err.Error()
		}
		return res
	}
	reply := extractReply(raw)
	res.Reply = trim(reply, 240)
	res.OK = true
	res.Class = "ok"
	return res
}

type ChatMessage struct {
	Role       string          `json:"role"`
	Content    FlexString      `json:"content,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || len(b) == 0 {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(b)
	return nil
}

func (f FlexString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(f))
}

type ChatResult struct {
	Content   string
	ToolCalls []ToolCall
	UsageText string
}

func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage, tools []map[string]any) (ChatResult, error) {
	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	status, raw, _, err := c.chatRaw(ctx, payload)
	if err != nil && status == 0 {
		return ChatResult{}, err
	}
	if status < 200 || status >= 300 {
		return ChatResult{}, fmt.Errorf("NewAPI HTTP %d: %s", status, trim(extractAPIError(raw), 600))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   FlexString `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResult{}, fmt.Errorf("无法解析对话响应: %s", trim(string(raw), 300))
	}
	if len(parsed.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("NewAPI 没有返回 choices")
	}
	msg := parsed.Choices[0].Message
	return ChatResult{
		Content:   string(msg.Content),
		ToolCalls: msg.ToolCalls,
		UsageText: fmt.Sprintf("prompt %d / completion %d", parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens),
	}, nil
}

func (c *Client) chatRaw(ctx context.Context, payload any) (int, []byte, int64, error) {
	url := Resolve(c.BaseURL, "/chat/completions")
	if url == "" {
		return 0, nil, 0, fmt.Errorf("未填写 NewAPI 地址")
	}
	if c.APIKey == "" {
		return 0, nil, 0, fmt.Errorf("未填写 NewAPI 密钥")
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return 0, nil, 0, err
	}
	c.decorate(req)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := c.http().Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return 0, nil, latency, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, body, latency, nil
}

func (c *Client) decorate(req *http.Request) {
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("User-Agent", "maya-auto")
}

func classify(status int, body []byte) string {
	text := strings.ToLower(string(body))
	switch {
	case status == 401 || status == 403:
		return "auth"
	case status == 404 || strings.Contains(text, "model_not_found") || strings.Contains(text, "no such model"):
		return "not_found"
	case status == 429 || strings.Contains(text, "rate limit"):
		return "rate_limit"
	case status == 408 || status == 504:
		return "timeout"
	case status >= 400:
		return "bad_request"
	default:
		return "ok"
	}
}

func extractAPIError(body []byte) string {
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &wrapped) == nil {
		if wrapped.Error.Message != "" {
			return wrapped.Error.Message
		}
		if wrapped.Message != "" {
			return wrapped.Message
		}
	}
	return string(body)
}

func extractReply(body []byte) string {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content FlexString `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &parsed) != nil || len(parsed.Choices) == 0 {
		return ""
	}
	return string(parsed.Choices[0].Message.Content)
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func ParseToolArgs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		if strings.TrimSpace(s) == "" {
			return map[string]any{}, nil
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(s), &args); err != nil {
			return nil, err
		}
		if args == nil {
			args = map[string]any{}
		}
		return args, nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}
