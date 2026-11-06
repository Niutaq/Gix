package infrastructure

import (
	"context"
	"fmt"
	"log"
	"time"

	redistrace "github.com/DataDog/dd-trace-go/contrib/redis/go-redis.v9/v2"
	"github.com/Niutaq/Gix/pkg/search"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func ConnectToDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
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

func InitSchema(ctx context.Context, db *pgxpool.Pool) error {
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
        longitude DECIMAL(9,6) DEFAULT 0,
        address TEXT
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
    SELECT add_retention_policy('rates', INTERVAL '30 days', if_not_exists => TRUE);

    CREATE TABLE IF NOT EXISTS provider_unit_costs (
        time        TIMESTAMPTZ       NOT NULL,
        provider_id VARCHAR(50)       NOT NULL,
        scraper_type VARCHAR(20)      NOT NULL,
        duration_ms BIGINT            NOT NULL,
        estimated_cost_usd NUMERIC(18, 9) NOT NULL,
        trace_id    VARCHAR(100),
        service_category VARCHAR(50),
        resource_name VARCHAR(100)
    );
    SELECT create_hypertable('provider_unit_costs', 'time', if_not_exists => TRUE);
    SELECT add_retention_policy('provider_unit_costs', INTERVAL '60 days', if_not_exists => TRUE);

    CREATE TABLE IF NOT EXISTS focus_cost_records (
        time TIMESTAMPTZ NOT NULL,
        provider_name VARCHAR(50) NOT NULL DEFAULT 'OpenCost',
        source VARCHAR(50) NOT NULL,
        billed_cost NUMERIC(15, 10),
        effective_cost NUMERIC(15, 10) NOT NULL,
        service_category VARCHAR(50),
        resource_name VARCHAR(100),
        x_application VARCHAR(50) DEFAULT 'gix',
        x_team VARCHAR(50) DEFAULT 'platform',
        x_environment VARCHAR(50) DEFAULT 'local',
        charge_description TEXT
    );
    SELECT create_hypertable('focus_cost_records', 'time', if_not_exists => TRUE);
    SELECT add_retention_policy('focus_cost_records', INTERVAL '90 days', if_not_exists => TRUE);
    `
	_, err := db.Exec(ctx, schema)
	return err
}

func ConnectToRedis(ctx context.Context, redisURL string) (redis.UniversalClient, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("can't parse the REDIS_URL: %w", err)
	}
	client := redistrace.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := client.Ping(pingCtx).Result(); err != nil {
		return nil, fmt.Errorf("couldn't ping Redis: %w", err)
	}
	return client, nil
}

func SetupNATS(nc *nats.Conn) nats.JetStreamContext {
	if nc == nil {
		return nil
	}

	js, err := nc.JetStream()
	if err != nil {
		log.Printf("Warning: Can't init JetStream: %v", err)
		return nil
	}

	log.Println("JetStream initialized.")
	ensureStream(js, &nats.StreamConfig{
		Name:     "RATES",
		Subjects: []string{"rates.*"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
	})
	ensureStream(js, &nats.StreamConfig{
		Name:     "FINOPS_EVENTS",
		Subjects: []string{"gix.scrape.v1.completed"},
		MaxAge:   60 * 24 * time.Hour,
		Storage:  nats.FileStorage,
	})
	return js
}

func ensureStream(js nats.JetStreamContext, cfg *nats.StreamConfig) {
	if _, err := js.StreamInfo(cfg.Name); err == nil {
		return
	}
	if _, err := js.AddStream(cfg); err != nil {
		log.Printf("Warning: Could not create stream %s: %v", cfg.Name, err)
	}
}

func SyncCantorsToES(app *AppState) {
	if app == nil || app.Search == nil {
		return
	}
	ctx := context.Background()
	rows, err := app.DB.Query(ctx, "SELECT id, display_name, name, latitude, longitude FROM cantors")
	if err != nil {
		log.Printf("Sync DB Error: %v", err)
		return
	}
	defer rows.Close()

	var failCount int
	var lastErr error
	for rows.Next() {
		var cr search.CantorRecord
		if err := rows.Scan(&cr.ID, &cr.DisplayName, &cr.Name, &cr.Location.Lat, &cr.Location.Lon); err != nil {
			continue
		}
		if err := app.Search.IndexCantor(cr); err != nil {
			failCount++
			lastErr = err
		}
	}
	if failCount > 0 {
		log.Printf("Sync ES: %d cantor(s) failed (last error: %v)", failCount, lastErr)
		return
	}
	log.Println("Sync completed: All cantors pushed to Elasticsearch.")
}
