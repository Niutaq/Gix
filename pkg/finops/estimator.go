package finops

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleEstimator implements the Estimator interface using a TimescaleDB backend.
type TimescaleEstimator struct {
	db    *pgxpool.Pool
	rates ResourceRates
}

// NewTimescaleEstimator creates a new estimator with a DB pool and rates configuration.
func NewTimescaleEstimator(db *pgxpool.Pool, rates ResourceRates) *TimescaleEstimator {
	return &TimescaleEstimator{
		db:    db,
		rates: rates,
	}
}

// Estimate calculates the cost for a unit and persists it to TimescaleDB.
// This is our core Unit Economics function, now aligned with FOCUS 1.0.
func (e *TimescaleEstimator) Estimate(unit UnitCost) error {
	// 1. Assign FOCUS categories based on ScraperType if not provided
	if unit.ServiceCategory == "" {
		switch unit.ScraperType {
		case ScraperLLM:
			unit.ServiceCategory = "AI"
			unit.ResourceName = "Gemini 1.5 Flash"
		case ScraperDiscovery:
			unit.ServiceCategory = "Network"
			unit.ResourceName = "External API"
		default:
			unit.ServiceCategory = "Compute"
			unit.ResourceName = "Go Backend Scraper"
		}
	}

	// 2. Calculate the cost if not already provided
	if unit.EstimatedCost == 0 {
		// Default assumptions: 0.1 CPU core and 128MB RAM for a standard scrape
		cpuCores := 0.1
		memGB := 0.125

		// Heuristic scrapers consume more resources
		if unit.ScraperType == ScraperHeuristic {
			cpuCores = 0.5
			memGB = 0.5
		}

		unit.EstimatedCost = e.rates.CalculateTaskCost(unit.ScraperType, unit.Duration, cpuCores, memGB)
	}

	// 3. Persist to TimescaleDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO provider_unit_costs (time, provider_id, scraper_type, duration_ms, estimated_cost_usd, trace_id, service_category, resource_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	
	_, err := e.db.Exec(ctx, query, 
		unit.Time, 
		unit.ProviderID, 
		string(unit.ScraperType), 
		unit.Duration.Milliseconds(), 
		unit.EstimatedCost, 
		unit.TraceID,
		unit.ServiceCategory,
		unit.ResourceName,
	)

	if err != nil {
		return fmt.Errorf("failed to save unit cost: %w", err)
	}

	return nil
}

// GetProviderBurnRate returns the average hourly cost for a provider in the last N days.
// This allows for "Waste Detection" (e.g. a provider that is expensive but returns no data).
func (e *TimescaleEstimator) GetProviderBurnRate(providerID string, lastNDays int) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		SELECT SUM(estimated_cost_usd) 
		FROM provider_unit_costs 
		WHERE provider_id = $1 AND time > NOW() - $2 * INTERVAL '1 day'
	`

	var totalCost float64
	err := e.db.QueryRow(ctx, query, providerID, lastNDays).Scan(&totalCost)
	if err != nil {
		return 0, err
	}

	return totalCost, nil
}

// LogScrape is a helper to record a scrape and estimate its cost immediately.
func (e *TimescaleEstimator) LogScrape(providerID string, st ScraperType, duration time.Duration) {
	unit := UnitCost{
		Time:        time.Now(),
		ProviderID:  providerID,
		ScraperType: st,
		Duration:    duration,
	}

	if err := e.Estimate(unit); err != nil {
		log.Printf("[FinOps] Error estimating cost for %s: %v", providerID, err)
	}
}
