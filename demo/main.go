package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	agent "github.com/ggcode/debugagent"
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

// ─── Main ───────────────────────────────────────────────────────────────────

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

	// ── Gin router ──
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

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

	// ── API routes ──

	// GET /api/orders — list all orders
	router.GET("/api/orders", func(c *gin.Context) {
		var orders []Order
		if err := db.Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[API] GET /api/orders — returning %d orders", len(orders))
		c.JSON(http.StatusOK, orders)
	})

	// POST /api/orders — create a new order
	router.POST("/api/orders", func(c *gin.Context) {
		var in OrderInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
	router.GET("/api/orders/:id", func(c *gin.Context) {
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

	// PUT /api/orders/:id — update an order
	router.PUT("/api/orders/:id", func(c *gin.Context) {
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

	// DELETE /api/orders/:id — delete an order
	router.DELETE("/api/orders/:id", func(c *gin.Context) {
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
	fmt.Println("  ┌──────────────────────────────────────────────────────────┐")
	fmt.Println("  │     Go Debug Agent — Order Management (Gin+Redis+GORM)   │")
	fmt.Println("  └──────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  API Endpoints:")
	fmt.Println("    GET    /api/orders       — List all orders")
	fmt.Println("    POST   /api/orders       — Create a new order")
	fmt.Println("    GET    /api/orders/:id   — Get order by ID (Redis cached)")
	fmt.Println("    PUT    /api/orders/:id   — Update order")
	fmt.Println("    DELETE /api/orders/:id   — Delete order")
	fmt.Println("    GET    /api/health       — Health check")
	fmt.Println("    GET    /api/slow         — Slow endpoint (500ms)")
	fmt.Println("    GET    /api/error        — Error endpoint (500)")
	fmt.Println()
	fmt.Println("  Stack:")
	fmt.Printf("    Redis:  %s\n", redisURL)
	fmt.Println("    DB:     SQLite (orders.db)")
	fmt.Println("    Routes: Gin")
	fmt.Println()
	fmt.Println("  Debug Agent:")
	fmt.Println("    http://localhost:8080/agent")
	fmt.Println()
	fmt.Println("  Registered Inspectors:")
	fmt.Println("    - Gin routes (get_gin_routes)")
	fmt.Println("    - Redis pool/info/latency (get_redis_*)")
	fmt.Println("    - GORM stats/models (get_gorm_*)")
	fmt.Println()

	var orderCount int64
	db.Model(&Order{}).Count(&orderCount)
	fmt.Printf("  Seeded %d sample orders.\n", orderCount)
	fmt.Println()
	log.Println("Server starting on :8080")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

var startTime = time.Now()

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
