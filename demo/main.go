package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	agent "github.com/topcheer/go-debug-agent"
)

// ─── GORM model ─────────────────────────────────────────────────────────────

type Order struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Customer  string    `json:"customer" gorm:"not null"`
	Product   string    `json:"product" gorm:"not null"`
	Quantity  int       `json:"quantity"`
	Price     float64   `json:"price"`
	Status    string    `json:"status" gorm:"default:'pending'"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderInput struct {
	Customer string  `json:"customer" binding:"required"`
	Product  string  `json:"product" binding:"required"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// ─── App config for config inspector ─────────────────────────────────────────

type AppConfig struct {
	AppName  string            `json:"app_name"`
	Port     int               `json:"port"`
	DBPath   string            `json:"db_path"`
	RedisURL string            `json:"redis_url"`
	APIKeys  map[string]string `json:"api_keys"`
}

// ─── Migration tracker ──────────────────────────────────────────────────────

type Migration struct {
	Version     string
	Description string
	SQL         string
}

// allMigrations defines the ordered list of available migrations.
var allMigrations = []Migration{
	{Version: "001", Description: "create_orders_table", SQL: `CREATE TABLE IF NOT EXISTS orders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		customer TEXT NOT NULL,
		product TEXT NOT NULL,
		quantity INTEGER DEFAULT 0,
		price REAL DEFAULT 0,
		status TEXT DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`},
	{Version: "002", Description: "add_index_orders_customer", SQL: `CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer);`},
	{Version: "003", Description: "add_index_orders_status", SQL: `CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);`},
}

// runMigrations executes pending migrations and returns the status for inspection.
func runMigrations(db *sql.DB) agent.MigrationStatus {
	// Ensure migrations table exists
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		version TEXT PRIMARY KEY,
		description TEXT,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`)

	applied := []string{}
	pending := []string{}
	history := []agent.MigrationRecord{}

	for _, m := range allMigrations {
		var existingVersion string
		err := db.QueryRow("SELECT version FROM migrations WHERE version = ?", m.Version).Scan(&existingVersion)
		if err == nil {
			// Already applied
			var appliedAt string
			db.QueryRow("SELECT applied_at FROM migrations WHERE version = ?", m.Version).Scan(&appliedAt)
			applied = append(applied, m.Version+":"+m.Description)
			history = append(history, agent.MigrationRecord{
				Version:   m.Version,
				AppliedAt: appliedAt,
				Duration:  "0s",
			})
		} else {
			// Pending — apply it
			start := time.Now()
			_, err := db.Exec(m.SQL)
			elapsed := time.Since(start)
			if err != nil {
				log.Printf("[MIGRATION] Failed to apply %s: %v", m.Version, err)
				pending = append(pending, m.Version+":"+m.Description)
				continue
			}
			_, err = db.Exec("INSERT INTO migrations (version, description) VALUES (?, ?)", m.Version, m.Description)
			if err != nil {
				log.Printf("[MIGRATION] Failed to record %s: %v", m.Version, err)
				pending = append(pending, m.Version+":"+m.Description)
				continue
			}
			applied = append(applied, m.Version+":"+m.Description)
			history = append(history, agent.MigrationRecord{
				Version:   m.Version,
				AppliedAt: time.Now().Format(time.RFC3339),
				Duration:  elapsed.String(),
			})
			log.Printf("[MIGRATION] Applied %s:%s (%s)", m.Version, m.Description, elapsed)
		}
	}

	current := "none"
	if len(applied) > 0 {
		current = applied[len(applied)-1]
	}

	return agent.MigrationStatus{
		Current: current,
		Applied: applied,
		Pending: pending,
		History: history,
	}
}

// ─── Auth config for security inspector ─────────────────────────────────────

type AuthConfig struct {
	Type         string `json:"type"`
	APIKeyName   string `json:"api_key_name"`
	APIKeyPrefix string `json:"api_key_prefix"`
	// Secret is intentionally masked by the inspector
	Secret          string `json:"-"`
	MinPasswordLen  int  `json:"min_password_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireDigit    bool `json:"require_digit"`
	ExpiryDays      int  `json:"expiry_days"`
}

// ─── Session store for security inspector ───────────────────────────────────

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionInfo
}

type SessionInfo struct {
	SessionID  string    `json:"session_id"`
	UserID     string    `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
	LastAccess time.Time `json:"last_access"`
	IP         string    `json:"ip"`
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]SessionInfo)}
}

func (s *SessionStore) Create(sessionID, userID, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = SessionInfo{
		SessionID:  sessionID,
		UserID:     userID,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		IP:         ip,
	}
}

func (s *SessionStore) List() []SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]SessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, sess)
	}
	return result
}

// ─── WebSocket upgrader ─────────────────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ─── Order mutex (for mutex inspection demo) ─────────────────────────────────

var orderMu sync.Mutex

// ─── Main ───────────────────────────────────────────────────────────────────

var startTime = time.Now()

func main() {
	// ── Connect to Redis ──
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: "",
		DB:       0,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[WARN] Redis connection failed (cache will be disabled): %v", err)
	} else {
		log.Printf("[INFO] Connected to Redis at %s", redisURL)
	}
	cancel()

	// ── Connect to SQLite via GORM ──
	db, err := gorm.Open(sqlite.Open("orders.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate
	if err := db.AutoMigrate(&Order{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Seed sample data if empty
	seedOrders(db)

	// ── Auth config and session store ──
	authCfg := AuthConfig{
		Type:             "api_key",
		APIKeyName:       "X-API-Key",
		APIKeyPrefix:     "sk-",
		Secret:           "super-secret-key-do-not-share",
		MinPasswordLen:   12,
		RequireUppercase: true,
		RequireDigit:     true,
		ExpiryDays:       90,
	}
	sessionStore := NewSessionStore()

	// Create a demo session
	sessionStore.Create("sess-demo-001", "admin", "127.0.0.1")

	// ── Register auth config and session store for security inspection ──
	agent.RegisterAuthConfig("api_key_auth", authCfg)
	agent.RegisterSessionStore("default", sessionStore.sessions)

	// ── Register health checks ──
	agent.RegisterHealthCheck("database", func() (string, map[string]any) {
		sqlDB, err := db.DB()
		if err != nil {
			return "DOWN", map[string]any{"error": err.Error()}
		}
		if err := sqlDB.Ping(); err != nil {
			return "DOWN", map[string]any{"error": err.Error()}
		}
		var count int64
		db.Model(&Order{}).Count(&count)
		return "UP", map[string]any{
			"type":   "sqlite",
			"orders": count,
		}
	})

	agent.RegisterHealthCheck("redis", func() (string, map[string]any) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			return "DOWN", map[string]any{"error": err.Error()}
		}
		return "UP", map[string]any{
			"addr": redisURL,
		}
	})

	agent.RegisterHealthCheck("disk_space", func() (string, map[string]any) {
		// Simple check — just verify the DB file exists
		if _, err := os.Stat("orders.db"); err != nil {
			return "DOWN", map[string]any{"error": "orders.db not found"}
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return "UP", map[string]any{
			"alloc_mb":    m.Alloc / 1024 / 1024,
			"uptime_s":    int(time.Since(startTime).Seconds()),
		}
	})

	// ── Register scheduled job ──
	healthJob := agent.RegisterScheduledJob("health_metrics_collector", "every 30s")
	healthJob.StartTicker(30*time.Second, func() error {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Printf("[SCHEDULER] health metrics collected — alloc=%dMB, goroutines=%d",
			m.Alloc/1024/1024, runtime.NumGoroutine())
		return nil
	})

	// ── Gin router ──
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger())

	// Custom error recovery middleware that captures panics
	router.Use(func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := agent.GetStack()
				agent.CapturePanic(r, stack)
				log.Printf("[PANIC] Recovered: %v\n%s", r, stack)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":  "Internal server error",
					"panic":  fmt.Sprintf("%v", r),
				})
			}
		}()
		c.Next()
	})

	// Custom middleware to record HTTP requests for the debug agent
	router.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		agent.RecordHTTPRequest(
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			float64(time.Since(start).Microseconds())/1000.0,
			c.ClientIP(),
		)
	})

	// ── Register framework instances for inspection ──
	agent.RegisterGinEngine(router)
	agent.RegisterRedisClient("default", rdb)
	agent.RegisterGormDB("default", db)
	agent.RegisterGormModels("default", &Order{})

	// ── v0.6.0: Enable mutex profiling for lock contention inspection ──
	runtime.SetMutexProfileFraction(1)

	// ── v0.6.0: Get raw *sql.DB and register for pool inspection ──
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("[WARN] Failed to get *sql.DB: %v", err)
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		agent.RegisterDatabase("default", sqlDB)
	}

	// ── v0.6.0: Run migrations and register migration status ──
	if sqlDB != nil {
		migrationStatus := runMigrations(sqlDB)
		agent.RegisterMigrationStatus(func() agent.MigrationStatus {
			return runMigrations(sqlDB)
		})
		log.Printf("[MIGRATION] Current version: %s, applied: %d, pending: %d",
			migrationStatus.Current, len(migrationStatus.Applied), len(migrationStatus.Pending))
	}

	// ── v0.6.0: Register app config for config inspection ──
	appConfig := AppConfig{
		AppName:  "Order Management API",
		Port:     8080,
		DBPath:   "orders.db",
		RedisURL: redisURL,
		APIKeys: map[string]string{
			"api_key":   "sk-production-key-12345",
			"admin_key": "admin-secret-token-67890",
		},
	}
	agent.RegisterConfig("app", appConfig)

	// ── v0.6.0: Register feature flags ──
	agent.RegisterFeatureFlag("new_order_ui", agent.FeatureFlag{
		Enabled: true,
		Variant: "v2",
		Reason:  "rolled_out_to_all_users",
	})
	agent.RegisterFeatureFlag("experimental_cache", agent.FeatureFlag{
		Enabled: false,
		Variant: "",
		Reason:  "in_development",
	})
	agent.RegisterFeatureFlag("ai_recommendations", agent.FeatureFlag{
		Enabled: true,
		Variant: "model_a",
		Reason:  "enabled_for_production",
	})

	// ── v0.6.0: Register order mutex for lock inspection ──
	agent.RegisterMutex("order_creation", &orderMu)

	// ── API key auth middleware ──
	apiKeyAuth := func(c *gin.Context) {
		key := c.GetHeader(authCfg.APIKeyName)
		if key == "" || key != authCfg.Secret {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or missing API key. Provide X-API-Key header.",
			})
			return
		}
		c.Next()
	}

	// ── API routes ──

	// GET /api/auth-check — returns auth info (requires API key)
	router.GET("/api/auth-check", apiKeyAuth, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"authenticated": true,
			"auth_type":     authCfg.Type,
			"timestamp":     time.Now().Format(time.RFC3339),
		})
	})

	// GET /api/panic — triggers a panic (for error tracking demo)
	router.GET("/api/panic", func(c *gin.Context) {
		log.Printf("[API] GET /api/panic — triggering panic")
		panic("intentional panic for error tracking demo")
	})

	// Orders group with optional auth (auth applied to mutating operations)
	orders := router.Group("/api/orders")

	// GET /api/orders — list all orders
	orders.GET("", func(c *gin.Context) {
		var orders []Order
		if err := db.Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[API] GET /api/orders — returning %d orders", len(orders))
		c.JSON(http.StatusOK, orders)
	})

	// POST /api/orders — create a new order (requires API key)
	orders.POST("", apiKeyAuth, func(c *gin.Context) {
		var in OrderInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Use registered mutex for order creation (demonstrates lock inspection)
		orderMu.Lock()
		defer orderMu.Unlock()

		order := Order{
			Customer:  in.Customer,
			Product:   in.Product,
			Quantity:  in.Quantity,
			Price:     in.Price,
			Status:    "pending",
			CreatedAt: time.Now(),
		}
		if err := db.Create(&order).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Invalidate cache
		rdb.Del(context.Background(), fmt.Sprintf("order:%d", order.ID))
		log.Printf("[API] POST /api/orders — created order #%d for %s", order.ID, order.Customer)
		c.JSON(http.StatusCreated, order)
	})

	// GET /api/orders/:id — get order by ID (with Redis cache)
	orders.GET("/:id", func(c *gin.Context) {
		id := c.Param("id")
		cacheKey := fmt.Sprintf("order:%s", id)

		// Try Redis cache first
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil {
			log.Printf("[API] GET /api/orders/%s — cache HIT", id)
			c.Data(http.StatusOK, "application/json", []byte(cached))
			return
		}

		// Cache miss — query database
		var order Order
		if err := db.First(&order, "id = ?", id).Error; err != nil {
			log.Printf("[API] GET /api/orders/%s — not found", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}

		// Cache the result with 60s TTL
		log.Printf("[API] GET /api/orders/%s — cache MISS, querying DB", id)
		if data, err := json.Marshal(order); err == nil {
			rdb.Set(context.Background(), cacheKey, string(data), 60*time.Second)
		}

		c.JSON(http.StatusOK, order)
	})

	// PUT /api/orders/:id — update an order (requires API key)
	orders.PUT("/:id", apiKeyAuth, func(c *gin.Context) {
		id := c.Param("id")
		var order Order
		if err := db.First(&order, "id = ?", id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		var in OrderInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		order.Customer = in.Customer
		order.Product = in.Product
		order.Quantity = in.Quantity
		order.Price = in.Price
		if err := db.Save(&order).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Invalidate cache
		rdb.Del(context.Background(), fmt.Sprintf("order:%s", id))
		log.Printf("[API] PUT /api/orders/%s — updated", id)
		c.JSON(http.StatusOK, order)
	})

	// DELETE /api/orders/:id — delete an order (requires API key)
	orders.DELETE("/:id", apiKeyAuth, func(c *gin.Context) {
		id := c.Param("id")
		result := db.Delete(&Order{}, "id = ?", id)
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		// Invalidate cache
		rdb.Del(context.Background(), fmt.Sprintf("order:%s", id))
		log.Printf("[API] DELETE /api/orders/%s — deleted", id)
		c.JSON(http.StatusOK, gin.H{"deleted": id})
	})

	// GET /api/health — health check
	router.GET("/api/health", func(c *gin.Context) {
		var count int64
		db.Model(&Order{}).Count(&count)
		c.JSON(http.StatusOK, gin.H{
			"status":   "UP",
			"orders":   count,
			"uptime_s": int(time.Since(startTime).Seconds()),
		})
	})

	// GET /api/slow — slow endpoint (500ms)
	router.GET("/api/slow", func(c *gin.Context) {
		log.Printf("[API] GET /api/slow — sleeping 500ms")
		time.Sleep(500 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"message": "This response was intentionally slow (500ms)"})
	})

	// GET /api/error — always returns 500
	router.GET("/api/error", func(c *gin.Context) {
		log.Printf("[API] GET /api/error — returning 500")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Intentional server error for testing"})
	})

	// ── WebSocket endpoint ──
	router.GET("/ws", func(c *gin.Context) {
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[WS] Upgrade error: %v", err)
			return
		}
		defer conn.Close()

		connID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		agent.RegisterWSConnection(connID, conn)
		log.Printf("[WS] New connection: %s from %s", connID, conn.RemoteAddr())

		defer agent.UnregisterWSConnection(connID)

		// Echo server
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[WS] Connection %s closed: %v", connID, err)
				break
			}
			agent.WSIncrementRecv(connID, int64(len(msg)))

			log.Printf("[WS] %s received: %s", connID, string(msg))

			if err := conn.WriteMessage(msgType, msg); err != nil {
				log.Printf("[WS] Write error: %v", err)
				break
			}
			agent.WSIncrementSent(connID, int64(len(msg)))
		}
	})

	// ── Mount debug agent ──
	agentHandler := agent.Middleware(nil)
	router.Any("/agent", func(c *gin.Context) {
		agentHandler.ServeHTTP(c.Writer, c.Request)
	})
	router.Any("/agent/*any", func(c *gin.Context) {
		agentHandler.ServeHTTP(c.Writer, c.Request)
	})

	// ── Startup banner ──
	fmt.Println()
	fmt.Println("  ┌──────────────────────────────────────────────────────────────────┐")
	fmt.Println("  │   Go Debug Agent v0.6.0 — Order Management (Gin+Redis+GORM)     │")
	fmt.Println("  └──────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  API Endpoints:")
	fmt.Println("    GET    /api/orders       — List all orders")
	fmt.Println("    POST   /api/orders       — Create order (API key required)")
	fmt.Println("    GET    /api/orders/:id   — Get order by ID (Redis cached)")
	fmt.Println("    PUT    /api/orders/:id   — Update order (API key required)")
	fmt.Println("    DELETE /api/orders/:id   — Delete order (API key required)")
	fmt.Println("    GET    /api/auth-check   — Auth check (API key required)")
	fmt.Println("    GET    /api/panic        — Triggers a panic (error tracking)")
	fmt.Println("    GET    /api/health       — Health check")
	fmt.Println("    GET    /api/slow         — Slow endpoint (500ms)")
	fmt.Println("    GET    /api/error        — Error endpoint (500)")
	fmt.Println("    GET    /ws               — WebSocket echo server")
	fmt.Println()
	fmt.Println("  Stack:")
	fmt.Printf("    Redis:  %s\n", redisURL)
	fmt.Println("    DB:     SQLite (orders.db)")
	fmt.Println("    Routes: Gin")
	fmt.Println()
	fmt.Println("  Debug Agent:")
	fmt.Println("    http://localhost:8080/agent")
	fmt.Println()
	fmt.Println("  v0.6.0 New Inspectors:")
	fmt.Println("    - Locks     (get_lock_contention, get_block_profile, detect_deadlock, get_mutex_holders)")
	fmt.Println("    - Migration (get_migration_status, get_pending_migrations, get_migration_history)")
	fmt.Println("    - Config    (get_config_snapshot, get_env_vars, get_config_diff)")
	fmt.Println("    - Flags     (get_feature_flags, evaluate_flag)")
	fmt.Println("    - Endpoint  (test_endpoint, batch_test_endpoints, get_endpoint_coverage)")
	fmt.Println("    - Pool      (get_pool_details, detect_pool_leaks, get_pool_wait_stats)")
	fmt.Println()

	var orderCount int64
	db.Model(&Order{}).Count(&orderCount)
	fmt.Printf("  Seeded %d sample orders.\n", orderCount)
	fmt.Println()
	log.Println("Server starting on :8080")

	// Graceful shutdown
	server := &http.Server{Addr: ":8080", Handler: router}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Shutdown] Received signal, shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Shutdown] Error during shutdown: %v", err)
	}
	log.Println("[Shutdown] Server stopped")
}

// ─── Seed data ──────────────────────────────────────────────────────────────

func seedOrders(db *gorm.DB) {
	var count int64
	db.Model(&Order{}).Count(&count)
	if count > 0 {
		return
	}

	samples := []OrderInput{
		{Customer: "Alice", Product: "Laptop", Quantity: 2, Price: 1299.99},
		{Customer: "Bob", Product: "Wireless Mouse", Quantity: 5, Price: 29.99},
		{Customer: "Charlie", Product: "USB-C Hub", Quantity: 3, Price: 49.99},
	}
	for _, s := range samples {
		order := Order{
			Customer:  s.Customer,
			Product:   s.Product,
			Quantity:  s.Quantity,
			Price:     s.Price,
			Status:    "pending",
			CreatedAt: time.Now(),
		}
		if err := db.Create(&order).Error; err != nil {
			log.Printf("[WARN] Failed to seed order: %v", err)
		}
	}
	log.Printf("[INFO] Seeded %d sample orders", len(samples))
}
