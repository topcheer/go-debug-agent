package debugagent

import (
	"reflect"
	"sync"
	"time"
)

// ─── WebSocket registry ─────────────────────────────────────────────────────

// WSConnInfo holds metadata about an active WebSocket connection.
type WSConnInfo struct {
	ID            string    `json:"id"`
	RemoteAddress string    `json:"remote_address"`
	ConnectedSince time.Time `json:"connected_since"`
	MessagesSent   int64     `json:"messages_sent"`
	MessagesRecv   int64     `json:"messages_received"`
	BytesSent      int64     `json:"bytes_sent"`
	BytesRecv      int64     `json:"bytes_received"`
	Room           string    `json:"room,omitempty"`
}

// WSGlobal tracks aggregate WebSocket statistics.
type WSGlobal struct {
	TotalConnections int64 `json:"total_connections"`
	ActiveConnections int64 `json:"active_connections"`
	TotalSent         int64 `json:"total_messages_sent"`
	TotalRecv         int64 `json:"total_messages_received"`
	TotalBytesSent    int64 `json:"total_bytes_sent"`
	TotalBytesRecv    int64 `json:"total_bytes_received"`
}

// WSRoom tracks a WebSocket room/channel for pub/sub patterns.
type WSRoom struct {
	Name        string   `json:"name"`
	Subscribers int      `json:"subscribers"`
	Members     []string `json:"members"`
}

var (
	wsConnections = map[string]WSConnInfo{}
	wsRooms       = map[string]*WSRoom{}
	wsGlobal      = WSGlobal{}
	wsMu          sync.RWMutex
)

// RegisterWSConnection registers a WebSocket connection for inspection.
func RegisterWSConnection(id string, conn any) {
	wsMu.Lock()
	defer wsMu.Unlock()

	info := WSConnInfo{
		ID:             id,
		ConnectedSince: time.Now(),
	}

	// Try to extract remote address via reflection
	if remote := extractWSRemoteAddr(conn); remote != "" {
		info.RemoteAddress = remote
	}

	wsConnections[id] = info
	wsGlobal.TotalConnections++
	wsGlobal.ActiveConnections = int64(len(wsConnections))
}

// UnregisterWSConnection removes a WebSocket connection from tracking.
func UnregisterWSConnection(id string) {
	wsMu.Lock()
	defer wsMu.Unlock()
	delete(wsConnections, id)
	wsGlobal.ActiveConnections = int64(len(wsConnections))

	// Remove from any rooms
	for _, room := range wsRooms {
		newMembers := make([]string, 0, len(room.Members))
		for _, m := range room.Members {
			if m != id {
				newMembers = append(newMembers, m)
			}
		}
		room.Members = newMembers
		room.Subscribers = len(room.Members)
	}
}

// WSIncrementSent updates counters for a sent message.
func WSIncrementSent(id string, bytes int64) {
	wsMu.Lock()
	defer wsMu.Unlock()
	if conn, ok := wsConnections[id]; ok {
		conn.MessagesSent++
		conn.BytesSent += bytes
		wsConnections[id] = conn
	}
	wsGlobal.TotalSent++
	wsGlobal.TotalBytesSent += bytes
}

// WSIncrementRecv updates counters for a received message.
func WSIncrementRecv(id string, bytes int64) {
	wsMu.Lock()
	defer wsMu.Unlock()
	if conn, ok := wsConnections[id]; ok {
		conn.MessagesRecv++
		conn.BytesRecv += bytes
		wsConnections[id] = conn
	}
	wsGlobal.TotalRecv++
	wsGlobal.TotalBytesRecv += bytes
}

// WSJoinRoom adds a connection to a room.
func WSJoinRoom(roomName, connID string) {
	wsMu.Lock()
	defer wsMu.Unlock()
	if _, ok := wsRooms[roomName]; !ok {
		wsRooms[roomName] = &WSRoom{Name: roomName, Members: []string{}}
	}
	room := wsRooms[roomName]
	// Check if already a member
	for _, m := range room.Members {
		if m == connID {
			return
		}
	}
	room.Members = append(room.Members, connID)
	room.Subscribers = len(room.Members)

	if conn, ok := wsConnections[connID]; ok {
		conn.Room = roomName
		wsConnections[connID] = conn
	}
}

// extractWSRemoteAddr tries to get the remote address from a WebSocket connection.
func extractWSRemoteAddr(conn any) string {
	defer func() { recover() }()
	v := reflect.ValueOf(conn)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	// Try RemoteAddr() method
	method := reflect.ValueOf(conn).MethodByName("RemoteAddr")
	if method.IsValid() {
		results := method.Call(nil)
		if len(results) > 0 && results[0].CanInterface() {
			if s, ok := results[0].Interface().(string); ok {
				return s
			}
			if addr, ok := results[0].Interface().(interface{ String() string }); ok {
				return addr.String()
			}
		}
	}
	return ""
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerWebSocketInspector() {
	RegisterTool("get_ws_connections", "List active WebSocket connections (session ID, remote address, connected since, messages sent/received)", nil, func(args map[string]any) (any, error) {
		wsMu.RLock()
		defer wsMu.RUnlock()

		if len(wsConnections) == 0 {
			return map[string]any{
				"message": "No WebSocket connections registered. Call debugagent.RegisterWSConnection(id, conn) to enable WebSocket inspection.",
				"count":   0,
			}, nil
		}

		conns := make([]map[string]any, 0, len(wsConnections))
		for id, info := range wsConnections {
			entry := map[string]any{
				"id":               id,
				"remote_address":   info.RemoteAddress,
				"connected_since":  info.ConnectedSince.Format(time.RFC3339),
				"duration":         time.Since(info.ConnectedSince).Round(time.Second).String(),
				"messages_sent":    info.MessagesSent,
				"messages_recv":    info.MessagesRecv,
				"bytes_sent":       info.BytesSent,
				"bytes_recv":       info.BytesRecv,
			}
			if info.Room != "" {
				entry["room"] = info.Room
			}
			conns = append(conns, entry)
		}

		return map[string]any{
			"count":       len(conns),
			"connections": conns,
		}, nil
	})

	RegisterTool("get_ws_stats", "Get WebSocket statistics (total connections, active, messages sent, messages received, avg message size)", nil, func(args map[string]any) (any, error) {
		wsMu.RLock()
		defer wsMu.RUnlock()

		result := map[string]any{
			"total_connections":   wsGlobal.TotalConnections,
			"active_connections":  wsGlobal.ActiveConnections,
			"total_messages_sent": wsGlobal.TotalSent,
			"total_messages_recv": wsGlobal.TotalRecv,
			"total_bytes_sent":    wsGlobal.TotalBytesSent,
			"total_bytes_recv":    wsGlobal.TotalBytesRecv,
		}

		// Compute average message sizes
		totalMsgs := wsGlobal.TotalSent + wsGlobal.TotalRecv
		totalBytes := wsGlobal.TotalBytesSent + wsGlobal.TotalBytesRecv
		if totalMsgs > 0 {
			result["avg_message_size_bytes"] = totalBytes / totalMsgs
		} else {
			result["avg_message_size_bytes"] = int64(0)
		}

		// Per-connection averages
		if len(wsConnections) > 0 {
			var totalSent, totalRecv int64
			for _, conn := range wsConnections {
				totalSent += conn.MessagesSent
				totalRecv += conn.MessagesRecv
			}
			result["avg_sent_per_conn"] = totalSent / int64(len(wsConnections))
			result["avg_recv_per_conn"] = totalRecv / int64(len(wsConnections))
		}

		return result, nil
	})

	RegisterTool("get_ws_rooms", "List WebSocket rooms/channels with subscriber counts (for pub/sub patterns)", nil, func(args map[string]any) (any, error) {
		wsMu.RLock()
		defer wsMu.RUnlock()

		if len(wsRooms) == 0 {
			return map[string]any{
				"message": "No WebSocket rooms registered. Call debugagent.WSJoinRoom(roomName, connID) to enable room inspection.",
				"count":   0,
			}, nil
		}

		rooms := make([]map[string]any, 0, len(wsRooms))
		for name, room := range wsRooms {
			entry := map[string]any{
				"name":        name,
				"subscribers": room.Subscribers,
				"members":     room.Members,
			}
			rooms = append(rooms, entry)
		}

		return map[string]any{
			"count": len(rooms),
			"rooms": rooms,
		}, nil
	})
}
