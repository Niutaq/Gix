package utilities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"github.com/PuerkitoBio/goquery"
)

// TriggerDiscovery is the main entry point for finding new cantors in a city or chunk.
func TriggerDiscovery(state *AppState, config AppConfig, lat, lon float64) {
	chunkKey := fmt.Sprintf("%.2f,%.2f", lat, lon)

	state.CantorsMu.Lock()
	if state.UI.ScannedChunks == nil {
		state.UI.ScannedChunks = make(map[string]bool)
	}
	if state.UI.ScannedChunks[chunkKey] {
		state.CantorsMu.Unlock()
		return // Already scanned
	}
	state.UI.ScannedChunks[chunkKey] = true
	state.CantorsMu.Unlock()

	log.Printf("Discovery: Exploring new chunk at %s", chunkKey)

	// Step 1: Real discovery using OpenStreetMap (Nominatim)
	searchURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=kantor+near+[%f,%f]&format=json&addressdetails=1&limit=5", lat, lon)

	go func() {
		req, _ := http.NewRequest("GET", searchURL, nil)
		req.Header.Set("User-Agent", "Gix-App/1.0")
		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Discovery: OSM search failed: %v", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		type OSMResult struct {
			DisplayName string `json:"display_name"`
			Lat         string `json:"lat"`
			Lon         string `json:"lon"`
			// Note: OSM doesn't always provide the website in the short search result,
			// but we can try to find it or use the name to search.
		}
		var results []OSMResult
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			log.Printf("Discovery: Failed to decode OSM results: %v", err)
			return
		}

		for _, res := range results {
			cleanName := strings.Split(res.DisplayName, ",")[0]
			log.Printf("Discovery: Found real location: %s", cleanName)

			// Generate a unique key for OSM results to avoid duplicates
			osmKey := fmt.Sprintf("osm_%s", res.Lat[:5]+res.Lon[:5])

			state.CantorsMu.Lock()
			if state.Cantors[osmKey] == nil {
				cLat, _ := strconv.ParseFloat(res.Lat, 64)
				cLon, _ := strconv.ParseFloat(res.Lon, 64)
				state.Cantors[osmKey] = &CantorInfo{
					ID:          0,
					DisplayName: cleanName,
					Latitude:    cLat,
					Longitude:   cLon,
					Strategy:    "OSM",
				}
			}
			state.CantorsMu.Unlock()
		}

		if state.Window != nil {
			state.Window.Invalidate()
		}
	}()
}

// triggerHeuristicDiscovery calls the backend /discover API with the provided URL.
// Returns true if discovery was successful and the cantor was added.
func triggerHeuristicDiscovery(url string, expectedName string, expectedLat, expectedLon float64, expectedAddress string, state *AppState, config AppConfig, window *app.Window) bool {
	log.Printf("Triggering Heuristic Discovery for: %s", url)

	payload := map[string]interface{}{
		"url":              url,
		"expected_name":    expectedName,
		"expected_lat":     expectedLat,
		"expected_lon":     expectedLon,
		"expected_address": expectedAddress,
	}
	body, _ := json.Marshal(payload)

	apiURL := config.APIDiscoverURL

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Heuristic Discovery Request Error: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 120 * time.Second} // Scraping with LLM fallback takes longer
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Heuristic Discovery Error: %v", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		log.Printf("Heuristic Discovery failed with status: %d", resp.StatusCode)
		return false
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Heuristic Discovery JSON Error: %v", err)
		return false
	}

	log.Printf("Heuristic Discovery Success: %v", result)

	// Add to UI state immediately so it shows up on map and list
	name, _ := result["name"].(string)
	addr, _ := result["address"].(string)
	lat, _ := result["lat"].(float64)
	lon, _ := result["lon"].(float64)
	idRaw, _ := result["id"].(float64)
	idStr := fmt.Sprintf("%d", int(idRaw))

	if addr == "" {
		addr = "Discovered automatically"
	}

	state.CantorsMu.Lock()
	if state.Cantors[idStr] == nil {
		state.Cantors[idStr] = &CantorInfo{
			ID:          int(idRaw),
			DisplayName: name,
			Latitude:    lat,
			Longitude:   lon,
			Address:     addr,
		}
	} else {
		// Update existing cantor info
		state.Cantors[idStr].DisplayName = name
		state.Cantors[idStr].Latitude = lat
		state.Cantors[idStr].Longitude = lon
		state.Cantors[idStr].Address = addr
	}
	state.CantorsMu.Unlock()

	// If no coordinates were found, don't jump to (0,0), but if we have them, jump!
	if lat != 0 && lon != 0 {
		state.UI.MapFocus.Latitude = lat
		state.UI.MapFocus.Longitude = lon
		state.UI.MapFocus.CityName = name
	}

	// Also mock some initial rate in Vault so it shows up
	buyRate, _ := result["buyRate"].(string)
	sellRate, _ := result["sellRate"].(string)

	if br, ok := result["buyRate"].(float64); ok {
		buyRate = fmt.Sprintf("%.4f", br)
	} else if br, ok := result["buyRate"].(string); ok {
		buyRate = br
	}

	if sr, ok := result["sellRate"].(float64); ok {
		sellRate = fmt.Sprintf("%.4f", sr)
	} else if sr, ok := result["sellRate"].(string); ok {
		sellRate = sr
	}

	if buyRate != "" || sellRate != "" {
		state.Vault.Mu.Lock()

		// The backend explicitly scraped "EUR" in handleDiscover,
		// so we mock the EUR rate regardless of current UI currency.
		targetCurrency := "EUR"

		if state.Vault.Rates[targetCurrency] == nil {
			state.Vault.Rates[targetCurrency] = make(map[string]*CantorEntry)
		}

		entry := &CantorEntry{
			Rate: ExchangeRates{
				BuyRate:  buyRate,
				SellRate: sellRate,
			},
			UpdatePulse:      1.0,
			AppearanceSpring: Spring{Current: 1, Target: 1, Tension: 150, Friction: 22},
		}
		RefreshDisplayStrings(entry)
		state.Vault.Rates[targetCurrency][idStr] = entry

		state.Vault.Mu.Unlock()
	}

	state.UI.FilteredIDs = nil // Force list re-filter so it shows up immediately
	window.Invalidate()

	return true
}

// TriggerDeleteCantor calls the backend /cantors/:id API with the DELETE method.
func TriggerDeleteCantor(state *AppState, config AppConfig, cantorKey string, window *app.Window) {
	log.Printf("Triggering Delete for Cantor: %s", cantorKey)

	state.CantorsMu.RLock()
	cantor, ok := state.Cantors[cantorKey]
	state.CantorsMu.RUnlock()
	if !ok {
		return
	}

	apiURL := fmt.Sprintf("%s/%d", config.APICantorsURL, cantor.ID)

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		log.Printf("Delete Cantor Request Error: %v", err)
		return
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Delete Cantor Error: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		log.Printf("Successfully deleted cantor %s from DB.", cantorKey)
		// Remove from UI state
		state.CantorsMu.Lock()
		delete(state.Cantors, cantorKey)
		state.CantorsMu.Unlock()

		state.Vault.Mu.Lock()
		for currency := range state.Vault.Rates {
			delete(state.Vault.Rates[currency], cantorKey)
		}
		state.Vault.Mu.Unlock()

		state.UI.FilteredIDs = nil // Force re-filter
		if state.UI.SelectedCantor == cantorKey {
			state.UI.SelectedCantor = ""
		}
		window.Invalidate()
	} else {
		log.Printf("Delete Cantor failed with status: %d", resp.StatusCode)
	}
}

// LLMDiscoverCityCantors finds physical cantors via Nominatim and their URLs via DDG
func LLMDiscoverCityCantors(city string, state *AppState, config AppConfig, window *app.Window) {
	city = strings.TrimSpace(city)
	if city == "" {
		return
	}

	state.CantorsMu.Lock()
	if state.UI.ScannedChunks == nil {
		state.UI.ScannedChunks = make(map[string]bool)
	}
	// Always allow re-scanning the city by removing the early return block.
	state.UI.ScannedChunks[strings.ToLower(city)] = true
	state.CantorsMu.Unlock()

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	// Disable GPS lock when searching for a new city so the newly discovered cantors aren't hidden by the distance filter
	state.UI.UserLocation.Active = false
	state.UI.FilteredIDs = nil

	if window != nil {
		window.Invalidate()
	}

	defer func() {
		state.IsLoading.Store(false)
		if window != nil {
			window.Invalidate()
		}
	}()

	log.Printf("Starting Nominatim discovery for city/area: %s", city)

	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Geocode city
	geocodeURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1", url.QueryEscape(city))
	reqCity, _ := http.NewRequest("GET", geocodeURL, nil)
	reqCity.Header.Set("User-Agent", "Gix-App/1.0")
	respCity, err := client.Do(reqCity)
	if err != nil {
		log.Printf("Geocode Error: %v", err)
		if state.UI.SearchEditor.Text() == city {
			state.UI.SearchEditor.SetText(GetTranslation(state.UI.Language, "err_api_connection"))
		}
		return
	}
	defer func() { _ = respCity.Body.Close() }()

	var geoResults []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.NewDecoder(respCity.Body).Decode(&geoResults); err != nil || len(geoResults) == 0 {
		log.Printf("Could not find coordinates for city: %s", city)
		if state.UI.SearchEditor.Text() == city {
			state.UI.SearchEditor.SetText("Nie znaleziono miasta!")
		}
		return
	}

	lat, _ := strconv.ParseFloat(geoResults[0].Lat, 64)
	lon, _ := strconv.ParseFloat(geoResults[0].Lon, 64)

	state.UI.MapFocus.Latitude = lat
	state.UI.MapFocus.Longitude = lon
	state.UI.MapFocus.CityName = city
	if window != nil {
		window.Invalidate()
	}

	// Step 2: Find cantors near the city using Overpass API (radius: 15km, expanding to 30km if no results)
	type OverpassElement struct {
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Center struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"center"`
		Tags map[string]string `json:"tags"`
	}
	type OverpassResponse struct {
		Elements []OverpassElement `json:"elements"`
	}

	var cantors []OverpassElement
	radius := 15000

	// Focus on the most reliable endpoints to avoid long hangs on 504s
	endpoints := []string{
		"https://overpass-api.de/api/interpreter",
		"https://lz4.overpass-api.de/api/interpreter",
	}

	for radius <= 30000 {
		query := fmt.Sprintf(`[out:json][timeout:10];(nwr["amenity"="bureau_de_change"](around:%d, %f, %f););out center;`, radius, lat, lon)
		formData := url.Values{}
		formData.Set("data", query)

		success := false
		for _, endpoint := range endpoints {
			// Shorter timeout for individual Overpass requests
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			reqOverpass, _ := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(formData.Encode()))
			reqOverpass.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			reqOverpass.Header.Set("User-Agent", "Gix-App/1.0")

			respOverpass, err := client.Do(reqOverpass)
			cancel()

			if err != nil {
				log.Printf("Overpass API Error (endpoint: %s, radius %d): %v", endpoint, radius, err)
				continue
			}

			if respOverpass.StatusCode != http.StatusOK {
				log.Printf("Overpass API HTTP %d from %s (Service might be overloaded)", respOverpass.StatusCode, endpoint)
				_ = respOverpass.Body.Close()
				continue
			}

			var overpassResp OverpassResponse
			if err := json.NewDecoder(respOverpass.Body).Decode(&overpassResp); err == nil {
				if len(overpassResp.Elements) > 0 {
					cantors = overpassResp.Elements
				}
				success = true
				_ = respOverpass.Body.Close()
				break
			}
			_ = respOverpass.Body.Close()
			}

		if success && len(cantors) > 0 {
			break
		}
		if !success && radius == 30000 {
			log.Printf("Discovery: All Overpass endpoints failed or timed out.")
			if state.UI.SearchEditor.Text() == city {
				state.UI.SearchEditor.SetText(GetTranslation(state.UI.Language, "err_discovery_failed"))
			}
		}

		radius += 15000
	}

	if len(cantors) == 0 {
		log.Printf("No real cantors found near %s", city)
		if state.UI.SearchEditor.Text() == city {
			state.UI.SearchEditor.SetText(GetTranslation(state.UI.Language, "err_no_cantors_found"))
		}
		return
	}

	// Calculate distance and sort
	haversine := func(lat1, lon1, lat2, lon2 float64) float64 {
		const R = 6371.0 // Earth radius in km
		dLat := (lat2 - lat1) * math.Pi / 180.0
		dLon := (lon2 - lon1) * math.Pi / 180.0
		lat1Rad := lat1 * math.Pi / 180.0
		lat2Rad := lat2 * math.Pi / 180.0
		a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
		c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
		return R * c
	}

	type CantorWithDistance struct {
		Element  OverpassElement
		Distance float64
	}
	var cantorsWithDist []CantorWithDistance
	for _, c := range cantors {
		if c.Lat == 0 && c.Center.Lat != 0 {
			c.Lat = c.Center.Lat
			c.Lon = c.Center.Lon
		}

		alreadyExists := false
		state.CantorsMu.RLock()
		for _, existing := range state.Cantors {
			if existing.Latitude == 0 || existing.Longitude == 0 {
				continue
			}
			distExisting := haversine(c.Lat, c.Lon, existing.Latitude, existing.Longitude)
			if distExisting < 0.05 {
				alreadyExists = true
				break
			}
		}
		state.CantorsMu.RUnlock()

		if alreadyExists {
			continue
		}

		dist := haversine(lat, lon, c.Lat, c.Lon)
		cantorsWithDist = append(cantorsWithDist, CantorWithDistance{Element: c, Distance: dist})
	}

	sort.Slice(cantorsWithDist, func(i, j int) bool {
		return cantorsWithDist[i].Distance < cantorsWithDist[j].Distance
	})

	// Take top 5
	limit := 5
	if len(cantorsWithDist) < limit {
		limit = len(cantorsWithDist)
	}
	topCantors := cantorsWithDist[:limit]

	// For each cantor, find or scrape URL
	var wg sync.WaitGroup
	for i, cw := range topCantors {
		c := cw.Element
		cLat := c.Lat
		cLon := c.Lon
		cName := c.Tags["name"]
		if cName == "" || len(cName) <= 3 {
			cName = "" // Let heuristic extraction find the real name
		}

		var addrParts []string
		if c.Tags["addr:street"] != "" {
			addrParts = append(addrParts, c.Tags["addr:street"])
		}
		if c.Tags["addr:housenumber"] != "" {
			addrParts = append(addrParts, c.Tags["addr:housenumber"])
		}
		if c.Tags["addr:city"] != "" {
			addrParts = append(addrParts, c.Tags["addr:city"])
		}
		cAddr := strings.Join(addrParts, ", ")
		if cAddr == "" {
			cAddr = cName
		}

		website := c.Tags["website"]
		if website == "" {
			website = c.Tags["contact:website"]
		}

		if website != "" {
			wg.Add(1)
			go func(urlStr, name string, lt, ln float64, addr string) {
				defer wg.Done()
				triggerHeuristicDiscovery(urlStr, name, lt, ln, addr, state, config, window)
			}(website, cName, cLat, cLon, cAddr)
		} else { // Find via DDG using precise cantor name
			wg.Add(1)
			go func(name, cityName string, lt, ln float64, addr string, index int) {
				defer wg.Done()
				time.Sleep(time.Duration(index) * 2 * time.Second) // Stagger requests to avoid DDG limits

				searchTerm := name
				if searchTerm == "" {
					if addr == "" {
						searchTerm = "Kantor " + cityName
					} else {
						searchTerm = "Kantor " + addr + " " + cityName
					}
				} else {
					searchTerm = name + " " + cityName
				}
				searchQuery := url.QueryEscape(fmt.Sprintf("%s kursy walut", searchTerm))

				ddgReq, _ := http.NewRequest("GET", "https://html.duckduckgo.com/html/?q="+searchQuery, nil)
				ddgReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
				ddgResp, err := client.Do(ddgReq)
				if err != nil {
					return
				}
				defer func() { _ = ddgResp.Body.Close() }()

				doc, err := goquery.NewDocumentFromReader(ddgResp.Body)
				if err != nil {
					return
				}

				doc.Find("a.result__url").EachWithBreak(func(i int, s *goquery.Selection) bool {
					href, exists := s.Attr("href")
					if !exists {
						return true
					}

					// The href might look like "//duckduckgo.com/l/?uddg=https%3A%2F%2Fkantor..."
					var decoded string
					if strings.Contains(href, "uddg=") {
						parts := strings.Split(href, "uddg=")
						if len(parts) > 1 {
							decoded, _ = url.QueryUnescape(strings.Split(parts[1], "&")[0])
						}
					} else {
						// Sometimes DDG returns direct URLs
						decoded = href
						if strings.HasPrefix(decoded, "//") {
							decoded = "https:" + decoded
						}
					}

					if decoded == "" {
						return true
					}

					decodedLower := strings.ToLower(decoded)

					// Skip aggregators, directories, maps, social media, and crypto exchanges
					excludedDomains := []string{
						"facebook.com", "instagram.com", "google.com", "linkedin.com",
						"kantor.pl", "quantor.pl", "tavex.pl", "kantor.live", "kantroom.pl",
						"kantorywsieci.pl", "marketportal.pl", "zlata.ws", "dobrykantor.pl", "wymieniarka.pl",
						"mapy.com", "targeo.pl", "zumi.pl", "e-mapa.net", "osm.org", "openstreetmap.org",
						"pkt.pl", "panoramafirm.pl", "biznesfinder.pl", "cylex-polska.pl", "gowork.pl",
						"biznes.gov.pl", "aleo.com", "orly.pl", "baza-firm.com.pl", "katalog.wp.pl",
						"kursarz.pl", "strefawalut.pl", "revieweuro.com", "oferteo.pl", "cinkciarz.pl",
						"walutomat.pl", "internetowykantor.pl", "amronet.pl", "liderwalut.pl", "money.pl",
						"bankier.pl", "najlepszekantory.pl", "kursy-walut.pl", "kantory.pl", "katalog.onet.pl",
						"przeliczwalut.pl", "firmania.pl", "aliorbank.pl", "kryptokurier.pl", "cashify.eu",
						"krypto", "crypto", "bitcoin", "bithub", "coin", "mennica",
					}

					isExcluded := false
					for _, domain := range excludedDomains {
						if strings.Contains(decodedLower, domain) {
							isExcluded = true
							break
						}
					}

					if !isExcluded {
						success := triggerHeuristicDiscovery(decoded, name, lt, ln, addr, state, config, window)
						if success {
							return false // Stop at the first plausible specific URL that worked
						}
					}

					return true // Continue checking other results
				})
			}(cName, city, cLat, cLon, cAddr, i)
		}
	}
	wg.Wait()
}
