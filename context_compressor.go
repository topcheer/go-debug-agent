package debugagent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContextCompressor compresses conversation history by summarizing older rounds via LLM.
type ContextCompressor struct {
	llm               *LLMClient
	model             string
	temperature       float64
	maxContextTokens  int
	recentRoundsToKeep int
}

func NewContextCompressor(llm *LLMClient, model string, temp float64, maxTokens int) *ContextCompressor {
	return &ContextCompressor{
		llm:               llm,
		model:             model,
		temperature:       temp,
		maxContextTokens:  maxTokens,
		recentRoundsToKeep: 3,
	}
}

func (c *ContextCompressor) NeedsCompression(currentTokens int) bool {
	return currentTokens > int(float64(c.maxContextTokens)*0.75)
}

type CompressionResult struct {
	OriginalTokens  int
	CompressedTokens int
	RemovedRounds   int
	Strategy        string
}

func (c *ContextCompressor) Compress(session *ChatSession) *CompressionResult {
	originalTokens := session.GetCurrentContextTokens()
	if !c.NeedsCompression(originalTokens) {
		return nil
	}

	rounds := c.identifyRounds(session.Messages)

	keepCount := c.recentRoundsToKeep
	if keepCount >= len(rounds) {
		keepCount = len(rounds) - 1
	}
	if keepCount < 1 {
		return nil
	}

	summarizeCount := len(rounds) - keepCount

	var toSummarize, toKeep []chatMessage
	for i := 0; i < summarizeCount; i++ {
		toSummarize = append(toSummarize, rounds[i]...)
	}
	for i := summarizeCount; i < len(rounds); i++ {
		toKeep = append(toKeep, rounds[i]...)
	}

	summary, err := c.summarizeWithLLM(toSummarize)
	if err != nil {
		summary = c.fallbackTruncate(toSummarize)
	}

	header := fmt.Sprintf("[Previous conversation summary — %d rounds compressed]\n\n", summarizeCount)
	compressed := append([]chatMessage{{Role: "system", Content: header + summary}}, toKeep...)

	compressedTokens := estimateTokens(compressed)
	session.ReplaceMessages(compressed)

	return &CompressionResult{
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		RemovedRounds:    summarizeCount,
		Strategy:         fmt.Sprintf("LLM summarized %d rounds", summarizeCount),
	}
}

func (c *ContextCompressor) summarizeWithLLM(oldMessages []chatMessage) (string, error) {
	var sb strings.Builder
	for _, msg := range oldMessages {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("[User] %s\n\n", msg.Content))
		case "assistant":
			if msg.Content != "" {
				sb.WriteString(fmt.Sprintf("[Assistant] %s\n\n", msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				sb.WriteString(fmt.Sprintf("[Tool Call] %s(%s)\n\n", tc.Function.Name, tc.Function.Arguments))
			}
		case "tool":
			content := msg.Content
			if len(content) > 2000 {
				content = content[:2000] + "...[truncated]"
			}
			sb.WriteString(fmt.Sprintf("[Tool Result] %s\n\n", content))
		}
	}

	prompt := `You are a conversation summarizer for a Go debugging assistant.
Summarize the KEY diagnostic findings from the conversation below concisely.

Focus on preserving:
- Problems investigated and their root causes (if found)
- Key tool results: actual numbers, statuses, error messages, configuration values
- Recommendations or fixes already suggested
- Any unresolved issues or follow-up actions pending

Rules:
- Be concise but preserve ALL important data points
- Use bullet points
- Do NOT include full JSON dumps — extract only the meaningful values
- Keep it under 600 words`

	messages := []chatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "Conversation to summarize:\n\n" + sb.String()},
	}

	resp, err := c.llm.ChatNonStreaming(messages, nil)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}
	return "(summary unavailable)", nil
}

func (c *ContextCompressor) fallbackTruncate(messages []chatMessage) string {
	var sb strings.Builder
	sb.WriteString("Previous conversation summary (fallback):\n\n")
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content != "" {
			q := msg.Content
			if len(q) > 100 {
				q = q[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- User asked: %s\n", q))
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				sb.WriteString(fmt.Sprintf("- Called tool: %s\n", tc.Function.Name))
			}
		}
	}
	return sb.String()
}

func (c *ContextCompressor) identifyRounds(messages []chatMessage) [][]chatMessage {
	var rounds [][]chatMessage
	var current []chatMessage
	hasAssistant := false

	for _, msg := range messages {
		if msg.Role == "user" {
			if len(current) > 0 {
				rounds = append(rounds, current)
				current = nil
				hasAssistant = false
			}
			current = append(current, msg)
		} else if msg.Role == "assistant" {
			if hasAssistant {
				rounds = append(rounds, current)
				current = nil
				hasAssistant = false
			}
			current = append(current, msg)
			hasAssistant = true
		} else {
			current = append(current, msg)
		}
	}
	if len(current) > 0 {
		rounds = append(rounds, current)
	}
	return rounds
}

func estimateTokens(messages []chatMessage) int {
	chars := 0
	for _, msg := range messages {
		chars += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			chars += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return chars / 4
}

// DebugAgent JSON helper for non-streaming response
func init() {
	_ = json.Marshal // ensure import used
}
