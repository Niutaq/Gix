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

// ... (funkcje connectToDB i connectToRedis bez zmian) ...
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
		w.Header().Set("Content-Type", "application/json")
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
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var cantors []CantorListResponse

		for rows.Next() {
			var c CantorListResponse

			if err := rows.Scan(&c.ID, &c.DisplayName, &c.Name); err != nil {
				log.Printf("Error while scanning cantors list: %v\n", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			cantors = append(cantors, c)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error after scanning cantors list: %v\n", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(cantors); err != nil {
			log.Printf("Error encoding cantors list to JSON: %v\n", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(buf.Bytes())
		if err != nil {
			return
		}
	}
}

// ... (handleGetRates jest już poprawny, bez zmian) ...
func handleGetRates(app *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Part 1: Getting data from a URL
		cantorIDStr := r.URL.Query().Get("cantor_id")
		currency := r.URL.Query().Get("currency")

		if cantorIDStr == "" || currency == "" {
			http.Error(w, "Missing required parameters: cantor_id and currency", http.StatusBadRequest)
			return
		}

		cantorID, err := strconv.Atoi(cantorIDStr)
		if err != nil {
			http.Error(w, "Invalid cantor_id", http.StatusBadRequest)
			return
		}

		// Part 2: Unique key for Redis
		cacheKey := fmt.Sprintf("rates:%d:%s", cantorID, currency)
		log.Printf("Trying to download data for cache-key: %s", cacheKey)
		ctx := r.Context()

		// Part 3: Cache (Redis)
		cachedVal, err := app.Cache.Get(ctx, cacheKey).Result()
		if err == nil {
			// Part 4: Cache Hit
			log.Printf("Cache hit for cache-key: %s", cacheKey)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(cachedVal)); err != nil {
				log.Printf("Error writing response: %v", err)
				return
			}
			return
		}

		// Part 5: Cache Miss
		if !errors.Is(redis.Nil, err) {
			log.Printf("Error while getting data from cache: %v\n", err)
			http.Error(w, "Internal server error (cache)", http.StatusInternalServerError)
			return
		}
		log.Printf("Cache miss for cache-key: %s\n", cacheKey)

		// a. Downloading strategy from DB
		var ci CantorInfo
		errCI := app.DB.QueryRow(ctx, "SELECT base_url, strategy, units FROM cantors where id = $1",
			cantorID).Scan(&ci.BaseURL, &ci.Strategy, &ci.Units)

		if errCI != nil {
			if errors.Is(errCI, pgx.ErrNoRows) {
				http.Error(w, "Not found cantor at specified ID", http.StatusNotFound)
				return
			}
			log.Printf("Error while getting cantor info: %v\n", err)
			http.Error(w, "Internal server error (database)", http.StatusInternalServerError)
			return
		}

		// b. Colly - dispatching to the correct strategy
		var result scrapers.ScrapeResult
		var scrapeErr error

		// This is the dispatcher
		switch ci.Strategy {
		case "C1":
			log.Println("Running strategy: C1 (Tadek)")
			result, scrapeErr = scrapers.FetchC1(ci.BaseURL, currency)
		case "C2":
			log.Println("Running strategy: C2 (Kwadrat)")
			result, scrapeErr = scrapers.FetchC2(ci.BaseURL, currency)
		case "C3":
			log.Println("Running strategy: C3 (Supersam)")
			result, scrapeErr = scrapers.FetchC3(ci.BaseURL, currency)
		default:
			scrapeErr = fmt.Errorf("unknown scrape strategy: %s", ci.Strategy)
		}

		if scrapeErr != nil {
			log.Printf("Scraper error (%s): %v", ci.Strategy, scrapeErr)
			http.Error(w, fmt.Sprintf("Error fetching data: %v", scrapeErr), http.StatusInternalServerError)
			return
		}

		// +++ Units formatting +++
		buyRateF, errB := strconv.ParseFloat(cleanRate(result.BuyRate), 64)
		sellRateF, errS := strconv.ParseFloat(cleanRate(result.SellRate), 64)

		if errB != nil || errS != nil {
			log.Printf("Couldn't parse data into floats: %s, %s", result.BuyRate, result.SellRate)
			http.Error(w, "Rates' parsing error:", http.StatusInternalServerError)
			return
		}

		finalBuyRateF := buyRateF
		finalSellRateF := sellRateF

		// Format the rates if units > 1
		if ci.Units > 1 {
			log.Printf("Dividing rates by %d", ci.Units)
			finalBuyRateF = buyRateF / float64(ci.Units)
			finalSellRateF = sellRateF / float64(ci.Units)
		}

		buyRateStr := fmt.Sprintf("%.4f", finalBuyRateF)
		sellRateStr := fmt.Sprintf("%.4f", finalSellRateF)

		// Response
		response := RatesResponse{
			BuyRate:  buyRateStr,
			SellRate: sellRateStr,
			CantorID: cantorID,
			Currency: currency,
		}

		// JSON response
		responseJSON, err := json.Marshal(response)
		if err != nil {
			log.Printf("JSON conversion error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// 60 sec caching time
		cacheDuration := 60 * time.Second
		err = app.Cache.Set(ctx, cacheKey, responseJSON, cacheDuration).Err()
		if err != nil {
			log.Printf("Warning: Couldn't send data to Redis: %v", err)
		}

		// Goroutine for rates saving to TimescaleDB's archive
		go saveToArchive(app.DB, cantorID, currency, finalBuyRateF, finalSellRateF)

		// Proper response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(responseJSON)
		if err != nil {
			return
		}
	}
}

// ... (cleanRate i saveToArchive bez zmian) ...
func cleanRate(raw string) string {
	s := strings.Replace(raw, ",", ".", -1)
	s = strings.TrimSpace(s)
	cleaned := ""
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned += string(r)
		}
	}
	return cleaned
}
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
