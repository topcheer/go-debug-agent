package debugagent

import (
	"database/sql"
	"sync"
)

var (
	dbRegistry   []*sql.DB
	dbNames      []string
	dbRegistryMu sync.Mutex
)

// RegisterDB registers a *sql.DB for inspection by the debug agent.
// Call this after opening a database connection to enable database tools.
func RegisterDB(name string, db *sql.DB) {
	dbRegistryMu.Lock()
	defer dbRegistryMu.Unlock()
	dbRegistry = append(dbRegistry, db)
	dbNames = append(dbNames, name)
}

func registerDatabaseInspector() {
	RegisterTool("get_db_connections", "List registered database connection pools with stats (open, idle, in-use, max connections)", nil, func(args map[string]any) (any, error) {
		dbRegistryMu.Lock()
		defer dbRegistryMu.Unlock()

		if len(dbRegistry) == 0 {
			return map[string]any{
				"message": "No databases registered. Call debugagent.RegisterDB(name, db) to enable database inspection.",
				"count":   0,
			}, nil
		}

		connections := make([]map[string]any, 0, len(dbRegistry))
		for i, db := range dbRegistry {
			stats := db.Stats()
			connections = append(connections, map[string]any{
				"name":                dbNames[i],
				"open_connections":    stats.OpenConnections,
				"in_use":             stats.InUse,
				"idle":               stats.Idle,
				"max_open":            stats.MaxOpenConnections,
				"wait_count":          stats.WaitCount,
				"max_lifetime_closed": stats.MaxLifetimeClosed,
				"max_idle_closed":     stats.MaxIdleClosed,
			})
		}

		return map[string]any{
			"count":       len(dbRegistry),
			"connections": connections,
		}, nil
	})

	RegisterTool("get_db_stats", "Get detailed DBStats for all registered databases (wait count/duration, max idle/open, closed connections)", nil, func(args map[string]any) (any, error) {
		dbRegistryMu.Lock()
		defer dbRegistryMu.Unlock()

		if len(dbRegistry) == 0 {
			return map[string]any{
				"message": "No databases registered.",
				"count":   0,
			}, nil
		}

		allStats := make([]map[string]any, 0, len(dbRegistry))
		for i, db := range dbRegistry {
			stats := db.Stats()
			allStats = append(allStats, map[string]any{
				"name":                 dbNames[i],
				"max_open_connections": stats.MaxOpenConnections,
				"open_connections":     stats.OpenConnections,
				"in_use":               stats.InUse,
				"idle":                 stats.Idle,
				"wait_count":           stats.WaitCount,
				"wait_duration":        stats.WaitDuration.String(),
				"max_idle_closed":      stats.MaxIdleClosed,
				"max_idle_time_closed": stats.MaxIdleTimeClosed,
				"max_lifetime_closed":  stats.MaxLifetimeClosed,
			})
		}

		return map[string]any{
			"count": len(dbRegistry),
			"stats": allStats,
		}, nil
	})
}
