package finops

import (
	"fmt"
	"sync"
	"time"
)

// ScraperType defines the complexity of the operation
type ScraperType string

const (
	ScraperStatic    ScraperType = "static"
	ScraperHeuristic ScraperType = "heuristic"
	ScraperDiscovery ScraperType = "discovery" // e.g., Geocoding/URL Analysis
	ScraperLLM       ScraperType = "llm"       // AI-powered operations (offline Ollama by default)
)

// GlobalStats tracks real-time performance metrics
type GlobalStats struct {
	mu             sync.RWMutex
	TotalScrapes   int
	TotalDuration  time.Duration
	ExpensiveTasks []string
}

var Stats = &GlobalStats{}

// Record updates the statistics with the given cantor ID and task duration.
func (s *GlobalStats) Record(cantorID string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalScrapes++
	s.TotalDuration += duration
	if duration.Seconds() > 2.0 {
		s.ExpensiveTasks = append(s.ExpensiveTasks, fmt.Sprintf("%s (%v)", cantorID, duration.Round(time.Millisecond)))
		if len(s.ExpensiveTasks) > 10 {
			s.ExpensiveTasks = s.ExpensiveTasks[1:] // Keep last 10
		}
	}
}

// GetSummary returns a summary of the statistics.
func (s *GlobalStats) GetSummary() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	avg := 0.0
	if s.TotalScrapes > 0 {
		avg = s.TotalDuration.Seconds() / float64(s.TotalScrapes)
	}
	return map[string]any{
		"total_scrapes":    s.TotalScrapes,
		"avg_duration_sec": fmt.Sprintf("%.2fs", avg),
		"expensive_tasks":  s.ExpensiveTasks,
	}
}

// UnitCost represents the calculated cost for a single operation, aligned with FOCUS 1.0
type UnitCost struct {
	Time            time.Time
	ProviderID      string
	ScraperType     ScraperType
	Duration        time.Duration
	EstimatedCost   float64 // In USD
	TraceID         string
	ServiceCategory string // FOCUS: ServiceCategory (e.g., AI, Compute)
	ResourceName    string // FOCUS: ResourceName
}

// ResourceRates defines hourly costs for resources.
type ResourceRates struct {
	CPUCoreHour      float64
	RAMGBHour        float64
	HeuristicPremium float64
	ExternalAPICost  float64
	AITokenCost      float64 // Cost per AI request (estimated)
	// Legacy fields kept for API compatibility (historical DO profile)
	DropletStandardBasic float64
	LoadBalancerStandard float64
}

// DefaultRates mirrors local FOSS / OpenCost custom pricing (k8s/finops/values-opencost.yaml).
// Absolute USD is for relative ranking & guardrails — not a cloud invoice.
var DefaultRates = ResourceRates{
	CPUCoreHour:          0.020,
	RAMGBHour:            0.003,
	HeuristicPremium:     1.5,
	ExternalAPICost:      0.0001,
	AITokenCost:          0.00005, // local Ollama unit estimate (electricity/amortization, not cloud tokens)
	DropletStandardBasic: 0.00893, // legacy DO reference
	LoadBalancerStandard: 0.02232, // legacy DO reference
}

// Summary describes the active FinOps profile (local CNCF stack by default).
func (r ResourceRates) Summary(droplets, lbs int) string {
	_ = droplets
	_ = lbs
	return "FinOps Profile: local CNCF/FOSS (k3d + OpenCost) — $0 cloud bill; synthetic unit rates for allocation"
}

// CalculateTaskCost computes the real-time cost of an operation based on resource consumption
func (r ResourceRates) CalculateTaskCost(st ScraperType, duration time.Duration, cpuCores float64, memGB float64) float64 {
	seconds := duration.Seconds()
	hours := seconds / 3600.0

	baseCost := (hours * cpuCores * r.CPUCoreHour) + (hours * memGB * r.RAMGBHour)

	switch st {
	case ScraperHeuristic:
		baseCost *= r.HeuristicPremium
	case ScraperDiscovery:
		baseCost += r.ExternalAPICost
	case ScraperLLM:
		baseCost += r.AITokenCost
	}

	return baseCost
}

// OptimizationTip provides actionable advice
type OptimizationTip struct {
	Title       string
	Description string
	Potential   float64 // Estimated monthly savings
}

// GenerateTips analyzes the current state and suggests savings
func GenerateTips(dropletCount, lbCount int) []OptimizationTip {
	tips := []OptimizationTip{
		{
			Title:       "Prefer local OpenCost over paid APM for FinOps",
			Description: "Use task finops:bootstrap (Prometheus + OpenCost + Grafana) for allocation without a cloud or Datadog bill.",
			Potential:   0,
		},
		{
			Title:       "Keep LLM scrapers behind the governance circuit breaker",
			Description: "Heuristic/LLM paths dominate unit spend. Block providers that exceed the daily budget before adding capacity.",
			Potential:   0,
		},
	}
	if lbCount > 1 || dropletCount > 1 {
		tips = append(tips, OptimizationTip{
			Title:       "Legacy cloud profile detected",
			Description: "DO/AWS paths are archival. Prefer k3d + Compose unless free credits return.",
			Potential:   0,
		})
	}
	return tips
}

// Estimator interface for different storage/processing backends
type Estimator interface {
	Estimate(unit UnitCost) error
	GetProviderBurnRate(providerID string, lastNDays int) (float64, error)
}

// ScrapeMetric tracks the efficiency of data collection (Legacy compatibility)
type ScrapeMetric struct {
	CantorID     string
	Duration     time.Duration
	BytesFetched int
}
