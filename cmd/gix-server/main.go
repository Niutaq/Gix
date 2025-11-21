// cmd/gix-server/main.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Niutaq/Gix/pkg/scrapers"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Constants
const (
	// Content-Type headers
	contentTypeText = "text/plain"
	contentTypeJSON = "application/json"

	// Error messages
	internalServerError = "Internal server error"
)

// AppState struct holds all app-wide components like DB pools
type AppState struct {
	DB    *pgxpool.Pool
	Cache *redis.Client
}

// CantorInfo - an internal struct to hold data from the 'cantors' table
type CantorInfo struct {
	ID          int
	DisplayName string
	BaseURL     string
	Strategy    string // C1, C2, C3, etc.
	Units       int
}

// CantorListResponse - a response struct for the /api/v1/cantors endpoint
type CantorListResponse struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
}

// RatesResponse - a response struct for the /api/v1/rates endpoint
type RatesResponse struct {
	BuyRate  string `json:"buyRate"`
	SellRate string `json:"sellRate"`
	CantorID int    `json:"cantorID"`
	Currency string `json:"currency"`
}

// rateParams - a struct for holding parameters from the URL
type rateParams struct {
	CantorID int
	Currency string
}

// processedRates - a struct for holding the final rates after processing
type processedRates struct {
	Buy  float64
	Sell float64
}

// ++++++++++++++++++++ MAIN Function ++++++++++++++++++++
func main() {
	log.Println("Launching Gix server...")

	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")

	if databaseURL == "" || redisURL == "" {
		log.Fatal("Can't start server. Missing DATABASE_URL or REDIS_URL environment variables.")
	}

	dbpool, err := connectToDB(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Can't connect to database: %v\n", err)
	}
	defer dbpool.Close()
	log.Println("Successfully connected to database.")

	if err := initSchema(ctx, dbpool); err != nil {
		log.Fatalf("Can't initialize database schema: %v\n", err)
	}

	rdb, err := connectToRedis(ctx, redisURL)
	if err != nil {
		log.Fatalf("Can't connect to caching client: %v\n", err)
	}
	log.Println("Successfully connected to Redis.")

	appState := &AppState{
		DB:    dbpool,
		Cache: rdb,
	}

	// Gin middleware
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Next()
		//c.Writer.Header().Set(contentTypeText, contentTypeJSON)
	})

	// Endpoints
	r.GET("/healthz", handleHealthCheck(appState))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/cantors", handleCantorsList(appState))
		v1.GET("/rates", handleGetRates(appState))
	}

	log.Println("Gin API listens at port :8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// Grupowanie API v1

// --- HTTP handlers ---
// handleHealthCheck - returns 200 OK if DB and Redis are up
func handleHealthCheck(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := app.DB.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "service": "database"})
			return
		}
		if _, err := app.Cache.Ping(c.Request.Context()).Result(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "service": "cache"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Gix is alive with Gin."})
	}
}

// handleCantorsList - returns a list of all cantors
func handleCantorsList(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := app.DB.Query(c.Request.Context(), "SELECT id, display_name, name FROM cantors")
		if err != nil {
			log.Printf("DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		defer rows.Close()

		var cantors []CantorListResponse
		for rows.Next() {
			var cr CantorListResponse
			if err := rows.Scan(&cr.ID, &cr.DisplayName, &cr.Name); err != nil {
				log.Printf("Scan Error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
				return
			}
			cantors = append(cantors, cr)
		}

		c.JSON(http.StatusOK, cantors)
	}
}

// handleGetRates - returns the current exchange rates for a specified cantor and currency
func handleGetRates(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		cantorIDStr := c.Query("cantor_id")
		currency := c.Query("currency")

		if cantorIDStr == "" || currency == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing cantor_id or currency"})
			return
		}

		cantorID, err := strconv.Atoi(cantorIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cantor_id"})
			return
		}

		ctx := c.Request.Context()
		cacheKey := fmt.Sprintf("rates:%d:%s", cantorID, currency)

		if cachedVal, err := app.Cache.Get(ctx, cacheKey).Result(); err == nil {
			c.Data(http.StatusOK, "application/json", []byte(cachedVal))
			return
		}

		var ci CantorInfo
		err = app.DB.QueryRow(ctx, "SELECT base_url, strategy, units FROM cantors where id = $1", cantorID).
			Scan(&ci.BaseURL, &ci.Strategy, &ci.Units)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Cantor not found"})
			} else {
				log.Printf("DB Error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			}
			return
		}

		scrapeResult, err := runScrapeStrategy(ci, currency)
		if err != nil {
			log.Printf("Scraper error (%s): %v", ci.Strategy, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		rates, err := processRates(scrapeResult, ci.Units)
		if err != nil {
			log.Printf("Parsing error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Rates parsing error"})
			return
		}

		response := RatesResponse{
			BuyRate:  fmt.Sprintf("%.4f", rates.Buy),
			SellRate: fmt.Sprintf("%.4f", rates.Sell),
			CantorID: cantorID,
			Currency: currency,
		}
		responseJSON, _ := json.Marshal(response)
		app.Cache.Set(ctx, cacheKey, responseJSON, 60*time.Second)

		go saveToArchive(app.DB, cantorID, currency, rates.Buy, rates.Sell)

		c.JSON(http.StatusOK, response)
	}
}

// --- Helpers ---

// connectToDB - connecting to the database
func connectToDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("can't create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("couldn't ping pool connection: %w", err)
	}
	return pool, nil
}

// connectToRedis - connecting to Redis
func connectToRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("can't parse the REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := client.Ping(pingCtx).Result(); err != nil {
		return nil, fmt.Errorf("couldn't ping Redis: %w", err)
	}
	return client, nil
}

// initSchema - helper to init schema (reused from previous steps)
func initSchema(ctx context.Context, db *pgxpool.Pool) error {
	const schema = `
    CREATE EXTENSION IF NOT EXISTS timescaledb;
    CREATE TABLE IF NOT EXISTS cantors (
        id SERIAL PRIMARY KEY,
        name VARCHAR(50) NOT NULL UNIQUE,
        display_name VARCHAR(100) NOT NULL,
        base_url TEXT NOT NULL,
        strategy VARCHAR(10) NOT NULL,
        units INTEGER DEFAULT 1
    );
    CREATE TABLE IF NOT EXISTS rates (
        time TIMESTAMPTZ NOT NULL,
        cantor_id INTEGER NOT NULL REFERENCES cantors(id),
        currency VARCHAR(3) NOT NULL,
        buy_rate NUMERIC(10, 4) NOT NULL,
        sell_rate NUMERIC(10, 4) NOT NULL,
        UNIQUE (time, cantor_id, currency)
    );
    SELECT create_hypertable('rates', 'time', if_not_exists => TRUE);
    `
	_, err := db.Exec(ctx, schema)
	return err
}

func runScrapeStrategy(ci CantorInfo, currency string) (scrapers.ScrapeResult, error) {
	switch ci.Strategy {
	case "C1":
		return scrapers.FetchC1(ci.BaseURL, currency)
	case "C2":
		return scrapers.FetchC2(ci.BaseURL, currency)
	case "C3":
		return scrapers.FetchC3(ci.BaseURL, currency)
	default:
		return scrapers.ScrapeResult{}, fmt.Errorf("unknown scrape strategy: %s", ci.Strategy)
	}
}

func processRates(result scrapers.ScrapeResult, units int) (processedRates, error) {
	buyRateF, errB := strconv.ParseFloat(cleanRate(result.BuyRate), 64)
	sellRateF, errS := strconv.ParseFloat(cleanRate(result.SellRate), 64)

	if errB != nil || errS != nil {
		return processedRates{}, fmt.Errorf("couldn't parse data")
	}

	if units > 1 {
		buyRateF = buyRateF / float64(units)
		sellRateF = sellRateF / float64(units)
	}

	return processedRates{Buy: buyRateF, Sell: sellRateF}, nil
}

func cleanRate(raw string) string {
	s := strings.ReplaceAll(raw, ",", ".")
	s = strings.TrimSpace(s)
	var cleaned strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned.WriteRune(r)
		}
	}
	return cleaned.String()
}

func saveToArchive(db *pgxpool.Pool, cantorID int, currency string, buyRate, sellRate float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.Exec(ctx,
		"INSERT INTO rates (time, cantor_id, currency, buy_rate, sell_rate) VALUES (NOW(), $1, $2, $3, $4)",
		cantorID, currency, buyRate, sellRate,
	)
	if err != nil {
		log.Printf("Archive Error: %v", err)
	}
}
