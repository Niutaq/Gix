package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

const contentTypeProtoBuf = "application/x-protobuf"

// HandleGetRates godoc
// @Summary      Get Rates
// @Description  Returns the buy and sell rates for a specific cantor and currency. Scrapes data in real-time if not cached.
// @Tags         rates
// @Produce      json
// @Param        cantor_id  query     int     true  "Cantor ID"
// @Param        currency   query     string  true  "Currency Code (e.g., EUR, USD)"
// @Success      200  {object}  pb.RateResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /rates [get]
func HandleGetRates(app *infrastructure.AppState) gin.HandlerFunc {
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

		cantorInfo, err := services.FetchCantorInfo(ctx, app.DB, cantorID)
		if err != nil {
			handleDBError(c, err)
			return
		}

		response, rates, err := services.ScrapeAndProcess(ctx, app, cantorInfo, cantorID, currency)
		if err != nil {
			log.Printf("Processing Error (%s): %v", cantorInfo.Strategy, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		services.CacheAndArchive(ctx, app, cacheKey, cantorID, currency, response, rates)

		if c.GetHeader("Accept") == contentTypeProtoBuf {
			c.ProtoBuf(http.StatusOK, response)
		} else {
			c.JSON(http.StatusOK, response)
		}
	}
}

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

func handleDBError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cantor not found"})
	} else {
		log.Printf("DB Error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
	}
}
