// Package aichat AI 聊天后端 HTTP 客户端
package aichat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AIChatClient PowerBot AI Backend 的 HTTP 客户端
type AIChatClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAIChatClient 创建客户端，backendURL 例如 "http://localhost:8000"
func NewAIChatClient(backendURL string) *AIChatClient {
	return &AIChatClient{
		baseURL: strings.TrimRight(backendURL, "/"),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ChatRequest 对应 Python 后端的 ChatRequest
type ChatRequest struct {
	SessionID            string   `json:"session_id"`
	UserID               string   `json:"user_id"`
	UserName             string   `json:"user_name"`
	Message              string   `json:"message"`
	Images               []string `json:"images,omitempty"`
	SystemPromptOverride *string  `json:"system_prompt_override,omitempty"`
	UseRAG               bool     `json:"use_rag"`
	Stream               bool     `json:"stream"`
}

// ChatResponse 对应 Python 后端的 ChatResponse
type ChatResponse struct {
	Reply       string      `json:"reply"`
	ToolCalls   []ToolCall  `json:"tool_calls"`
	Sources     []Source    `json:"sources"`
	FinishReason *string    `json:"finish_reason"`
	Usage       *UsageInfo  `json:"usage"`
	Error       *ErrorDetail `json:"error,omitempty"`
}

// ToolCall Agent 工具调用
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	CallID    string         `json:"call_id"`
}

// Source RAG 引用来源
type Source struct {
	DocumentID string  `json:"document_id"`
	ChunkID    string  `json:"chunk_id"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
}

// UsageInfo token 用量
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorDetail API 错误
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SessionConfig 会话配置，对应 Python 后端
type SessionConfig struct {
	Temperature      float64 `json:"temperature"`
	ReplyProbability int     `json:"reply_probability"`
	SystemPromptID   *string `json:"system_prompt_id"`
	ModelBackendID   *string `json:"model_backend_id"`
	UseAgent         bool    `json:"use_agent"`
	MaxTokens        int     `json:"max_tokens"`
	TopP             float64 `json:"top_p"`
	UseRAG           bool    `json:"use_rag"`
}

// Chat 发送聊天请求到 Python 后端
func (c *AIChatClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		logrus.Warnln("[aichat] backend returned", resp.StatusCode, ":", string(respBody))
		return nil, fmt.Errorf("backend error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("backend error: [%s] %s", chatResp.Error.Code, chatResp.Error.Message)
	}

	return &chatResp, nil
}

// UpdateConfig 同步会话配置到 Python 后端
func (c *AIChatClient) UpdateConfig(ctx context.Context, sessionID string, cfg *SessionConfig) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PUT",
		c.baseURL+"/api/v1/sessions/"+sessionID+"/config", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http put: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		logrus.Warnln("[aichat] config sync failed", resp.StatusCode, ":", string(respBody))
		return fmt.Errorf("backend error: status=%d", resp.StatusCode)
	}

	return nil
}
