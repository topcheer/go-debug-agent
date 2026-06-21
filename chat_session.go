package debugagent

import "time"

// ChatSession manages conversation history and cumulative token usage.
type ChatSession struct {
	SessionID                string
	Messages                 []chatMessage
	CreatedAt                time.Time
	LastActiveAt             time.Time
	LastTokenUsage           *tokenUsage
	CumulativePromptTokens   int
	CumulativeCompletionTokens int
}

type tokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewChatSession(sessionID string) *ChatSession {
	now := time.Now()
	return &ChatSession{
		SessionID:    sessionID,
		Messages:     []chatMessage{},
		CreatedAt:    now,
		LastActiveAt: now,
	}
}

func (s *ChatSession) AddMessage(msg chatMessage) {
	s.Messages = append(s.Messages, msg)
	s.LastActiveAt = time.Now()
}

func (s *ChatSession) ReplaceMessages(msgs []chatMessage) {
	s.Messages = msgs
	s.LastActiveAt = time.Now()
}

func (s *ChatSession) RecordTokenUsage(usage *tokenUsage) {
	if usage == nil {
		return
	}
	s.LastTokenUsage = usage
	s.CumulativePromptTokens = usage.PromptTokens
	s.CumulativeCompletionTokens += usage.CompletionTokens
}

func (s *ChatSession) GetCurrentContextTokens() int {
	return s.CumulativePromptTokens
}

func (s *ChatSession) Clear() {
	s.Messages = []chatMessage{}
	s.LastTokenUsage = nil
	s.CumulativePromptTokens = 0
	s.CumulativeCompletionTokens = 0
	s.LastActiveAt = time.Now()
}
