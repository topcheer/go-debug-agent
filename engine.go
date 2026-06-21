package debugagent

import (
	"encoding/json"
	"fmt"
	"sync"
)

// DebugEngine orchestrates the LLM ↔ Tool reasoning loop.
type DebugEngine struct {
	config           *AgentConfig
	llm              *LLMClient
	sessions         sync.Map // map[string]*ChatSession
	promptBuilder    *SystemPromptBuilder
	systemPrompt     string
	contextCompressor *ContextCompressor
}

var (
	engineOnce sync.Once
)

// NewDebugEngine creates a new engine with the given config.
func NewDebugEngine(config *AgentConfig) *DebugEngine {
	initInspectors()
	engine := &DebugEngine{
		config:        config,
		llm:           NewLLMClient(config.LLM),
		promptBuilder: NewSystemPromptBuilder(),
	}
	engine.systemPrompt = engine.promptBuilder.Build()
	engine.contextCompressor = NewContextCompressor(
		engine.llm, config.LLM.Model, config.LLM.Temperature, config.LLM.ContextWindowTokens,
	)
	return engine
}

func (e *DebugEngine) getOrCreateSession(sessionID string) *ChatSession {
	if sessionID == "" {
		sessionID = "default"
	}
	val, ok := e.sessions.Load(sessionID)
	if ok {
		return val.(*ChatSession)
	}
	session := NewChatSession(sessionID)
	e.sessions.Store(sessionID, session)
	return session
}

// Chat processes a user message with streaming via the callback.
func (e *DebugEngine) Chat(userMessage string, sessionID string, cb ChatCallback) {
	session := e.getOrCreateSession(sessionID)
	session.AddMessage(chatMessage{Role: "user", Content: userMessage})
	e.runToolLoop(session, cb)
}

// ClearSession clears a session's history.
func (e *DebugEngine) ClearSession(sessionID string) {
	if session, ok := e.sessions.Load(sessionID); ok {
		session.(*ChatSession).Clear()
	}
}

func (e *DebugEngine) runToolLoop(session *ChatSession, cb ChatCallback) {
	maxRounds := e.config.LLM.MaxToolRounds
	emptyRetried := false // guard against infinite empty-response loop

	for round := 0; round < maxRounds; round++ {
		// Context compression check
		if round > 0 && e.contextCompressor.NeedsCompression(session.GetCurrentContextTokens()) {
			result := e.contextCompressor.Compress(session)
			if result != nil {
				cb.OnContent(fmt.Sprintf("\n\n> [Context auto-compressed: %d → ~%d tokens (%s)]\n\n",
					result.OriginalTokens, result.CompressedTokens, result.Strategy))
				cb.OnContextCompressed(result.OriginalTokens, result.CompressedTokens, result.RemovedRounds)
			}
		}

		// Build messages with system prompt
		messages := make([]chatMessage, 0, len(session.Messages)+1)
		messages = append(messages, chatMessage{Role: "system", Content: e.systemPrompt})
		messages = append(messages, session.Messages...)

		// Stream the response
		var contentBuilder string
		var toolCallHolder []toolCall
		var usageHolder *tokenUsage
		hadError := false

		var wg sync.WaitGroup
		wg.Add(1)

		handler := &engineStreamHandler{
			onContent: func(chunk string) {
				contentBuilder += chunk
				cb.OnContent(chunk)
			},
			onComplete: func(toolCalls []toolCall, finishReason string, usage *tokenUsage) {
				toolCallHolder = toolCalls
				usageHolder = usage
				wg.Done()
			},
			onError: func(err error) {
				hadError = true
				cb.OnError(fmt.Sprintf("LLM API error: %v", err))
				wg.Done()
			},
		}

		toolChoice := "auto"
		e.llm.ChatStream(messages, AllSchemas(), toolChoice, handler)
		wg.Wait()

		if hadError {
			return
		}

		// Record token usage
		if usageHolder != nil {
			session.RecordTokenUsage(usageHolder)
		}

		if len(toolCallHolder) == 0 {
			// If LLM returned empty content after tool calls, prompt it to summarize once
			if contentBuilder == "" && !emptyRetried && round > 0 {
				emptyRetried = true
				session.AddMessage(chatMessage{Role: "assistant", Content: ""})
				session.AddMessage(chatMessage{
					Role: "system",
					Content: "You just completed diagnostic tool calls but returned no analysis. " +
						"Based on the tool results above, please provide a clear summary " +
						"of your findings and actionable recommendations.",
				})
				continue
			}
			// Final answer
			session.AddMessage(chatMessage{Role: "assistant", Content: contentBuilder})
			cb.OnComplete()
			return
		}

		// Reset empty-retry guard — new tool calls mean a new round sequence
		emptyRetried = false

		// Execute tool calls
		assistantMsg := chatMessage{Role: "assistant", Content: contentBuilder, ToolCalls: toolCallHolder}
		session.AddMessage(assistantMsg)

		for _, tc := range toolCallHolder {
			toolName := tc.Function.Name
			args := map[string]any{}
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}

			cb.OnToolStart(toolName, tc.Function.Arguments)

			result := ExecuteTool(toolName, args)
			resultStr, _ := json.Marshal(result)
			resultStrTrunc := string(resultStr)
			if len(resultStrTrunc) > 12000 {
				resultStrTrunc = resultStrTrunc[:12000]
			}

			cb.OnToolResult(toolName, resultStrTrunc)
			session.AddMessage(chatMessage{Role: "tool", ToolCallID: tc.ID, Content: resultStrTrunc})
		}
	}

	// Max rounds — force final summary
	finalMessages := make([]chatMessage, 0, len(session.Messages)+2)
	finalMessages = append(finalMessages, chatMessage{Role: "system", Content: e.systemPrompt})
	finalMessages = append(finalMessages, session.Messages...)
	finalMessages = append(finalMessages, chatMessage{
		Role: "system",
		Content: "You have reached the maximum number of tool-calling rounds. " +
			"Based on all the diagnostic data you have gathered so far, " +
			"provide a comprehensive analysis and actionable recommendations NOW. " +
			"Do not attempt to call more tools.",
	})

	var wg sync.WaitGroup
	wg.Add(1)
	handler := &engineStreamHandler{
		onContent: func(chunk string) { cb.OnContent(chunk) },
		onComplete: func(toolCalls []toolCall, finishReason string, usage *tokenUsage) {
			if usage != nil {
				session.RecordTokenUsage(usage)
			}
			wg.Done()
		},
		onError: func(err error) {
			cb.OnContent("\n\n*I've gathered diagnostic data from multiple tools but reached the analysis limit.*")
			wg.Done()
		},
	}
	e.llm.ChatStream(finalMessages, []map[string]any{}, "none", handler)
	wg.Wait()
	cb.OnComplete()
}

// engineStreamHandler implements StreamHandler for the engine.
type engineStreamHandler struct {
	onContent   func(chunk string)
	onComplete  func(toolCalls []toolCall, finishReason string, usage *tokenUsage)
	onError     func(err error)
}

func (h *engineStreamHandler) OnContent(chunk string)          { h.onContent(chunk) }
func (h *engineStreamHandler) OnStreamComplete(toolCalls []toolCall, finishReason string, usage *tokenUsage) {
	h.onComplete(toolCalls, finishReason, usage)
}
func (h *engineStreamHandler) OnStreamError(err error) { h.onError(err) }
