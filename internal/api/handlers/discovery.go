package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/internal/workers"
	"github.com/Niutaq/Gix/pkg/scrapers"
	"github.com/Niutaq/Gix/pkg/search"
	"github.com/Niutaq/Gix/pkg/types"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// HandleDiscover godoc
// @Summary      Discover New Cantor
// @Description  Attempts to discover exchange rates on a new URL using heuristic scraping and adds it to the database and Elasticsearch.
// @Tags         discovery
// @Accept       json
// @Produce      json
// @Param        discovery  body      map[string]string  true  "Discovery parameters (url, name, lat, lon)"
// @Success      201  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /discover [post]
func HandleDiscover(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL             string  `json:"url"`
			ExpectedName    string  `json:"expected_name,omitempty"`
			ExpectedLat     float64 `json:"expected_lat,omitempty"`
			ExpectedLon     float64 `json:"expected_lon,omitempty"`
			ExpectedAddress string  `json:"expected_address,omitempty"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		if req.URL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
			return
		}

		var info *scrapers.DiscoveredCantor
		discoveryStart := time.Now()

		if req.ExpectedName != "" {
			info = &scrapers.DiscoveredCantor{
				DisplayName: req.ExpectedName,
				Latitude:    req.ExpectedLat,
				Longitude:   req.ExpectedLon,
				Address:     req.ExpectedAddress,
			}
			log.Printf("Discovery: Using provided OSM metadata for %s", req.ExpectedName)
		} else {
			log.Printf("Discovery: Analyzing URL %s for metadata...", req.URL)
			var err error
			info, err = scrapers.HeuristicDiscoverCantor(req.URL)
			if err != nil {
				log.Printf("Discovery Metadata Error: %v", err)
				info = &scrapers.DiscoveredCantor{
					DisplayName: "New Discovered Cantor",
				}
			}

			if req.ExpectedLat != 0 || req.ExpectedLon != 0 {
				info.Latitude = req.ExpectedLat
				info.Longitude = req.ExpectedLon
			}
			if req.ExpectedAddress != "" {
				info.Address = req.ExpectedAddress
			}
		}

		discoveryDuration := time.Since(discoveryStart)

		if app.JS != nil {
			event := &pb.ScrapeCompletedEvent{
				ProviderId:  req.URL,
				ScraperType: "discovery",
				DurationMs:  discoveryDuration.Milliseconds(),
				Timestamp:   time.Now().Unix(),
			}
			protoBytes, _ := proto.Marshal(event)
			_, _ = app.JS.Publish("gix.scrape.v1.completed", protoBytes)
		}

		log.Printf("Discovery: Attempting heuristic scrape for %s...", req.URL)
		result, err := scrapers.HeuristicScrape(c.Request.Context(), req.URL, "EUR")

		scraperTypeUsed := "heuristic"
		if result.UsedScraperType != "" {
			scraperTypeUsed = result.UsedScraperType
		}

		if err != nil || result.BuyRate == "" || result.SellRate == "" {
			log.Printf("Discovery: Heuristic scrape failed for %s (will use placeholder rates): %v", req.URL, err)
			result.BuyRate = "0.0000"
			result.SellRate = "0.0000"
		} else {
			log.Printf("Discovery: Found rates using %s: Buy=%s, Sell=%s", scraperTypeUsed, result.BuyRate, result.SellRate)
		}

		if app.JS != nil {
			event := &pb.ScrapeCompletedEvent{
				ProviderId:  req.URL,
				ScraperType: scraperTypeUsed,
				DurationMs:  time.Since(discoveryStart).Milliseconds() - discoveryDuration.Milliseconds(),
				Timestamp:   time.Now().Unix(),
			}
			protoBytes, _ := proto.Marshal(event)
			_, _ = app.JS.Publish("gix.scrape.v1.completed", protoBytes)
		}
		var id int
		nameLower := strings.ToLower(info.DisplayName)
		err = app.DB.QueryRow(c.Request.Context(),
			"INSERT INTO cantors (name, display_name, base_url, strategy, latitude, longitude, address) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (name) DO UPDATE SET display_name = EXCLUDED.display_name, base_url = EXCLUDED.base_url, address = EXCLUDED.address RETURNING id",
			nameLower, info.DisplayName, req.URL, "HEURISTIC", info.Latitude, info.Longitude, info.Address).Scan(&id)

		if err != nil {
			log.Printf("Discovery DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save to database"})
			return
		}

		if app.Search != nil {
			cr := search.CantorRecord{
				ID:          id,
				Name:        strings.ToLower(info.DisplayName),
				DisplayName: info.DisplayName,
				Location:    types.GeoPoint{Lat: info.Latitude, Lon: info.Longitude},
			}
			_ = app.Search.IndexCantor(cr)
		}

		go func(newID int, displayName, baseURL string) {
			currencies := types.GlobalCurrencies
			ci := infrastructure.CantorInfo{
				ID:          newID,
				DisplayName: displayName,
				BaseURL:     baseURL,
				Strategy:    "HEURISTIC",
				Units:       1,
			}
			log.Printf("Starting post-discovery background harvest for new cantor: %s", displayName)
			var wg sync.WaitGroup
			sem := make(chan struct{}, 2)
			for _, curr := range currencies {
				wg.Add(1)
				go func(c string) {
					defer wg.Done()
					sem <- struct{}{}
					workers.ProcessCantorCurrency(context.Background(), app, ci, c)
					time.Sleep(1 * time.Second)
					<-sem
				}(curr)
			}
			wg.Wait()
			log.Printf("Finished post-discovery background harvest for: %s", displayName)
		}(id, info.DisplayName, req.URL)

		c.JSON(http.StatusCreated, gin.H{
			"status":   "discovered",
			"id":       id,
			"name":     info.DisplayName,
			"address":  info.Address,
			"lat":      info.Latitude,
			"lon":      info.Longitude,
			"buyRate":  result.BuyRate,
			"sellRate": result.SellRate,
		})
	}
}
