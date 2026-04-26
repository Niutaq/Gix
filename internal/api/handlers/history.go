package handlers

import (
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func HandleGetHistory(app *infrastructure.AppState) gin.HandlerFunc {
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

func parseHistoryParams(c *gin.Context) infrastructure.HistoryParams {
	cantorIDStr := c.Query("cantor_id")
	daysStr := c.DefaultQuery("days", "7")
	days, _ := strconv.Atoi(daysStr)

	if days <= 0 {
		days = 7
	}

	cantorID, _ := strconv.Atoi(cantorIDStr)

	return infrastructure.HistoryParams{
		CantorID: cantorID,
		Days:     days,
		Cutoff:   time.Now().AddDate(0, 0, -days),
	}
}

func buildHistoryQuery(currency string, params infrastructure.HistoryParams) (string, []interface{}) {
	var query string
	var args []interface{}
	args = append(args, currency, params.Cutoff)
	if params.CantorID > 0 {
		query = `
				SELECT time_bucket('1 hour', time) AS bucket,
					   AVG(buy_rate)::FLOAT,
					   AVG(sell_rate)::FLOAT
				FROM rates
				WHERE currency = $1 AND time > $2 AND cantor_id = $3
				GROUP BY bucket
				ORDER BY bucket ASC`
		args = append(args, params.CantorID)
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
			BuyRate:  int64(math.Round(buy * infrastructure.MoneyMultiplier)),
			SellRate: int64(math.Round(sell * infrastructure.MoneyMultiplier)),
		})
	}
	return points
}

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
