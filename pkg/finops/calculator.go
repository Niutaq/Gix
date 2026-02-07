package finops

import (
	// Standard libraries
	"fmt"
	"sync"
	"time"
)

// GlobalStats tracks real-time performance metrics
type GlobalStats struct {
	mu             sync.RWMutex
	TotalScrapes   int
	TotalDuration  time.Duration
	ExpensiveTasks []string // List of cantors that took > 2s
}

var Stats = &GlobalStats{}

// Record updates the statistics with the given cantor ID and task duration, tracking expensive tasks exceeding 2 seconds.
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

// GetSummary returns a summary of the statistics, including total scrapes, average duration, and expensive tasks.
func (s *GlobalStats) GetSummary() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	avg := 0.0
	if s.TotalScrapes > 0 {
		avg = s.TotalDuration.Seconds() / float64(s.TotalScrapes)
	}
	return map[string]interface{}{
		"total_scrapes":    s.TotalScrapes,
		"avg_duration_sec": fmt.Sprintf("%.2fs", avg),
		"expensive_tasks":  s.ExpensiveTasks,
	}
}

// ResourceRates defines hourly costs for DO resources based on standard pricing.
// These should ideally be fetched from the DigitalOcean Price API in the future.
type ResourceRates struct {
	DropletStandardBasic float64 // e.g., $6/mo -> ~$0.0089/h
	LoadBalancerStandard float64 // e.g., $15/mo -> ~$0.0223/h
	BlockStoragePerGB    float64 // e.g., $0.10/GB/mo
}

// DefaultRates provides baseline pricing (Standard DO prices as of early 2024)
var DefaultRates = ResourceRates{
	DropletStandardBasic: 0.00893,
	LoadBalancerStandard: 0.02232,
	BlockStoragePerGB:    0.10 / 672, // Hourly per GB
}

// CostReport represents a calculated cost breakdown
type CostReport struct {
	ResourceName string
	HourlySpend  float64
	MonthlySpend float64
}

// ScrapeMetric tracks the efficiency of data collection
type ScrapeMetric struct {
	CantorID     string
	Duration     time.Duration
	BytesFetched int
}

// CalculateEfficiency returns a score where lower is better (less CPU time per scrape)
func (m ScrapeMetric) CalculateEfficiency() float64 {
	return m.Duration.Seconds()
}

// OptimizationTip provides actionable advice
type OptimizationTip struct {
	Title       string
	Description string
	Potential   float64 // Estimated monthly savings
}

// GenerateTips analyzes the current state and suggests savings
func GenerateTips(lbCount int) []OptimizationTip {
	var tips []OptimizationTip

	// Tip 1: Load Balancer is expensive for small projects
	if lbCount > 0 {
		tips = append(tips, OptimizationTip{
			Title:       "Consolidate Load Balancer",
			Description: "Digital Ocean LBs cost $15/mo. Consider using a NodePort or HostPort Ingress if traffic is low.",
			Potential:   15.0,
		})
	}

	return tips
}

func (r ResourceRates) Summary(droplets, lbs int) string {
	hourly := (float64(droplets) * r.DropletStandardBasic) + (float64(lbs) * r.LoadBalancerStandard)
	return fmt.Sprintf("FinOps Profile: Estimated spend $%.2f/mo", hourly*672)
}
