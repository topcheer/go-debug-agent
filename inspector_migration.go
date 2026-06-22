package debugagent

import (
	"sort"
	"sync"
)

// ─── Migration types ─────────────────────────────────────────────────────────

// MigrationRecord represents a single applied migration entry.
type MigrationRecord struct {
	Version   string `json:"version"`
	AppliedAt string `json:"applied_at"`
	Duration  string `json:"duration"`
}

// MigrationStatus represents the overall migration state of the application.
type MigrationStatus struct {
	Current  string            `json:"current"`
	Applied  []string          `json:"applied"`
	Pending  []string          `json:"pending"`
	History  []MigrationRecord `json:"history"`
}

var (
	migrationStatusFn func() MigrationStatus
	migrationMu       sync.RWMutex
)

// RegisterMigrationStatus registers a function that returns the current migration
// status when called by the debug agent.
func RegisterMigrationStatus(fn func() MigrationStatus) {
	migrationMu.Lock()
	defer migrationMu.Unlock()
	migrationStatusFn = fn
}

func registerMigrationInspector() {
	RegisterTool("get_migration_status", "Get current database schema migration status: current version, applied migration count, last applied migration.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		migrationMu.RLock()
		fn := migrationStatusFn
		migrationMu.RUnlock()

		if fn == nil {
			return map[string]any{
				"message": "No migration status function registered. Call debugagent.RegisterMigrationStatus(fn) to enable migration inspection.",
				"configured": false,
			}, nil
		}

		status := fn()
		result := map[string]any{
			"configured":         true,
			"current_version":    status.Current,
			"applied_count":      len(status.Applied),
			"pending_count":      len(status.Pending),
		}

		if len(status.Applied) > 0 {
			result["last_applied"] = status.Applied[len(status.Applied)-1]
		} else {
			result["last_applied"] = nil
		}
		if len(status.Pending) > 0 {
			result["has_pending"] = true
		} else {
			result["has_pending"] = false
		}

		return result, nil
	})

	RegisterTool("get_pending_migrations", "Get migrations not yet applied (version, description, file path).", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		migrationMu.RLock()
		fn := migrationStatusFn
		migrationMu.RUnlock()

		if fn == nil {
			return map[string]any{
				"message": "No migration status function registered.",
				"configured": false,
				"pending":  []string{},
			}, nil
		}

		status := fn()
		pending := make([]map[string]any, 0, len(status.Pending))
		for _, p := range status.Pending {
			entry := map[string]any{
				"version": p,
			}
			// Try to extract version + description from format "version:description"
			if idx := indexOfByte(p, ':'); idx >= 0 {
				entry["version"] = p[:idx]
				entry["description"] = p[idx+1:]
			}
			pending = append(pending, entry)
		}

		// Sort by version
		sort.Slice(pending, func(i, j int) bool {
			vi, _ := pending[i]["version"].(string)
			vj, _ := pending[j]["version"].(string)
			return vi < vj
		})

		return map[string]any{
			"configured":    true,
			"current":       status.Current,
			"pending_count": len(pending),
			"pending":       pending,
		}, nil
	})

	RegisterTool("get_migration_history", "Get applied migration history (version, applied_at, duration).", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		migrationMu.RLock()
		fn := migrationStatusFn
		migrationMu.RUnlock()

		if fn == nil {
			return map[string]any{
				"message":    "No migration status function registered.",
				"configured": false,
				"history":    []MigrationRecord{},
			}, nil
		}

		status := fn()

		// Convert history to []map[string]any for consistent JSON
		history := make([]map[string]any, 0, len(status.History))
		for _, rec := range status.History {
			history = append(history, map[string]any{
				"version":    rec.Version,
				"applied_at": rec.AppliedAt,
				"duration":   rec.Duration,
			})
		}

		return map[string]any{
			"configured": true,
			"current":    status.Current,
			"count":      len(history),
			"history":    history,
		}, nil
	})
}

// indexOfByte returns the index of the first occurrence of b in s, or -1.
func indexOfByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
