// cmd/gix-server/main.go
package main

import (
	// Standard libraries
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

	// External libraries
	"github.com/Niutaq/Gix/pkg/scrapers"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	// Swagger utilities
	_ "github.com/Niutaq/Gix/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Constants
const (
	// Content-Type headers
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
	ID          int     `json:"id"`
	DisplayName string  `json:"displayName"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// RatesResponse - a response struct for the /api/v1/rates endpoint
type RatesResponse struct {
	BuyRate  string `json:"buyRate"`
	SellRate string `json:"sellRate"`
	CantorID int    `json:"cantorID"`
	Currency string `json:"currency"`
}

// processedRates - a struct for holding the final rates after processing
type processedRates struct {
	Buy  float64
	Sell float64
}

// @title           Gix API
// @version         1.0
// @description     This is the backend API for the Gix exchange rate application.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      165.227.246.100:8080
// @BasePath  /api/v1

// main initializes the application by connecting to the database and Redis, setting up routes, and starting the HTTP server.
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
	})

	// Endpoints
	r.GET("/healthz", handleHealthCheck(appState))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/cantors", handleCantorsList(appState))
		v1.GET("/rates", handleGetRates(appState))
	}

	// Swagger route
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	log.Println("Gin API listens at port :8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// --- HTTP handlers ---

// handleHealthCheck godoc
// @Summary      Health Check
// @Description  Checks if the server, database, and Redis are running correctly.
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /healthz [get]
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

// handleCantorsList godoc
// @Summary      List Cantors
// @Description  Returns a list of all available cantors with their geolocations.
// @Tags         cantors
// @Produce      json
// @Success      200  {array}   CantorListResponse
// @Failure      500  {object}  map[string]string
// @Router       /cantors [get]
func handleCantorsList(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := app.DB.Query(c.Request.Context(), "SELECT id, display_name, name, latitude, longitude FROM cantors")
		if err != nil {
			log.Printf("DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": internalServerError})
			return
		}
		defer rows.Close()

		var cantors []CantorListResponse
		for rows.Next() {
			var cr CantorListResponse
			if err := rows.Scan(&cr.ID, &cr.DisplayName, &cr.Name, &cr.Latitude, &cr.Longitude); err != nil {
				log.Printf("Scan Error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": internalServerError})
				return
			}
			cantors = append(cantors, cr)
		}

		c.JSON(http.StatusOK, cantors)
	}
}

// handleGetRates godoc
// @Summary      Get Rates
// @Description  Returns the buy and sell rates for a specific cantor and currency. Scrapes data in real-time if not cached.
// @Tags         rates
// @Produce      json
// @Param        cantor_id  query     int     true  "Cantor ID"
// @Param        currency   query     string  true  "Currency Code (e.g., EUR, USD)"
// @Success      200  {object}  RatesResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /rates [get]
func handleGetRates(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		cantorID, currency, err := parseRateParams(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		cacheKey := fmt.Sprintf("rates:%d:%s", cantorID, currency)

		if respondFromCache(c, app.Cache, cacheKey) {
			return
		}

		cantorInfo, err := fetchCantorInfo(ctx, app.DB, cantorID)
		if err != nil {
			handleDBError(c, err)
			return
		}

		response, rates, err := scrapeAndProcess(cantorInfo, cantorID, currency)
		if err != nil {
			log.Printf("Processing Error (%s): %v", cantorInfo.Strategy, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		cacheAndArchive(ctx, app, cacheKey, cantorID, currency, response, rates)

		c.JSON(http.StatusOK, response)
	}
}

// parseRateParams - a helper function to parse rate params from the query string
func parseRateParams(c *gin.Context) (int, string, error) {
	cantorIDStr := c.Query("cantor_id")
	currency := c.Query("currency")

	if cantorIDStr == "" || currency == "" {
		return 0, "", fmt.Errorf("missing cantor_id or currency")
	}

	cantorID, err := strconv.Atoi(cantorIDStr)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cantor_id")
	}

	return cantorID, currency, nil
}

// respondFromCache - a helper function to respond from cache if available
func respondFromCache(c *gin.Context, cache *redis.Client, key string) bool {
	if cachedVal, err := cache.Get(c.Request.Context(), key).Result(); err == nil {
		c.Data(http.StatusOK, contentTypeJSON, []byte(cachedVal))
		return true
	}
	return false
}

// fetchCantorInfo - a helper function to fetch cantor info from the DB
func fetchCantorInfo(ctx context.Context, db *pgxpool.Pool, id int) (CantorInfo, error) {
	var ci CantorInfo
	err := db.QueryRow(ctx, "SELECT base_url, strategy, units FROM cantors where id = $1", id).
		Scan(&ci.BaseURL, &ci.Strategy, &ci.Units)
	return ci, err
}

// handleDBError - a helper function to handle DB errors
func handleDBError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cantor not found"})
	} else {
		log.Printf("DB Error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
	}
}

// scrapeAndProcess - a helper function to scrape and process the rates
func scrapeAndProcess(ci CantorInfo, id int, currency string) (RatesResponse, processedRates, error) {
	scrapeResult, err := runScrapeStrategy(ci, currency)
	if err != nil {
		return RatesResponse{}, processedRates{}, err
	}

	rates, err := processRates(scrapeResult, ci.Units)
	if err != nil {
		return RatesResponse{}, processedRates{}, fmt.Errorf("rates parsing error: %w", err)
	}

	response := RatesResponse{
		BuyRate:  fmt.Sprintf("%.4f", rates.Buy),
		SellRate: fmt.Sprintf("%.4f", rates.Sell),
		CantorID: id,
		Currency: currency,
	}

	return response, rates, nil
}

// cacheAndArchive - a helper function to cache and archive the response
func cacheAndArchive(ctx context.Context, app *AppState, key string, id int, curr string, resp RatesResponse, rates processedRates) {
	if responseJSON, err := json.Marshal(resp); err == nil {
		app.Cache.Set(ctx, key, responseJSON, 60*time.Second)
	}
	go saveToArchive(app.DB, id, curr, rates.Buy, rates.Sell)
}

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
        units INTEGER DEFAULT 1,
        latitude DECIMAL(9,6) DEFAULT 0,
        longitude DECIMAL(9,6) DEFAULT 0
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

// runScrapeStrategy - a helper function to run the scrape strategy based on the cantor's strategy'
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

// processRates - a helper function to process the rates based on the cantor's units'
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

// cleanRate - a helper function to clean the raw rate string
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

// saveToArchive - a helper function to save the rates to the archive table
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
