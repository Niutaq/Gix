package finops

import (
	"context"
	"log"
	"sync"
	"time"
)

// GovernanceEngine is responsible for enforcing FinOps guardrails.
// It monitors burn rates and dynamically blocks operations that exceed budgets.
type GovernanceEngine struct {
	estimator *TimescaleEstimator
	mu        sync.RWMutex
	blocked   map[string]bool // map of ProviderID -> blocked status
	limitUSD  float64         // Maximum allowed spend per provider per 24h
}

// NewGovernanceEngine creates a new FinOps guardrail system.
func NewGovernanceEngine(estimator *TimescaleEstimator, dailyLimitUSD float64) *GovernanceEngine {
	return &GovernanceEngine{
		estimator: estimator,
		blocked:   make(map[string]bool),
		limitUSD:  dailyLimitUSD,
	}
}

// IsAllowed checks if a provider is permitted to execute an operation.
// This is an O(1) in-memory check, critical for high-throughput systems.
func (g *GovernanceEngine) IsAllowed(providerID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return !g.blocked[providerID]
}

// GetBlockedList returns the list of currently throttled ProviderIDs.
func (g *GovernanceEngine) GetBlockedList() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var list []string
	for id := range g.blocked {
		list = append(list, id)
	}
	return list
}

// StartMonitor begins a background routine that periodically evaluates burn rates.
func (g *GovernanceEngine) StartMonitor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				g.evaluateGuardrails()
			}
		}
	}()
}

func (g *GovernanceEngine) evaluateGuardrails() {
	// In a real scenario, you might query a list of active providers first.
	// We query TimescaleDB for providers that exceeded the limit in the last 1 day.
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT provider_id, SUM(estimated_cost_usd) as total_spend
		FROM provider_unit_costs
		WHERE time > NOW() - INTERVAL '1 day'
		GROUP BY provider_id
		HAVING SUM(estimated_cost_usd) > $1
	`
	
	rows, err := g.estimator.db.Query(ctx, query, g.limitUSD)
	if err != nil {
		log.Printf("[FinOps Governance] Failed to evaluate guardrails: %v", err)
		return
	}
	defer rows.Close()

	newBlocked := make(map[string]bool)
	var blockedCount int

	for rows.Next() {
		var providerID string
		var spend float64
		if err := rows.Scan(&providerID, &spend); err == nil {
			newBlocked[providerID] = true
			blockedCount++
			log.Printf("[FinOps Guardrail] ALARM: Provider %s exceeded daily budget ($%.4f). Circuit Breaker TRIPPED.", providerID, spend)
		}
	}

	g.mu.Lock()
	g.blocked = newBlocked
	g.mu.Unlock()

	if blockedCount > 0 {
		log.Printf("[FinOps Governance] Refreshed guardrails. %d providers are currently throttled.", blockedCount)
	}
}
