package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/gin-gonic/gin"
)

func HandleFinOps(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		summary := finops.Stats.GetSummary()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// App unit economics only — exclude OpenCost cluster mirror (esp. __unallocated__),
		// which was inflating Control Panel burn (e.g. ~$0.12+~$0.12 → $0.2466).
		const unitCostWhere = `time > NOW() - INTERVAL '1 day' AND provider_id NOT LIKE 'opencost:%'`

		var totalSpend float64
		err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM provider_unit_costs WHERE "+unitCostWhere).Scan(&totalSpend)
		if err != nil {
			log.Printf("FinOps DB Query Error: %v", err)
		}

		summary["real_spend_24h_usd"] = fmt.Sprintf("%.4f", totalSpend)
		summary["infrastructure"] = finops.DefaultRates.Summary(1, 1)
		summary["tips"] = finops.GenerateTips(0, 0)
		summary["stack"] = "cncf-foss-local" // k3d + OpenCost + Grafana

		if app.Governance != nil {
			summary["blocked_providers"] = app.Governance.GetBlockedList()
			summary["is_governance_active"] = true
		} else {
			summary["is_governance_active"] = false
		}

		summary["system_time"] = time.Now().Format(time.RFC3339)

		rows, err := app.DB.Query(ctx, "SELECT service_category, COALESCE(SUM(estimated_cost_usd), 0) FROM provider_unit_costs WHERE "+unitCostWhere+" GROUP BY service_category")
		if err == nil {
			breakdown := make(map[string]string)
			for rows.Next() {
				var cat string
				var cost float64
				if err := rows.Scan(&cat, &cost); err == nil {
					if cat == "" {
						cat = "Other"
					}
					breakdown[cat] = fmt.Sprintf("%.4f", cost)
				}
			}
			summary["service_breakdown"] = breakdown
			rows.Close()
		}

		c.JSON(http.StatusOK, summary)
	}
}

// HandleFinOpsFlow returns business → application → category cost allocation (FOCUS showback).
func HandleFinOpsFlow(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		type flowEdge struct {
			Business    string  `json:"business"`
			Application string  `json:"application"`
			Category    string  `json:"category"`
			Source      string  `json:"source"`
			USD         float64 `json:"usd"`
		}

		rows, err := app.DB.Query(ctx, `
			SELECT
				COALESCE(x_team, 'platform') AS business,
				COALESCE(x_application, 'gix') AS application,
				COALESCE(service_category, 'Other') AS category,
				COALESCE(source, 'unknown') AS source,
				COALESCE(SUM(effective_cost), 0)::float8 AS usd
			FROM focus_cost_records
			WHERE time > NOW() - INTERVAL '1 day'
			GROUP BY 1, 2, 3, 4
			ORDER BY usd DESC
		`)
		if err != nil {
			// Table may be empty / missing before first migration — fall back to unit costs.
			fallback, ferr := app.DB.Query(ctx, `
				SELECT
					'app' AS business,
					'gix' AS application,
					COALESCE(service_category, 'Other') AS category,
					'app-estimator' AS source,
					COALESCE(SUM(estimated_cost_usd), 0)::float8 AS usd
				FROM provider_unit_costs
				WHERE time > NOW() - INTERVAL '1 day'
				GROUP BY 3
				ORDER BY usd DESC
			`)
			if ferr != nil {
				log.Printf("FinOps flow fallback error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}
			defer fallback.Close()
			edges := make([]flowEdge, 0)
			for fallback.Next() {
				var e flowEdge
				if scanErr := fallback.Scan(&e.Business, &e.Application, &e.Category, &e.Source, &e.USD); scanErr == nil {
					edges = append(edges, e)
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"window": "24h",
				"edges":  edges,
				"note":   "focus_cost_records unavailable; showing provider_unit_costs fallback",
			})
			return
		}
		defer rows.Close()

		edges := make([]flowEdge, 0)
		var total float64
		for rows.Next() {
			var e flowEdge
			if scanErr := rows.Scan(&e.Business, &e.Application, &e.Category, &e.Source, &e.USD); scanErr == nil {
				edges = append(edges, e)
				total += e.USD
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"window":    "24h",
			"total_usd": total,
			"edges":     edges,
			"framework": "FOCUS",
			"stack":     "opencost+timescale",
		})
	}
}
