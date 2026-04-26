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

		var totalSpend float64
		err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM provider_unit_costs WHERE time > NOW() - INTERVAL '1 day'").Scan(&totalSpend)
		if err != nil {
			log.Printf("FinOps DB Query Error: %v", err)
		}

		summary["real_spend_24h_usd"] = fmt.Sprintf("%.8f", totalSpend)
		summary["infrastructure"] = finops.DefaultRates.Summary(1, 1)
		summary["tips"] = finops.GenerateTips(1, 1)

		if app.Governance != nil {
			summary["blocked_providers"] = app.Governance.GetBlockedList()
			summary["is_governance_active"] = true
		} else {
			summary["is_governance_active"] = false
		}

		summary["system_time"] = time.Now().Format(time.RFC3339)

		rows, err := app.DB.Query(ctx, "SELECT service_category, COALESCE(SUM(estimated_cost_usd), 0) FROM provider_unit_costs WHERE time > NOW() - INTERVAL '1 day' GROUP BY service_category")
		if err == nil {
			breakdown := make(map[string]string)
			for rows.Next() {
				var cat string
				var cost float64
				if err := rows.Scan(&cat, &cost); err == nil {
					if cat == "" {
						cat = "Other"
					}
					breakdown[cat] = fmt.Sprintf("%.8f", cost)
				}
			}
			summary["service_breakdown"] = breakdown
			rows.Close()
		}

		c.JSON(http.StatusOK, summary)
	}
}
