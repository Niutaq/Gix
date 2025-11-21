// cmd/gix-server/main.go
package main

import (
	"bytes"
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

// initSchema - a function for initializing the database schema
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
		created_at TIMESTAMP DEFAULT NOW()
	);
	
	CREATE TABLE IF NOT EXISTS rates (
		time TIMESTAMPTZ NOT NULL,
		cantor_id INTEGER NOT NULL REFERENCES cantors(id),
		currency VARCHAR(3) NOT NULL,
		buy_rate NUMERIC(10, 4) NOT NULL,
		sell_rate NUMERIC(10, 4) NOT NULL,
	
		UNIQUE (time, cantor_id, currency)
	);
    `

	log.Println("Verifying database schema...")
	_, err := db.Exec(ctx, schema)

	_, _ = db.Exec(ctx, "SELECT create_hypertable('rates', 'time', if_not_exists => TRUE);")

	return err
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

	// Endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthCheck(appState))
	mux.HandleFunc("/api/v1/cantors", handleCantorsList(appState))
	mux.HandleFunc("/api/v1/rates", handleGetRates(appState))

	log.Println("API listens at port :8080...")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("HTTP-server found error: %v", err)
	}
}

// connectToDB - a function for connecting to the database
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

// connectToRedis - a function for connecting to Redis
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

// --- HTTP handlers ---
// handleHealthCheck - returns 200 OK if DB and Redis are up
func handleHealthCheck(app *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := app.DB.Ping(r.Context()); err != nil {
			log.Printf("Health check failed (DB): %v", err)
			http.Error(w, `{"status":"error", "service":"database"}`, http.StatusServiceUnavailable)
			return
		}
		if _, err := app.Cache.Ping(r.Context()).Result(); err != nil {
			log.Printf("Health check failed (Cache): %v", err)
			http.Error(w, `{"status":"error", "service":"cache"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set(contentTypeText, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, `{"status": "ok", "message": "Gix is alive."}`)
		if err != nil {
			return
		}
	}
}

// handleCantorsList - returns a list of all cantors
func handleCantorsList(app *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := app.DB.Query(r.Context(),
			"SELECT id, display_name, name FROM cantors")

		if err != nil {
			log.Printf("Error while getting cantors list: %v\n", err)
			http.Error(w, internalServerError, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var cantors []CantorListResponse

		for rows.Next() {
			var c CantorListResponse

			if err := rows.Scan(&c.ID, &c.DisplayName, &c.Name); err != nil {
				log.Printf("Error while scanning cantors list: %v\n", err)
				http.Error(w, internalServerError, http.StatusInternalServerError)
				return
			}
			cantors = append(cantors, c)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error after scanning cantors list: %v\n", err)
			http.Error(w, internalServerError, http.StatusInternalServerError)
			return
		}

		w.Header().Set(contentTypeText, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(cantors); err != nil {
			log.Printf("Error encoding cantors list to JSON: %v\n", err)
			http.Error(w, internalServerError, http.StatusInternalServerError)
			return
		}

		w.Header().Set(contentTypeText, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		//_, err = w.Write(buf.Bytes())
		//if err != nil {
		//	return
		//}

		if err := json.NewEncoder(w).Encode(cantors); err != nil {
			log.Printf("Error encoding cantors list to JSON: %v\n", err)
			return
		}
	}
}

func handleGetRates(app *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		params, err := parseRateParams(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cacheKey := fmt.Sprintf("rates:%d:%s", params.CantorID, params.Currency)
		if cachedJSON, ok := getCachedRates(ctx, app.Cache, cacheKey); ok {
			log.Printf("Cache hit for cache-key: %s", cacheKey)
			writeJSONResponse(w, http.StatusOK, cachedJSON)
			return
		}
		log.Printf("Cache miss for cache-key: %s\n", cacheKey)

		ci, err := getCantorInfo(ctx, app.DB, params.CantorID)
		if err != nil {
			handleDBError(w, err) // Funkcja pomocnicza obsługuje błędy 404/500
			return
		}

		scrapeResult, err := runScrapeStrategy(ci, params.Currency)
		if err != nil {
			log.Printf("Scraper error (%s): %v", ci.Strategy, err)
			http.Error(w, fmt.Sprintf("Error fetching data: %v", err), http.StatusInternalServerError)
			return
		}

		rates, err := processRates(scrapeResult, ci.Units)
		if err != nil {
			log.Printf("Couldn't parse data into floats: %s, %s", scrapeResult.BuyRate, scrapeResult.SellRate)
			http.Error(w, "Rates' parsing error:", http.StatusInternalServerError)
			return
		}

		err = cacheAndRespond(ctx, app.Cache, w, params, rates, cacheKey)
		if err != nil {
			return
		}

		go saveToArchive(app.DB, params.CantorID, params.Currency, rates.Buy, rates.Sell)
	}
}

// parseRateParams - a helper function for parsing rate parameters from the URL
func parseRateParams(r *http.Request) (rateParams, error) {
	cantorIDStr := r.URL.Query().Get("cantor_id")
	currency := r.URL.Query().Get("currency")

	if cantorIDStr == "" || currency == "" {
		return rateParams{}, fmt.Errorf("missing required parameters: cantor_id and currency")
	}

	cantorID, err := strconv.Atoi(cantorIDStr)
	if err != nil {
		return rateParams{}, fmt.Errorf("invalid cantor_id")
	}
	return rateParams{CantorID: cantorID, Currency: currency}, nil
}

// getCachedRates - a helper function for getting cached rates from Redis
func getCachedRates(ctx context.Context, cache *redis.Client, cacheKey string) ([]byte, bool) {
	cachedVal, err := cache.Get(ctx, cacheKey).Result()
	if err == nil {
		return []byte(cachedVal), true // Cache Hit
	}
	if !errors.Is(err, redis.Nil) {
		log.Printf("Error while getting data from cache: %v\n", err)
	}
	return nil, false // Cache Miss
}

// getCantorInfo - a helper function for getting cantor info from the database
func getCantorInfo(ctx context.Context, db *pgxpool.Pool, cantorID int) (CantorInfo, error) {
	var ci CantorInfo
	err := db.QueryRow(ctx, "SELECT base_url, strategy, units FROM cantors where id = $1",
		cantorID).Scan(&ci.BaseURL, &ci.Strategy, &ci.Units)
	return ci, err
}

// handleDBError - a helper function for handling database errors
func handleDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Not found cantor at specified ID", http.StatusNotFound)
		return
	}
	log.Printf("Error while getting cantor info: %v\n", err)
	http.Error(w, "Internal server error (database)", http.StatusInternalServerError)
}

// runScrapeStrategy - a helper function for running scraping strategies
func runScrapeStrategy(ci CantorInfo, currency string) (scrapers.ScrapeResult, error) {
	switch ci.Strategy {
	case "C1":
		log.Println("Running strategy: C1 (Tadek)")
		return scrapers.FetchC1(ci.BaseURL, currency)
	case "C2":
		log.Println("Running strategy: C2 (Kwadrat)")
		return scrapers.FetchC2(ci.BaseURL, currency)
	case "C3":
		log.Println("Running strategy: C3 (Supersam)")
		return scrapers.FetchC3(ci.BaseURL, currency)
	default:
		return scrapers.ScrapeResult{}, fmt.Errorf("unknown scrape strategy: %s", ci.Strategy)
	}
}

// processRates - a helper function for processing rates from the source
func processRates(result scrapers.ScrapeResult, units int) (processedRates, error) {
	buyRateF, errB := strconv.ParseFloat(cleanRate(result.BuyRate), 64)
	sellRateF, errS := strconv.ParseFloat(cleanRate(result.SellRate), 64)

	if errB != nil || errS != nil {
		return processedRates{}, fmt.Errorf("couldn't parse data into floats: %s, %s", result.BuyRate, result.SellRate)
	}

	finalBuyRateF := buyRateF
	finalSellRateF := sellRateF

	if units > 1 {
		log.Printf("Dividing rates by %d", units)
		finalBuyRateF = buyRateF / float64(units)
		finalSellRateF = sellRateF / float64(units)
	}

	return processedRates{Buy: finalBuyRateF, Sell: finalSellRateF}, nil
}

// cacheAndRespond - a helper function for caching and sending responses
func cacheAndRespond(ctx context.Context, cache *redis.Client, w http.ResponseWriter,
	params rateParams, rates processedRates, cacheKey string) error {
	buyRateStr := fmt.Sprintf("%.4f", rates.Buy)
	sellRateStr := fmt.Sprintf("%.4f", rates.Sell)

	response := RatesResponse{
		BuyRate:  buyRateStr,
		SellRate: sellRateStr,
		CantorID: params.CantorID,
		Currency: params.Currency,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		log.Printf("JSON conversion error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return err
	}

	// 60 sec caching time
	cacheDuration := 60 * time.Second
	err = cache.Set(ctx, cacheKey, responseJSON, cacheDuration).Err()
	if err != nil {
		log.Printf("Warning: Couldn't send data to Redis: %v", err)
	}

	writeJSONResponse(w, http.StatusOK, responseJSON)
	return nil
}

// writeJSONResponse - a helper function for writing JSON responses
func writeJSONResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// cleanRate - a function for cleaning rates from the source
func cleanRate(raw string) string {
	s := strings.ReplaceAll(raw, ",", ".")
	s = strings.TrimSpace(s)

	var cleaned strings.Builder
	cleaned.Grow(len(s))

	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned.WriteRune(r)
		}
	}
	return cleaned.String()
}

// saveToArchive - a function for saving rates to TimescaleDB's archive'
func saveToArchive(db *pgxpool.Pool, cantorID int, currency string, buyRate, sellRate float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Exec(ctx,
		"INSERT INTO rates (time, cantor_id, currency, buy_rate, sell_rate) VALUES (NOW(), $1, $2, $3, $4)",
		cantorID, currency, buyRate, sellRate,
	)
	if err != nil {
		log.Printf("Couldn't save rates into archive: %v", err)
	} else {
		log.Println("Saved rates to TimescaleDB's archive.")
	}
}
