package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Niutaq/Gix/docs"
	"github.com/Niutaq/Gix/internal/api"
	"github.com/Niutaq/Gix/internal/api/rpc"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/internal/workers"
	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/Niutaq/Gix/pkg/search"
	"github.com/nats-io/nats.go"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

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
func main() {
	tracer.Start(tracer.WithService("gix-api"))
	defer tracer.Stop()

	log.Println("Launching Gix API...")

	if envHost := os.Getenv("SWAGGER_HOST"); envHost != "" {
		docs.SwaggerInfo.Host = envHost
	}

	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")

	if databaseURL == "" || redisURL == "" {
		log.Fatal("Can't start server. Missing DATABASE_URL or REDIS_URL environment variables.")
	}

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" {
		provider = "ollama"
	}
	switch provider {
	case "ollama", "auto":
		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			model = "llama3.2"
		}
		base := os.Getenv("OLLAMA_URL")
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		log.Printf("[LLM] Offline provider=%s model=%s url=%s (no API key)", provider, model, base)
	case "gemini":
		if os.Getenv("GEMINI_API_KEY") == "" {
			log.Printf("[LLM] Warning: LLM_PROVIDER=gemini but GEMINI_API_KEY is empty")
		} else {
			log.Printf("[LLM] Legacy cloud Gemini configured")
		}
	default:
		log.Printf("[LLM] Unknown LLM_PROVIDER=%q — defaulting to ollama", provider)
	}

	dbpool, err := infrastructure.ConnectToDB(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Can't connect to database: %v\n", err)
	}
	defer dbpool.Close()
	log.Println("Successfully connected to database.")

	if err := infrastructure.InitSchema(ctx, dbpool); err != nil {
		log.Fatalf("Can't initialize database schema: %v\n", err)
	}

	rdb, err := infrastructure.ConnectToRedis(ctx, redisURL)
	if err != nil {
		log.Fatalf("Can't connect to caching client: %v\n", err)
	}
	log.Println("Successfully connected to Redis.")

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
	defer func() {
		if nc != nil {
			_ = nc.Drain()
		}
	}()

	js := infrastructure.SetupNATS(nc)

	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}
	se, err := search.NewSearchEngine(esURL)
	if err != nil {
		log.Printf("[Search] Elasticsearch not running (optional) — skipping geo search sync")
		se = nil
	} else {
		go func() {
			for i := 0; i < 5; i++ {
				if err := se.CreateIndices(); err == nil {
					break
				}
				time.Sleep(3 * time.Second)
			}
		}()
	}

	estimator := finops.NewTimescaleEstimator(dbpool, finops.DefaultRates)
	govEngine := finops.NewGovernanceEngine(estimator, 0.05)
	log.Printf("FinOps governance active (breaker $%.2f/day per cantor)", 0.05)
	govEngine.StartMonitor(context.Background(), 1*time.Minute)

	appState := &infrastructure.AppState{
		DB:         dbpool,
		Cache:      rdb,
		JS:         js,
		Search:     se,
		Governance: govEngine,
	}

	if se != nil {
		go infrastructure.SyncCantorsToES(appState)
	}

	go rpc.StartDRPCServer(dbpool, rdb)

	go workers.StartBackgroundHarvester(appState)

	r := api.SetupRouter(appState)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("Gin API listens at port :8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
