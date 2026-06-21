package debugagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Middleware returns an http.Handler that serves the debug agent UI and API.
func Middleware(config *AgentConfig) http.Handler {
	cfg := config
	if cfg == nil {
		cfg = DefaultConfig()
	}
	engine := NewDebugEngine(cfg)
	base := cfg.BasePath

	mux := http.NewServeMux()

	// Chat UI
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, renderChatPage(base))
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base+"/" || r.URL.Path == base {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, renderChatPage(base))
			return
		}
		http.NotFound(w, r)
	})

	// SSE streaming chat
	mux.HandleFunc(base+"/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Message   string `json:"message"`
			SessionID string `json:"sessionId"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		sessionID := body.SessionID
		if sessionID == "" {
			sessionID = "session-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, _ := w.(http.Flusher)
		cb := &sseCallback{w: w, flusher: flusher}

		engine.Chat(body.Message, sessionID, cb)

		if flusher != nil {
			flusher.Flush()
		}
	})

	// Clear conversation
	mux.HandleFunc(base+"/api/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			SessionID string `json:"sessionId"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.SessionID != "" {
			engine.ClearSession(body.SessionID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
	})

	// Health check
	mux.HandleFunc(base+"/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "agent": "go-debug-agent"})
	})

	// List tools
	mux.HandleFunc(base+"/api/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tools": AllSchemas()})
	})

	return mux
}

// sseCallback implements ChatCallback for SSE streaming.
type sseCallback struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (c *sseCallback) write(event string, data string) {
	fmt.Fprintf(c.w, "event: %s\ndata: %s\n\n", event, data)
	if c.flusher != nil {
		c.flusher.Flush()
	}
}

func (c *sseCallback) OnContent(chunk string) {
	encoded, _ := json.Marshal(chunk)
	c.write("content", string(encoded))
}

func (c *sseCallback) OnToolStart(toolName, args string) {
	c.write("tool_start", toolName)
}

func (c *sseCallback) OnToolResult(toolName, result string) {
	c.write("tool_result", toolName+": "+result)
}

func (c *sseCallback) OnComplete() {
	c.write("done", "")
}

func (c *sseCallback) OnError(message string) {
	c.write("error", message)
}

func (c *sseCallback) OnContextCompressed(original, compressed, removedRounds int) {
	info, _ := json.Marshal(map[string]int{
		"originalTokens":  original,
		"compressedTokens": compressed,
		"removedRounds":   removedRounds,
	})
	c.write("context_compressed", string(info))
}
