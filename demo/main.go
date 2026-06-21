package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	agent "github.com/ggcode/debugagent"
)

// ─── Order model ──────────────────────────────────────────────────────────

type Order struct {
	ID       int     `json:"id"`
	Customer string  `json:"customer"`
	Item     string  `json:"item"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
	Total    float64 `json:"total"`
	Status   string  `json:"status"`
	Created  string  `json:"created_at"`
}

type OrderInput struct {
	Customer string  `json:"customer"`
	Item     string  `json:"item"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// ─── Thread-safe in-memory store ──────────────────────────────────────────

type OrderStore struct {
	mu     sync.Mutex
	orders map[int]*Order
	nextID int
}

func NewOrderStore() *OrderStore {
	s := &OrderStore{
		orders: make(map[int]*Order),
		nextID: 1,
	}
	s.seed()
	return s
}

func (s *OrderStore) seed() {
	samples := []OrderInput{
		{Customer: "Alice", Item: "Laptop", Quantity: 2, Price: 1299.99},
		{Customer: "Bob", Item: "Wireless Mouse", Quantity: 5, Price: 29.99},
		{Customer: "Charlie", Item: "USB-C Hub", Quantity: 3, Price: 49.99},
	}
	for _, in := range samples {
		s.create(in)
	}
}

func (s *OrderStore) create(in OrderInput) *Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	o := &Order{
		ID:       id,
		Customer: in.Customer,
		Item:     in.Item,
		Quantity: in.Quantity,
		Price:    in.Price,
		Total:    float64(in.Quantity) * in.Price,
		Status:   "pending",
		Created:  time.Now().Format(time.RFC3339),
	}
	s.orders[id] = o
	return o
}

func (s *OrderStore) get(id int) (*Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	return o, ok
}

func (s *OrderStore) list() []*Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*Order, 0, len(s.orders))
	for _, o := range s.orders {
		list = append(list, o)
	}
	return list
}

func (s *OrderStore) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[id]; !ok {
		return false
	}
	delete(s.orders, id)
	return true
}

func (s *OrderStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.orders)
}

// ─── HTTP response writer wrapper ──────────────────────────────────────────

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ─── Main ──────────────────────────────────────────────────────────────────

func main() {
	store := NewOrderStore()

	mux := http.NewServeMux()

	// ── CRUD endpoints ──
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			orders := store.list()
			log.Printf("[API] GET /api/orders — returning %d orders", len(orders))
			writeJSON(w, http.StatusOK, orders)

		case http.MethodPost:
			var in OrderInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				log.Printf("[API] POST /api/orders — bad request: %v", err)
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
				return
			}
			if in.Customer == "" || in.Item == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "customer and item are required"})
				return
			}
			o := store.create(in)
			log.Printf("[API] POST /api/orders — created order #%d for %s", o.ID, o.Customer)
			writeJSON(w, http.StatusCreated, o)

		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		}
	})

	mux.HandleFunc("/api/orders/", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Path[len("/api/orders/"):]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid order ID"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			o, ok := store.get(id)
			if !ok {
				log.Printf("[API] GET /api/orders/%d — not found", id)
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found"})
				return
			}
			log.Printf("[API] GET /api/orders/%d — found", id)
			writeJSON(w, http.StatusOK, o)

		case http.MethodDelete:
			if !store.delete(id) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Order not found"})
				return
			}
			log.Printf("[API] DELETE /api/orders/%d — deleted", id)
			writeJSON(w, http.StatusOK, map[string]any{"deleted": id})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		}
	})

	// ── Health check ──
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "UP",
			"orders":   store.count(),
			"uptime_s": int(time.Since(time.Now()).Seconds()),
		})
	})

	// ── Slow endpoint (simulates latency for request tracking demos) ──
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] GET /api/slow — sleeping 500ms")
		time.Sleep(500 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]string{"message": "This response was intentionally slow (500ms)"})
	})

	// ── Error endpoint (always returns 500) ──
	mux.HandleFunc("/api/error", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] GET /api/error — returning 500")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Intentional server error for testing"})
	})

	// ── Debug Agent: mount at /agent ──
	mux.Handle("/agent", agent.Middleware(nil))
	mux.Handle("/agent/", agent.Middleware(nil))

	// ── Wrap everything with request tracking middleware ──
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: 200}
		mux.ServeHTTP(wrapped, r)
		agent.RecordHTTPRequest(
			r.Method,
			r.URL.Path,
			wrapped.status,
			float64(time.Since(start).Microseconds())/1000.0,
			r.RemoteAddr,
		)
	})

	fmt.Println()
	fmt.Println("  ┌──────────────────────────────────────────────────┐")
	fmt.Println("  │         Go Debug Agent — Order Management        │")
	fmt.Println("  └──────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  API Endpoints:")
	fmt.Println("    GET    /api/orders       — List all orders")
	fmt.Println("    POST   /api/orders       — Create a new order")
	fmt.Println("    GET    /api/orders/{id}  — Get order by ID")
	fmt.Println("    DELETE /api/orders/{id}  — Delete order by ID")
	fmt.Println("    GET    /api/health       — Health check")
	fmt.Println("    GET    /api/slow         — Slow endpoint (500ms)")
	fmt.Println("    GET    /api/error        — Error endpoint (500)")
	fmt.Println()
	fmt.Println("  Debug Agent:")
	fmt.Println("    http://localhost:8080/agent")
	fmt.Println()
	fmt.Printf("  Seeded %d sample orders.\n", store.count())
	fmt.Println()
	log.Println("Server starting on :8080")

	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
