// cmd/gix-server/main.go
package main

import (
	// Standard libraries
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/Niutaq/Gix/pkg/scrapers"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	redistrace "github.com/DataDog/dd-trace-go/contrib/redis/go-redis.v9/v2"
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/Niutaq/Gix/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Constants
const (
	contentTypeProtoBuf = "application/x-protobuf"

	internalServerError = "Internal server error"
)

// AppState struct holds all app-wide components like DB pools
type AppState struct {
	DB    *pgxpool.Pool
	Cache redis.UniversalClient
	JS    nats.JetStreamContext
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

// historyParams represents parameters for querying historical data such as rates or transactions.
type historyParams struct {
	cantorID int
	days     int
	cutoff   time.Time
}

// processedRates - a struct for holding the final rates after processing
type processedRates struct {
	Buy  float64
	Sell float64
}

// RatesResponse - a response struct for the /api/v1/rates endpoint
//type RatesResponse struct {
//	BuyRate  string `json:"buyRate"`
//	SellRate string `json:"sellRate"`
//	CantorID int    `json:"cantorID"`
//	Currency string `json:"currency"`
//}

// @title           Gix API
// @version         1.0
// @description     This is the backend API for the Gix exchange rate application.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// main initializes the application by connecting to the database and Redis, setting up routes, and starting the HTTP server.
func main() {
	// Start DataDog tracer
	tracer.Start(tracer.WithService("gix-server"))
	defer tracer.Stop()

	log.Println("Launching Gix server...")

	if envHost := os.Getenv("SWAGGER_HOST"); envHost != "" {
		docs.SwaggerInfo.Host = envHost
	}

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

	// Connect to NATS
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Printf("Warning: Can't connect to NATS (Streaming disabled): %v", err)
	} else {
		log.Println("Successfully connected to NATS.")
	}

	js := setupNATS(nc)
	defer func() {
		if nc != nil {
			_ = nc.Drain()
		}
	}()

	// FinOps Summary Report on startup
	log.Println("--- FinOps Analysis ---")
	log.Println(finops.DefaultRates.Summary(1, 1)) // Assuming 1 node, 1 LB for now
	for _, tip := range finops.GenerateTips(1, 1) {
		log.Printf("[TIP] %s: %s (Potential: $%.2f/mo)", tip.Title, tip.Description, tip.Potential)
	}
	log.Println("-----------------------")

	appState := &AppState{
		DB:    dbpool,
		Cache: rdb,
		JS:    js,
	}

	// Start dRPC server
	go startDRPCServer(dbpool, rdb)

	// Start the background harvester to collect data 24/7
	go startBackgroundHarvester(appState)

	r := gin.Default()

	// DataDog Middleware
	r.Use(gintrace.Middleware("gix-server"))

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Next()
	})

	r.GET("/healthz", handleHealthCheck(appState))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/cantors", handleCantorsList(appState))
		v1.GET("/rates", handleGetRates(appState))
		v1.GET("/history", handleGetHistory(appState))
		v1.GET("/finops", handleFinOps())
	}
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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Gix is alive."})
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
		cacheKey := fmt.Sprintf("rates:proto%d:%s", cantorID, currency)

		if respondFromCache(c, app.Cache, cacheKey) {
			return
		}

		cantorInfo, err := fetchCantorInfo(ctx, app.DB, cantorID)
		if err != nil {
			handleDBError(c, err)
			return
		}

		response, rates, err := scrapeAndProcess(app, cantorInfo, cantorID, currency)
		if err != nil {
			log.Printf("Processing Error (%s): %v", cantorInfo.Strategy, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		cacheAndArchive(ctx, app, cacheKey, cantorID, currency, response, rates)

		if c.GetHeader("Accept") == contentTypeProtoBuf {
			c.ProtoBuf(http.StatusOK, response)
		} else {
			c.JSON(http.StatusOK, response)
		}
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
func respondFromCache(c *gin.Context, cache redis.UniversalClient, key string) bool {
	if cachedBytes, err := cache.Get(c.Request.Context(), key).Bytes(); err == nil {
		var cachedRate pb.RateResponse
		if err := proto.Unmarshal(cachedBytes, &cachedRate); err == nil {
			if c.GetHeader("Accept") == contentTypeProtoBuf {
				c.ProtoBuf(http.StatusOK, &cachedRate)
			} else {
				c.JSON(http.StatusOK, &cachedRate)
			}
			return true
		}
		log.Printf("Unmarshal Error: %v", err)
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
func scrapeAndProcess(app *AppState, ci CantorInfo, id int, currency string) (*pb.RateResponse, processedRates, error) {
	scrapeResult, err := runScrapeStrategy(ci, currency)
	if err != nil {
		return nil, processedRates{}, err
	}

	rates, err := processRates(scrapeResult, ci.Units)
	if err != nil {
		return nil, processedRates{}, fmt.Errorf("rates parsing error: %w", err)
	}

	response := &pb.RateResponse{
		BuyRate:   fmt.Sprintf("%.3f", rates.Buy),
		SellRate:  fmt.Sprintf("%.3f", rates.Sell),
		CantorId:  int32(id),
		Currency:  currency,
		FetchedAt: time.Now().Unix(),
	}

	// Calculate change from 24h ago
	if prevBuy, err := getPreviousRate(app.DB, id, currency); err == nil && prevBuy > 0 {
		change := ((rates.Buy - prevBuy) / prevBuy) * 100
		response.Change24H = change
	}

	return response, rates, nil
}

// getPreviousRate retrieves the rate from approximately 24 hours ago.
func getPreviousRate(db *pgxpool.Pool, id int, currency string) (float64, error) {
	var buyRate float64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := db.QueryRow(ctx,
		"SELECT buy_rate FROM rates WHERE cantor_id=$1 AND currency=$2 AND time <= NOW() - INTERVAL '24 hours' ORDER BY time DESC LIMIT 1",
		id, currency).Scan(&buyRate)
	return buyRate, err
}

// cacheAndArchive - a helper function to cache and archive the response
func cacheAndArchive(ctx context.Context, app *AppState, key string, id int, curr string,
	resp *pb.RateResponse, rates processedRates) {
	if protoBytes, err := proto.Marshal(resp); err == nil {
		app.Cache.Set(ctx, key, protoBytes, 60*time.Second)
	} else {
		log.Printf("Marshal Error: %v", err)
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

// setupNATS initializes a connection to NATS and sets up the JetStream context with a default stream.
func setupNATS(nc *nats.Conn) nats.JetStreamContext {
	if nc == nil {
		return nil
	}

	js, err := nc.JetStream()
	if err != nil {
		log.Printf("Warning: Can't init JetStream: %v", err)
		return nil
	}

	log.Println("JetStream initialized.")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "RATES",
		Subjects: []string{"rates.*"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
	})
	if err != nil {
		log.Printf("Warning: Could not create stream: %v", err)
	}
	return js
}

// startDRPCServer begins listening for and serving dRPC requests on a dedicated port.
func startDRPCServer(db *pgxpool.Pool, cache redis.UniversalClient) {
	lis, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("failed to listen for dRPC: %v", err)
	}
	mux := drpcmux.New()
	err = pb.DRPCRegisterRatesService(mux, &RatesDRPCServer{Cache: cache, DB: db})
	if err != nil {
		log.Fatalf("failed to register dRPC service: %v", err)
	}
	srv := drpcserver.New(mux)
	log.Println("dRPC server listening on :8081")
	if err := srv.Serve(context.Background(), lis); err != nil {
		log.Fatalf("dRPC serve error: %v", err)
	}
}

// connectToRedis - connecting to Redis
func connectToRedis(ctx context.Context, redisURL string) (redis.UniversalClient, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("can't parse the REDIS_URL: %w", err)
	}
	// Use redistrace.NewClient which automatically adds the tracing hook
	client := redistrace.NewClient(opts)

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
	scraper, err := scrapers.GetScraper(ci.Strategy)
	if err != nil {
		return scrapers.ScrapeResult{}, err
	}
	return scraper(ci.BaseURL, currency)
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

// handleGetHistory processes the /history endpoint, retrieves historical exchange rates, and returns them as JSON or ProtoBuf.
func handleGetHistory(app *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		currency := c.Query("currency")
		if currency == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing currency"})
			return
		}

		params := parseHistoryParams(c)
		query, args := buildHistoryQuery(currency, params)
		rows, err := app.DB.Query(c.Request.Context(), query, args...)

		if err != nil {
			log.Printf("History DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": internalServerError})
			return
		}

		defer rows.Close()
		points := scanHistoryPoints(rows)
		sendHistoryResponse(c, currency, points)
	}
}

// parseHistoryParams extracts Cantor ID and historical query parameters from the request and returns a historyParams struct.
func parseHistoryParams(c *gin.Context) historyParams {
	cantorIDStr := c.Query("cantor_id")
	daysStr := c.DefaultQuery("days", "7")
	days, _ := strconv.Atoi(daysStr)

	if days <= 0 {
		days = 7
	}

	cantorID, _ := strconv.Atoi(cantorIDStr)

	return historyParams{
		cantorID: cantorID,
		days:     days,
		cutoff:   time.Now().AddDate(0, 0, -days),
	}
}

// buildHistoryQuery constructs a SQL query and its arguments to fetch historical exchange rates based on input parameters.
func buildHistoryQuery(currency string, params historyParams) (string, []interface{}) {
	var query string
	var args []interface{}
	args = append(args, currency, params.cutoff)
	if params.cantorID > 0 {
		query = `
				SELECT time_bucket('1 hour', time) AS bucket,
					   AVG(buy_rate)::FLOAT,
					   AVG(sell_rate)::FLOAT
				FROM rates
				WHERE currency = $1 AND time > $2 AND cantor_id = $3
				GROUP BY bucket
				ORDER BY bucket ASC`
		args = append(args, params.cantorID)
	} else {
		query = `
				SELECT time_bucket('1 hour', time) AS bucket,
					   AVG(buy_rate)::FLOAT,
					   AVG(sell_rate)::FLOAT
				FROM rates
				WHERE currency = $1 AND time > $2
				GROUP BY bucket
				ORDER BY bucket ASC`
	}
	return query, args
}

// scanHistoryPoints scans rows from the database and maps them to a slice of HistoryPoint objects, handling time and rate fields.
func scanHistoryPoints(rows pgx.Rows) []*pb.HistoryPoint {
	var points []*pb.HistoryPoint
	for rows.Next() {

		var t time.Time
		var buy, sell float64

		if err := rows.Scan(&t, &buy, &sell); err != nil {
			log.Printf("History Scan Error: %v", err)
			continue
		}

		points = append(points, &pb.HistoryPoint{
			Time:     t.Unix(),
			BuyRate:  buy,
			SellRate: sell,
		})
	}
	return points
}

// sendHistoryResponse formats and sends a currency's historical rate response in JSON or ProtoBuf based on the "Accept" header.
func sendHistoryResponse(c *gin.Context, currency string, points []*pb.HistoryPoint) {
	resp := &pb.HistoryResponse{
		Points:   points,
		Currency: currency,
	}

	if c.GetHeader("Accept") == contentTypeProtoBuf {
		c.ProtoBuf(http.StatusOK, resp)

	} else {
		c.JSON(http.StatusOK, resp)
	}
}

// startBackgroundHarvester runs a loop that fetches rates every 15 minutes.
func startBackgroundHarvester(app *AppState) {
	currencies := []string{
		"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK",
		"CZK", "HUF", "UAH", "BGN", "RON", "TRY", "ISK", "LEK",
	}

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Run once immediately on start
	harvest(app, currencies)

	for range ticker.C {
		harvest(app, currencies)
	}
}

// harvest iterates through all cantors and currencies to fetch and save rates.
func harvest(app *AppState, currencies []string) {
	log.Println("Background Harvest: Starting collection cycle...")
	ctx := context.Background()

	cantors, err := fetchAllCantors(ctx, app.DB)
	if err != nil {
		log.Printf("Harvest Error (DB): %v", err)
		return
	}

	for _, ci := range cantors {
		for _, curr := range currencies {
			processCantorCurrency(ctx, app, ci, curr)
		}
	}
	log.Println("Background Harvest: Cycle completed.")
}

// fetchAllCantors retrieves all cantors from the database.
func fetchAllCantors(ctx context.Context, db *pgxpool.Pool) ([]CantorInfo, error) {
	rows, err := db.Query(ctx, "SELECT id, display_name, base_url, strategy, units FROM cantors")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cantors []CantorInfo
	for rows.Next() {
		var ci CantorInfo
		if err := rows.Scan(&ci.ID, &ci.DisplayName, &ci.BaseURL, &ci.Strategy, &ci.Units); err != nil {
			continue
		}
		cantors = append(cantors, ci)
	}
	return cantors, nil
}

// processCantorCurrency handles scraping and saving data for a single cantor and currency.
func processCantorCurrency(ctx context.Context, app *AppState, ci CantorInfo, curr string) {
	start := time.Now()
	time.Sleep(500 * time.Millisecond) // Throttling to be polite

	_, rates, err := scrapeAndProcess(app, ci, ci.ID, curr)
	duration := time.Since(start)

	if err != nil {
		return
	}

	if rates.Buy == 0 && rates.Sell == 0 {
		return
	}

	log.Printf("Harvesting: %s -> %s (%.3f / %.3f) [Perf: %v]", ci.DisplayName, curr, rates.Buy, rates.Sell, duration)
	finops.Stats.Record(ci.DisplayName, duration)

	saveToArchive(app.DB, ci.ID, curr, rates.Buy, rates.Sell)
	updateCacheAndNotify(ctx, app, ci.ID, curr, rates)
}

// handleFinOps provides a handler that generates a FinOps summary including cost breakdown and optimization tips.
func handleFinOps() gin.HandlerFunc {
	return func(c *gin.Context) {
		summary := finops.Stats.GetSummary()
		summary["infrastructure"] = finops.DefaultRates.Summary(1, 1)
		summary["tips"] = finops.GenerateTips(1, 1)
		c.JSON(http.StatusOK, summary)
	}
}

// updateCacheAndNotify updates Redis cache and publishes the event via Pub/Sub.
func updateCacheAndNotify(ctx context.Context, app *AppState, cantorID int, curr string, rates processedRates) {
	cacheKey := fmt.Sprintf("rates:proto%d:%s", cantorID, curr)
	response := &pb.RateResponse{
		BuyRate:   fmt.Sprintf("%.3f", rates.Buy),
		SellRate:  fmt.Sprintf("%.3f", rates.Sell),
		CantorId:  int32(cantorID),
		Currency:  curr,
		FetchedAt: time.Now().Unix(),
	}

	protoBytes, err := proto.Marshal(response)
	if err != nil {
		log.Printf("Marshal Error: %v", err)
		return
	}

	app.Cache.Set(ctx, cacheKey, protoBytes, 60*time.Second)
	app.Cache.Publish(ctx, "rates_updates", protoBytes)

	// Publish to NATS JetStream (Persistent Stream)
	if app.JS != nil {
		subject := fmt.Sprintf("rates.%s", curr)
		_, err := app.JS.Publish(subject, protoBytes)
		if err != nil {
			log.Printf("NATS Publish Error: %v", err)
		}
	}
}
