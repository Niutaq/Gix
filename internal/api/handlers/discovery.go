package handlers

import (
	"context"
	"fmt"
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
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
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
		if err := scrapers.ValidateURL(req.URL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsafe or invalid URL"})
			return
		}
		req.ExpectedName = sanitizeDisplayName(req.ExpectedName)
		req.ExpectedAddress = sanitizeAddressField(req.ExpectedAddress)
		if err := validateDiscoverMeta(req.ExpectedName, req.ExpectedAddress, req.ExpectedLat, req.ExpectedLon); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid metadata"})
			return
		}
		// FinOps gate by stable cantor id when URL already known (never by raw URL).
		if app.Governance != nil {
			if existingID, ok := lookupCantorIDByURL(c.Request.Context(), app, req.URL); ok {
				if !app.Governance.IsAllowed(fmt.Sprintf("%d", existingID)) {
					c.JSON(http.StatusTooManyRequests, gin.H{"error": "provider is currently blocked due to exceeding FinOps budget"})
					return
				}
			}
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
			info, err = scrapers.HeuristicDiscoverCantorWithContext(c.Request.Context(), req.URL)
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
		info.DisplayName = sanitizeDisplayName(info.DisplayName)
		info.Address = sanitizeAddressField(info.Address)

		discoveryDuration := time.Since(discoveryStart)

		log.Printf("Discovery: Attempting heuristic scrape for %s...", req.URL)
		result, err := scrapers.HeuristicScrape(c.Request.Context(), req.URL, "EUR")

		scraperTypeUsed := "heuristic"
		if result.UsedScraperType != "" {
			scraperTypeUsed = result.UsedScraperType
		}

		if err != nil || result.BuyRate == "" || result.SellRate == "" {
			log.Printf("Discovery: Heuristic scrape failed for %s: %v", req.URL, err)
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not scrape exchange rates from URL"})
			return
		}
		if ok, buy, sell := scrapers.SanitizePairForCurrency("EUR", result.BuyRate, result.SellRate); !ok {
			log.Printf("Discovery: rejected out-of-band EUR pair for %s: %s/%s", req.URL, result.BuyRate, result.SellRate)
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "could not scrape exchange rates from URL"})
			return
		} else {
			result.BuyRate, result.SellRate = buy, sell
			log.Printf("Discovery: Found rates using %s: Buy=%s, Sell=%s", scraperTypeUsed, result.BuyRate, result.SellRate)
		}

		var id int
		// Unique slug from URL host (+ coords) — generic OSM names like "Kantor" must not collide.
		uniqueName := cantorSlug(req.URL, info.DisplayName, info.Latitude, info.Longitude)
		displayName := strings.TrimSpace(info.DisplayName)
		if displayName == "" || strings.EqualFold(displayName, "kantor") || strings.EqualFold(displayName, "new discovered cantor") {
			displayName = humanizeHost(req.URL)
		}
		err = app.DB.QueryRow(c.Request.Context(),
			"INSERT INTO cantors (name, display_name, base_url, strategy, latitude, longitude, address) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (name) DO UPDATE SET display_name = EXCLUDED.display_name, base_url = EXCLUDED.base_url, latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude, address = EXCLUDED.address RETURNING id",
			uniqueName, displayName, req.URL, "HEURISTIC", info.Latitude, info.Longitude, info.Address).Scan(&id)

		if err != nil {
			log.Printf("Discovery DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save to database"})
			return
		}

		providerID := fmt.Sprintf("%d", id)
		if app.Governance != nil && !app.Governance.IsAllowed(providerID) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "provider is currently blocked due to exceeding FinOps budget"})
			return
		}
		if app.JS != nil {
			publishScrapeEvent(c.Request.Context(), app, providerID, "discovery", discoveryDuration)
			publishScrapeEvent(c.Request.Context(), app, providerID, scraperTypeUsed, time.Since(discoveryStart)-discoveryDuration)
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
			// FinOps: harvest majors first — full GlobalCurrencies burns budget + LLM slots.
			currencies := []string{"EUR", "USD", "GBP", "CHF"}
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
		}(id, displayName, req.URL)

		c.JSON(http.StatusCreated, gin.H{
			"status":   "discovered",
			"id":       id,
			"name":     displayName,
			"address":  info.Address,
			"lat":      info.Latitude,
			"lon":      info.Longitude,
			"buyRate":  result.BuyRate,
			"sellRate": result.SellRate,
		})
	}
}

func cantorSlug(rawURL, display string, lat, lon float64) string {
	host := humanizeHost(rawURL)
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-':
			return r
		default:
			return '-'
		}
	}, host)
	if host == "" {
		host = "cantor"
	}
	return fmt.Sprintf("%s_%.4f_%.4f", host, lat, lon)
}

func humanizeHost(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if u == "" {
		return "Kantor"
	}
	return u
}

func lookupCantorIDByURL(ctx context.Context, app *infrastructure.AppState, rawURL string) (int, bool) {
	var id int
	err := app.DB.QueryRow(ctx, "SELECT id FROM cantors WHERE base_url = $1 LIMIT 1", rawURL).Scan(&id)
	if err != nil {
		return 0, false
	}
	return id, true
}

func publishScrapeEvent(ctx context.Context, app *infrastructure.AppState, providerID, scraperType string, duration time.Duration) {
	event := &pb.ScrapeCompletedEvent{
		ProviderId:  providerID,
		ScraperType: scraperType,
		DurationMs:  duration.Milliseconds(),
		Timestamp:   time.Now().Unix(),
		TraceId:     handlerTraceID(ctx),
	}
	protoBytes, err := proto.Marshal(event)
	if err != nil {
		log.Printf("FinOps Event Marshal Error: %v", err)
		return
	}
	if _, err := app.JS.Publish("gix.scrape.v1.completed", protoBytes); err != nil {
		log.Printf("FinOps Event Publish Error: %v", err)
	}
}

func handlerTraceID(ctx context.Context) string {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok || span == nil {
		return ""
	}
	return fmt.Sprintf("%d", span.Context().TraceID())
}
