package debugagent

import (
	"reflect"
	"sync"
	"unsafe"
)

// ─── WaitGroup registry ─────────────────────────────────────────────────────────

var (
	waitGroupRegistry   = map[string]*sync.WaitGroup{}
	waitGroupRegistryMu sync.RWMutex
)

// RegisterWaitGroup registers a *sync.WaitGroup for inspection. The inspector
// reads the internal counter and waiter count.
func RegisterWaitGroup(name string, wg *sync.WaitGroup) {
	waitGroupRegistryMu.Lock()
	defer waitGroupRegistryMu.Unlock()
	waitGroupRegistry[name] = wg
}

func registerSyncInspector() {
	RegisterTool("get_wait_groups", "List registered sync.WaitGroup states (counter, waiters count)", nil, func(args map[string]any) (any, error) {
		waitGroupRegistryMu.RLock()
		defer waitGroupRegistryMu.RUnlock()

		if len(waitGroupRegistry) == 0 {
			return map[string]any{
				"message": "No wait groups registered. Call RegisterWaitGroup(name, wg) to enable inspection.",
				"count":   0,
			}, nil
		}

		groups := make([]map[string]any, 0, len(waitGroupRegistry))
		for name, wg := range waitGroupRegistry {
			entry := map[string]any{"name": name}
			if wg == nil {
				entry["error"] = "nil waitgroup"
			} else {
				counter, waiters := extractWaitGroupState(wg)
				entry["counter"] = counter
				entry["waiters"] = waiters
				if counter < 0 {
					entry["note"] = "Could not read internal state (Go version may differ)"
				}
			}
			groups = append(groups, entry)
		}

		return map[string]any{
			"count":  len(groups),
			"groups": groups,
		}, nil
	})
}

// extractWaitGroupState reads the internal counter and waiter count from a
// sync.WaitGroup using unsafe reflection. Returns (-1, -1) on failure.
func extractWaitGroupState(wg *sync.WaitGroup) (counter int32, waiters int32) {
	defer func() {
		if r := recover(); r != nil {
			counter = -1
			waiters = -1
		}
	}()

	rv := reflect.ValueOf(wg).Elem()
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return -1, -1
	}

	// Walk fields looking for the state field (atomic.Uint64 in Go 1.19+)
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if !f.IsValid() || !f.CanAddr() {
			continue
		}

		// Check if this is an atomic.Uint64 field
		if f.Type().PkgPath() == "sync/atomic" && f.Type().Name() == "Uint64" {
			// Read the uint64 value inside atomic.Uint64
			state := readAtomicUint64(f)
			if state >= 0 {
				return int32(state >> 32), int32(state)
			}
		}
	}

	return -1, -1
}

// readAtomicUint64 reads the underlying uint64 value from an atomic.Uint64
// reflect.Value by navigating its internal struct.
func readAtomicUint64(rv reflect.Value) uint64 {
	for j := 0; j < rv.NumField(); j++ {
		inner := rv.Field(j)
		if inner.Kind() == reflect.Uint64 && inner.CanAddr() {
			ptr := unsafe.Pointer(inner.UnsafeAddr())
			return *(*uint64)(ptr)
		}
	}
	return 0
}
