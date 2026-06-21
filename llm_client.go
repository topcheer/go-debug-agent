package debugagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// LLMClient is an OpenAI-compatible chat client with streaming and retry.
type LLMClient struct {
	cfg    LLMConfig
	client *http.Client
}

func NewLLMClient(cfg LLMConfig) *LLMClient {
	return &LLMClient{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model         string             `json:"model"`
	Messages      []chatMessage      `json:"messages"`
	Temperature   float64            `json:"temperature"`
	MaxTokens     int                `json:"max_tokens"`
	Stream        bool               `json:"stream,omitempty"`
	StreamOptions *streamOptions     `json:"stream_options,omitempty"`
	Tools         []map[string]any   `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *tokenUsage `json:"usage,omitempty"`
}

// ChatCallback is the engine→UI streaming callback interface.
type ChatCallback interface {
	OnContent(chunk string)
	OnToolStart(toolName, args string)
	OnToolResult(toolName, result string)
	OnComplete()
	OnError(message string)
	OnContextCompressed(original, compressed, removedRounds int)
}

// StreamHandler is the low-level LLM stream callback (engine implements this).
type StreamHandler interface {
	OnContent(chunk string)
	OnStreamComplete(toolCalls []toolCall, finishReason string, usage *tokenUsage)
	OnStreamError(err error)
}

// ChatNonStreaming sends a non-streaming chat completion (for context compression).
func (c *LLMClient) ChatNonStreaming(messages []chatMessage, tools []map[string]any) (*chatResponse, error) {
	body := chatRequest{
		Model:       c.cfg.Model,
		Messages:    messages,
		Temperature: 0,
		MaxTokens:   1024,
	}
	if tools != nil {
		body.Tools = tools
	}
	return c.postWithRetry("/chat/completions", body)
}

// ChatStream sends a streaming chat completion, calling handler for each chunk.
func (c *LLMClient) ChatStream(messages []chatMessage, tools []map[string]any, toolChoice any, handler StreamHandler) {
	body := chatRequest{
		Model:         c.cfg.Model,
		Messages:      messages,
		Temperature:   c.cfg.Temperature,
		MaxTokens:     c.cfg.MaxTokens,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if tools != nil {
		body.Tools = tools
	}
	if toolChoice != nil {
		body.ToolChoice = toolChoice
	}

	maxRetries := c.cfg.MaxRetries
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.streamRequest("/chat/completions", body, handler)
		if err == nil {
			return
		}
		if isRetriableErr(err) && attempt < maxRetries {
			delay := c.calculateDelay(attempt)
			time.Sleep(delay)
			continue
		}
		handler.OnStreamError(err)
		return
	}
	handler.OnStreamError(fmt.Errorf("exhausted retries after %d attempts", maxRetries))
}

func (c *LLMClient) streamRequest(path string, body chatRequest, handler StreamHandler) error {
	jsonData, _ := json.Marshal(body)

	url := c.cfg.BaseURL + path
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return &retriableErr{statusCode: resp.StatusCode, msg: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	toolCallMap := map[int]*toolCall{}
	var finishReason string
	var usage *tokenUsage

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			continue
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string     `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *tokenUsage `json:"usage"`
		}

		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil && chunk.Usage.PromptTokens > 0 {
			usage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		if delta.Content != "" {
			handler.OnContent(delta.Content)
		}

		for _, tc := range delta.ToolCalls {
			if _, ok := toolCallMap[tc.Index]; !ok {
				toolCallMap[tc.Index] = &toolCall{
					Type:     "function",
					Function: toolFunction{},
				}
			}
			entry := toolCallMap[tc.Index]
			if tc.ID != "" {
				entry.ID = tc.ID
			}
			if tc.Function.Name != "" {
				entry.Function.Name += tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				entry.Function.Arguments += tc.Function.Arguments
			}
		}

		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
	}

	// Collect tool calls sorted by index
	var toolCalls []toolCall
	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok && tc.Function.Name != "" {
			toolCalls = append(toolCalls, *tc)
		}
	}

	_ = finishReason
	_ = usage
	handler.OnStreamComplete(toolCalls, finishReason, usage)
	return nil
}

// postWithRetry sends a non-streaming POST with retry logic.
func (c *LLMClient) postWithRetry(path string, body chatRequest) (*chatResponse, error) {
	maxRetries := c.cfg.MaxRetries
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := c.post(path, body)
		if err == nil {
			return result, nil
		}
		if isRetriableErr(err) && attempt < maxRetries {
			delay := c.calculateDelay(attempt)
			time.Sleep(delay)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("exhausted retries")
}

func (c *LLMClient) post(path string, body chatRequest) (*chatResponse, error) {
	jsonData, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", c.cfg.BaseURL+path, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, &retriableErr{statusCode: resp.StatusCode, msg: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}
	return &result, nil
}

type retriableErr struct {
	statusCode int
	msg        string
}

func (e *retriableErr) Error() string { return e.msg }

func isRetriableErr(err error) bool {
	if re, ok := err.(*retriableErr); ok {
		return isRetriableStatus(re.statusCode)
	}
	// Network errors (timeout, connection refused, etc.) are retriable
	return true
}

func isRetriableStatus(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 503 || code == 504
}

func (c *LLMClient) calculateDelay(attempt int) time.Duration {
	base := c.cfg.RetryBaseDelayMs * (1 << attempt)
	jitter := rand.Intn(base/2 + 1)
	delay := base + jitter
	maxDelay := c.cfg.RetryMaxDelayMs
	if delay > maxDelay {
		delay = maxDelay
	}
	return time.Duration(delay) * time.Millisecond
}
