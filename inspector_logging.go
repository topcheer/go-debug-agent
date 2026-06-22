package debugagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ─── Log ring buffer ─────────────────────────────────────────────────────────

// LogEntry represents a single captured log entry.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Source    string         `json:"source"`
	Fields    map[string]any `json:"fields,omitempty"`
}

var (
	logBuffer     []LogEntry
	logBufferMu   sync.Mutex
	logBufferMax  = 100
)

// LogCapture appends a log entry to the ring buffer. Demos and instrumentation
// code call this to feed logs into the debug agent.
func LogCapture(entry LogEntry) {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	logBufferMu.Lock()
	defer logBufferMu.Unlock()

	logBuffer = append(logBuffer, entry)
	if len(logBuffer) > logBufferMax {
		logBuffer = logBuffer[len(logBuffer)-logBufferMax:]
	}
}

func logBufferSnapshot() []LogEntry {
	logBufferMu.Lock()
	defer logBufferMu.Unlock()
	result := make([]LogEntry, len(logBuffer))
	copy(result, logBuffer)
	return result
}

// ─── Logger registry ──────────────────────────────────────────────────────────

type loggerInfo struct {
	logger any
	level  string
	mu     sync.Mutex
}

var (
	loggerRegistry   = map[string]*loggerInfo{}
	loggerRegistryMu sync.RWMutex
)

// RegisterLogger registers a logger (slog.Logger, zap.Logger, zerolog.Logger, or
// any custom logger) for inspection by the debug agent.
func RegisterLogger(name string, logger any) {
	loggerRegistryMu.Lock()
	defer loggerRegistryMu.Unlock()
	loggerRegistry[name] = &loggerInfo{logger: logger, level: "info"}
}

func registerLoggingInspector() {
	RegisterTool("get_log_buffer", "Return recent log entries from the built-in ring buffer (last 100 entries)", map[string]ToolParam{
		"limit":   {Type: "integer", Description: "Max number of entries to return (default 100)", Required: false},
		"level":   {Type: "string", Description: "Filter by log level (debug/info/warn/error)", Required: false},
		"source":  {Type: "string", Description: "Filter by source name", Required: false},
	}, func(args map[string]any) (any, error) {
		entries := logBufferSnapshot()

		limit := len(entries)
		if v, ok := args["limit"].(float64); ok && int(v) > 0 && int(v) < limit {
			limit = int(v)
		}

		levelFilter, _ := args["level"].(string)
		sourceFilter, _ := args["source"].(string)

		var filtered []LogEntry
		for _, e := range entries {
			if levelFilter != "" && e.Level != strings.ToLower(levelFilter) {
				continue
			}
			if sourceFilter != "" && e.Source != sourceFilter {
				continue
			}
			filtered = append(filtered, e)
		}
		if len(filtered) > limit {
			filtered = filtered[len(filtered)-limit:]
		}

		return map[string]any{
			"total_in_buffer": len(logBuffer),
			"returned":        len(filtered),
			"entries":         filtered,
		}, nil
	})

	RegisterTool("get_log_level", "Get current log level for all registered loggers (slog, zap, zerolog, etc.)", nil, func(args map[string]any) (any, error) {
		loggerRegistryMu.RLock()
		defer loggerRegistryMu.RUnlock()

		if len(loggerRegistry) == 0 {
			return map[string]any{
				"message": "No loggers registered. Call RegisterLogger(name, logger) to enable log inspection.",
				"count":   0,
			}, nil
		}

		loggers := make([]map[string]any, 0, len(loggerRegistry))
		for name, info := range loggerRegistry {
			loggers = append(loggers, map[string]any{
				"name":  name,
				"level": getLoggerLevel(info),
				"type":  reflectTypeName(info.logger),
			})
		}

		return map[string]any{
			"count":   len(loggers),
			"loggers": loggers,
		}, nil
	})

	RegisterTool("set_log_level", "Dynamically set the log level for a registered logger", map[string]ToolParam{
		"logger_name": {Type: "string", Description: "Name of the registered logger", Required: true},
		"level":       {Type: "string", Description: "Log level: debug, info, warn, or error", Required: true},
	}, func(args map[string]any) (any, error) {
		name, _ := args["logger_name"].(string)
		level, _ := args["level"].(string)

		if name == "" {
			return nil, fmt.Errorf("logger_name is required")
		}
		level = strings.ToLower(strings.TrimSpace(level))
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "warning": true, "error": true}
		if !validLevels[level] {
			return nil, fmt.Errorf("invalid level %q: must be debug, info, warn, or error", level)
		}
		if level == "warning" {
			level = "warn"
		}

		loggerRegistryMu.RLock()
		info, ok := loggerRegistry[name]
		loggerRegistryMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("logger %q not registered", name)
		}

		err := setLoggerLevel(info, level)
		if err != nil {
			return map[string]any{"name": name, "level": level, "warning": err.Error()}, nil
		}
		return map[string]any{"name": name, "level": level, "status": "updated"}, nil
	})

	RegisterTool("register_logger", "Register a logger for runtime inspection (note: loggers are typically registered programmatically via RegisterLogger)", map[string]ToolParam{
		"name": {Type: "string", Description: "Name to identify this logger", Required: true},
	}, func(args map[string]any) (any, error) {
		return map[string]any{
			"message": "Use RegisterLogger(name, logger) in your Go code to register a logger for inspection",
			"example": `debugagent.RegisterLogger("myapp", slog.Default())`,
		}, nil
	})
}

// ─── slog handler wrapper ─────────────────────────────────────────────────────

// CaptureSlogHandler wraps an inner slog.Handler and captures all log records
// into the agent ring buffer. Use it to auto-capture slog output.
//
// 	h := debugagent.NewCaptureSlogHandler(slog.NewTextHandler(os.Stderr, nil), "myapp")
// 	slog.SetDefault(slog.New(h))
type CaptureSlogHandler struct {
	inner  slog.Handler
	source string
}

// NewCaptureSlogHandler creates a slog.Handler that captures records into the
// ring buffer while forwarding to the optional inner handler.
func NewCaptureSlogHandler(inner slog.Handler, source string) *CaptureSlogHandler {
	return &CaptureSlogHandler{inner: inner, source: source}
}

func (h *CaptureSlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.inner != nil {
		return h.inner.Enabled(ctx, level)
	}
	return true
}

func (h *CaptureSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})
	LogCapture(LogEntry{
		Timestamp: r.Time.Format(time.RFC3339Nano),
		Level:     r.Level.String(),
		Message:   r.Message,
		Source:    h.source,
		Fields:    fields,
	})
	if h.inner != nil {
		return h.inner.Handle(ctx, r)
	}
	return nil
}

func (h *CaptureSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var inner slog.Handler
	if h.inner != nil {
		inner = h.inner.WithAttrs(attrs)
	}
	return &CaptureSlogHandler{inner: inner, source: h.source}
}

func (h *CaptureSlogHandler) WithGroup(name string) slog.Handler {
	var inner slog.Handler
	if h.inner != nil {
		inner = h.inner.WithGroup(name)
	}
	return &CaptureSlogHandler{inner: inner, source: h.source}
}

// ─── io.Writer hook ───────────────────────────────────────────────────────────

// CaptureWriter wraps an io.Writer and captures each complete line into the log
// ring buffer. Works with zap, zerolog, log, or any logger that writes to an
// io.Writer.
//
// 	w := debugagent.NewCaptureWriter(os.Stderr, "myapp")
// 	logger := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(w), level))
type CaptureWriter struct {
	inner   io.Writer
	source  string
	mu      sync.Mutex
	partial []byte
}

// NewCaptureWriter creates a writer hook that captures each line.
func NewCaptureWriter(inner io.Writer, source string) *CaptureWriter {
	return &CaptureWriter{inner: inner, source: source}
}

func (w *CaptureWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.mu.Lock()
	w.partial = append(w.partial, p...)
	for {
		idx := bytes.IndexByte(w.partial, '\n')
		if idx < 0 {
			break
		}
		line := string(w.partial[:idx])
		w.partial = w.partial[idx+1:]
		level, msg := parseLogLevel(line)
		LogCapture(LogEntry{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Level:     level,
			Message:   msg,
			Source:    w.source,
		})
	}
	w.mu.Unlock()
	if w.inner != nil {
		return w.inner.Write(p)
	}
	return n, nil
}

// parseLogLevel tries to extract the level and message from a log line.
func parseLogLevel(line string) (level, msg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "info", ""
	}

	// Try JSON
	var jsonEntry map[string]any
	if err := json.Unmarshal([]byte(line), &jsonEntry); err == nil {
		if l, ok := jsonEntry["level"].(string); ok {
			level = strings.ToLower(l)
		} else {
			level = "info"
		}
		if m, ok := jsonEntry["msg"].(string); ok {
			msg = m
		} else if m, ok := jsonEntry["message"].(string); ok {
			msg = m
		} else {
			msg = line
		}
		if level == "warning" {
			level = "warn"
		}
		return level, msg
	}

	// Heuristic: scan for common level keywords
	upper := strings.ToUpper(line)
	for _, l := range []string{"DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "PANIC", "TRACE"} {
		if strings.Contains(upper, l) {
			level = strings.ToLower(l)
			if level == "warning" {
				level = "warn"
			}
			break
		}
	}
	if level == "" {
		level = "info"
	}
	msg = line
	return level, msg
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func getLoggerLevel(info *loggerInfo) string {
	info.mu.Lock()
	defer info.mu.Unlock()

	// Try reflection: look for Level() method
	level := reflectCallStringMethod(info.logger, "Level")
	if level != "" {
		return strings.ToLower(level)
	}
	level = reflectCallStringMethod(info.logger, "GetLevel")
	if level != "" {
		return strings.ToLower(level)
	}

	if info.level != "" {
		return info.level
	}
	return "unknown"
}

func setLoggerLevel(info *loggerInfo, level string) error {
	info.mu.Lock()
	defer info.mu.Unlock()

	// Try reflection: call SetLevel or similar
	ok := reflectCallSetLevel(info.logger, level)
	if !ok {
		ok = reflectCallSetLevel(info.logger, strings.ToUpper(level))
	}

	// Always store the requested level
	info.level = level

	if !ok {
		return fmt.Errorf("logger type %T does not expose a SetLevel method; level stored in registry only", info.logger)
	}
	return nil
}
