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
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"github.com/Niutaq/Gix/pkg/scrapers"
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
		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			log.Printf("Discovery: failed to build OSM request: %v", err)
			return
		}
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
			osmKey := fmt.Sprintf("osm_%s_%s", safeCoordKey(res.Lat), safeCoordKey(res.Lon))

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
	if key := strings.TrimSpace(os.Getenv("GIX_API_KEY")); key != "" {
		req.Header.Set("X-Gix-API-Key", key)
	}

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
	// Remove nearby OSM placeholder pins — replaced by the real discovered cantor.
	for key, existing := range state.Cantors {
		if existing == nil {
			continue
		}
		if strings.HasPrefix(key, "osm_") || strings.EqualFold(existing.Strategy, "OSM") {
			if lat != 0 && lon != 0 && existing.Latitude != 0 && existing.Longitude != 0 {
				if CalculateDistance(lat, lon, existing.Latitude, existing.Longitude) < 0.15 {
					delete(state.Cantors, key)
				}
			}
		}
	}
	if state.Cantors[idStr] == nil {
		state.Cantors[idStr] = &CantorInfo{
			ID:          int(idRaw),
			DisplayName: name,
			Latitude:    lat,
			Longitude:   lon,
			Address:     addr,
			Strategy:    "HEURISTIC",
		}
	} else {
		existing := state.Cantors[idStr]
		state.Cantors[idStr] = &CantorInfo{
			ID:                 int(idRaw),
			DisplayName:        name,
			Latitude:           lat,
			Longitude:          lon,
			Address:            addr,
			Strategy:           "HEURISTIC",
			Button:             existing.Button,
			DeleteBtn:          existing.DeleteBtn,
			PressStart:         existing.PressStart,
			IsPressing:         existing.IsPressing,
			LongPressTriggered: existing.LongPressTriggered,
		}
	}
	state.CantorsMu.Unlock()

	// Jump map only for bare URL discovery — never clobber an active city search focus.
	if lat != 0 && lon != 0 && state.UI.MapFocus.CityName == "" {
		state.UI.MapFocus.Latitude = lat
		state.UI.MapFocus.Longitude = lon
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
			UpdatePulse:      1.0,
			AppearanceSpring: Spring{Current: 1, Target: 1, Tension: 150, Friction: 22},
		}
		if ok, b, s := scrapers.SanitizePairForCurrency(targetCurrency, buyRate, sellRate); ok {
			entry.Rate = ExchangeRates{BuyRate: b, SellRate: s}
		} else {
			entry.Error = "bad_rate"
		}
		RefreshDisplayStrings(entry)
		state.Vault.Rates[targetCurrency][idStr] = entry

		state.Vault.Mu.Unlock()
	}

	state.UI.FilteredIDs = nil // Force list re-filter so it shows up immediately
	window.Invalidate()

	return true
}

func removeCantorLocal(state *AppState, cantorKey string) {
	state.CantorsMu.Lock()
	delete(state.Cantors, cantorKey)
	state.CantorsMu.Unlock()

	state.Vault.Mu.Lock()
	for currency := range state.Vault.Rates {
		delete(state.Vault.Rates[currency], cantorKey)
	}
	state.Vault.Mu.Unlock()

	state.UI.FilteredIDs = nil
	if state.UI.SelectedCantor == cantorKey {
		state.UI.SelectedCantor = ""
	}
}

func clearAllCantorsLocal(state *AppState) {
	state.CantorsMu.Lock()
	state.Cantors = make(map[string]*CantorInfo)
	state.CantorsMu.Unlock()

	state.Vault.Mu.Lock()
	for curr := range state.Vault.Rates {
		state.Vault.Rates[curr] = make(map[string]*CantorEntry)
	}
	state.Vault.Mu.Unlock()

	state.UI.SelectedCantor = ""
	state.UI.FilteredIDs = nil
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

	// OSM pins exist only in the GUI — drop locally without hitting the API.
	if cantor.ID == 0 || strings.EqualFold(cantor.Strategy, "OSM") || strings.HasPrefix(cantorKey, "osm_") {
		removeCantorLocal(state, cantorKey)
		SaveCache(state)
		if window != nil {
			window.Invalidate()
		}
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
		removeCantorLocal(state, cantorKey)
		SaveCache(state)
		if window != nil {
			window.Invalidate()
		}
	} else {
		log.Printf("Delete Cantor failed with status: %d", resp.StatusCode)
	}
}

// TriggerDeleteAllCantors removes heuristic cantors from the API, then reloads seeded ones.
func TriggerDeleteAllCantors(state *AppState, config AppConfig, window *app.Window) {
	log.Printf("Triggering Delete-All for cantors")

	req, err := http.NewRequest("DELETE", config.APICantorsURL, nil)
	if err != nil {
		log.Printf("Delete-All Request Error: %v", err)
		return
	}

	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Delete-All Error: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Delete-All failed with status: %d", resp.StatusCode)
		return
	}

	// Drop only local heuristic / OSM pins — keep seeded providers, then refresh from API.
	state.CantorsMu.Lock()
	for key, c := range state.Cantors {
		if c == nil {
			continue
		}
		if c.ID == 0 || strings.EqualFold(c.Strategy, "OSM") || strings.EqualFold(c.Strategy, "HEURISTIC") || strings.HasPrefix(key, "osm_") {
			delete(state.Cantors, key)
		}
	}
	state.CantorsMu.Unlock()
	state.UI.MapFocus.CityName = ""
	state.UI.FilteredIDs = nil
	reloadCantorsFromAPI(state, config)
	SaveCache(state)
	log.Printf("Delete-All: heuristic/OSM cleared; seeded cantors reloaded")
	if window != nil {
		window.Invalidate()
	}
}

func reloadCantorsFromAPI(state *AppState, config AppConfig) {
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(config.APICantorsURL)
	if err != nil {
		log.Printf("Reload cantors: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var list []ApiCantorResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		log.Printf("Reload cantors decode: %v", err)
		return
	}
	state.CantorsMu.Lock()
	for _, c := range list {
		idStr := fmt.Sprintf("%d", c.ID)
		if state.Cantors[idStr] == nil {
			state.Cantors[idStr] = &CantorInfo{
				ID:          c.ID,
				DisplayName: c.DisplayName,
				Latitude:    c.Latitude,
				Longitude:   c.Longitude,
				Address:     c.Address,
				Strategy:    c.Strategy,
			}
		}
	}
	state.CantorsMu.Unlock()
	state.UI.FilteredIDs = nil
}

func safeCoordKey(coord string) string {
	coord = strings.TrimSpace(coord)
	if len(coord) <= 5 {
		return coord
	}
	return coord[:5]
}

type osmCantorPOI struct {
	Lat    float64           `json:"lat"`
	Lon    float64           `json:"lon"`
	Center struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Tags map[string]string `json:"tags"`
}

// fetchCantorsOverpass races a couple of mirrors with a short budget (fallback only).
func fetchCantorsOverpass(client *http.Client, lat, lon float64) []osmCantorPOI {
	endpoints := []string{
		"https://overpass-api.de/api/interpreter",
		"https://lz4.overpass-api.de/api/interpreter",
	}
	// Amenity-only — name~[Kk]antor pulled lombards and random shops.
	query := fmt.Sprintf(`[out:json][timeout:5];
(
  nwr["amenity"="bureau_de_change"](around:12000,%f,%f);
  nwr["amenity"="currency_exchange"](around:12000,%f,%f);
);
out center tags;`, lat, lon, lat, lon)
	formData := url.Values{}
	formData.Set("data", query)
	body := formData.Encode()

	type overpassResp struct {
		Elements []osmCantorPOI `json:"elements"`
	}

	type result struct{ elements []osmCantorPOI }
	ch := make(chan result, len(endpoints))
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	for _, endpoint := range endpoints {
		ep := endpoint
		go func() {
			req, err := http.NewRequestWithContext(ctx, "POST", ep, strings.NewReader(body))
			if err != nil {
				ch <- result{}
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set(UserAgentHeader, UserAgentApp+" (discovery; github.com/Niutaq/Gix)")
			resp, err := client.Do(req)
			if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					_ = resp.Body.Close()
				}
				ch <- result{}
				return
			}
			var parsed overpassResp
			err = json.NewDecoder(resp.Body).Decode(&parsed)
			_ = resp.Body.Close()
			if err != nil || len(parsed.Elements) == 0 {
				ch <- result{}
				return
			}
			out := make([]osmCantorPOI, 0, len(parsed.Elements))
			for _, el := range parsed.Elements {
				elat, elon := el.Lat, el.Lon
				if elat == 0 && elon == 0 {
					elat, elon = el.Center.Lat, el.Center.Lon
				}
				elat, elon = NormalizeLatLon(elat, elon)
				if InPoland(elat, elon) {
					el.Lat, el.Lon = elat, elon
					out = append(out, el)
				}
			}
			ch <- result{elements: out}
		}()
	}

	for i := 0; i < len(endpoints); i++ {
		r := <-ch
		if len(r.elements) > 0 {
			cancel() // stop siblings
			return r.elements
		}
	}
	return nil
}

func osmDisplayName(c osmCantorPOI, city string) string {
	name := ""
	street := ""
	if c.Tags != nil {
		name = strings.TrimSpace(c.Tags["name"])
		street = strings.TrimSpace(c.Tags["addr:street"])
		if hn := strings.TrimSpace(c.Tags["addr:housenumber"]); hn != "" && street != "" {
			street = street + " " + hn
		}
	}
	// Unnamed / street-as-name POIs → "Kantor · …" (localized later).
	if name == "" || strings.EqualFold(name, "kantor") || looksLikeStreetLabel(name) {
		if street != "" {
			return "Kantor · " + street
		}
		if name != "" && looksLikeStreetLabel(name) {
			return "Kantor · " + name
		}
		return "Kantor · " + city
	}
	return name
}

func looksLikeStreetLabel(name string) bool {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		return true
	}
	if strings.Contains(n, "kantor") || strings.Contains(n, "exchange") || strings.Contains(n, "wymian") {
		return false
	}
	prefixes := []string{
		"ul.", "ul ", "al.", "al ", "aleja ", "plac ", "pl.", "os.", "osiedle ",
		"generała ", "kapitana ", "księdza ", "marszałka ", "świętego ", "św.",
		"ks.", "gen.", "płk.", "mjr.",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

func isJunkKantorText(s string) bool {
	n := strings.ToLower(s)
	for _, bad := range []string{
		"lombard", "pawn", "bitcoin", "crypto", "bankomat", " atm", "atm ",
		"hotel", "hostel", "restaurant", "parking", "apteka", "żabka", "biedronka",
		"car wash", "myjnia", "salon", "barber",
	} {
		if strings.Contains(n, bad) {
			return true
		}
	}
	return false
}

func isAcceptableKantorPOI(c osmCantorPOI) bool {
	name, amenity, shop := "", "", ""
	if c.Tags != nil {
		name = c.Tags["name"]
		amenity = c.Tags["amenity"]
		shop = c.Tags["shop"]
	}
	blob := name + " " + amenity + " " + shop
	if isJunkKantorText(blob) {
		return false
	}
	hasAmenity := amenity == "bureau_de_change" || amenity == "currency_exchange"
	low := strings.ToLower(name)
	hasWord := strings.Contains(low, "kantor") || strings.Contains(low, "exchange") ||
		strings.Contains(low, "wymian") || strings.Contains(low, "walut")
	// Named "Kantor …" without amenity is OK; bare shops / random name~ hits are not.
	if hasAmenity {
		return true
	}
	return hasWord
}

// isStrongKantorPOI — real business name, not a bare street / house number.
func isStrongKantorPOI(c osmCantorPOI) bool {
	if !isAcceptableKantorPOI(c) {
		return false
	}
	name := ""
	if c.Tags != nil {
		name = strings.TrimSpace(c.Tags["name"])
	}
	if name == "" || strings.EqualFold(name, "kantor") || looksLikeStreetLabel(name) {
		return false
	}
	low := strings.ToLower(name)
	if strings.Contains(low, "kantor") || strings.Contains(low, "exchange") ||
		strings.Contains(low, "wymian") || strings.Contains(low, "walut") {
		return true
	}
	// Brand-like name on a bureau_de_change (e.g. "MERITUM", "P&R Exchange").
	amenity := ""
	if c.Tags != nil {
		amenity = c.Tags["amenity"]
	}
	if (amenity == "bureau_de_change" || amenity == "currency_exchange") && len([]rune(name)) >= 4 {
		return true
	}
	return false
}

const cityDiscoverRadiusKm = 12.0

func mergeOSMCantors(cityLat, cityLon float64, parts ...[]osmCantorPOI) []osmCantorPOI {
	seen := make(map[string]bool)
	type scored struct {
		c     osmCantorPOI
		dist  float64
		strong bool
	}
	var ranked []scored
	for _, list := range parts {
		for _, c := range list {
			lat, lon := c.Lat, c.Lon
			if lat == 0 && lon == 0 {
				lat, lon = c.Center.Lat, c.Center.Lon
			}
			lat, lon = NormalizeLatLon(lat, lon)
			if !InPoland(lat, lon) {
				continue
			}
			c.Lat, c.Lon = lat, lon
			if !isAcceptableKantorPOI(c) {
				continue
			}
			dist := CalculateDistance(cityLat, cityLon, lat, lon)
			if cityLat != 0 || cityLon != 0 {
				if dist > cityDiscoverRadiusKm {
					continue
				}
			}
			key := fmt.Sprintf("%.4f,%.4f", lat, lon)
			if seen[key] {
				continue
			}
			seen[key] = true
			ranked = append(ranked, scored{c: c, dist: dist, strong: isStrongKantorPOI(c)})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].strong != ranked[j].strong {
			return ranked[i].strong // strong first
		}
		return ranked[i].dist < ranked[j].dist
	})
	strongN := 0
	for _, s := range ranked {
		if s.strong {
			strongN++
		}
	}
	// Prefer named businesses; only keep street-only fillers if almost nothing named.
	const maxKeep = 8
	out := make([]osmCantorPOI, 0, maxKeep)
	for _, s := range ranked {
		if !s.strong && strongN >= 3 {
			continue
		}
		out = append(out, s.c)
		if len(out) >= maxKeep {
			break
		}
	}
	return out
}

func fetchCantorsNominatim(client *http.Client, city string, lat, lon float64) []osmCantorPOI {
	seen := make(map[string]bool)
	var out []osmCantorPOI

	appendResults := func(results []struct {
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
		Class       string `json:"class"`
		Type        string `json:"type"`
		Name        string `json:"name"`
		Address     struct {
			Road        string `json:"road"`
			Pedestrian  string `json:"pedestrian"`
			HouseNumber string `json:"house_number"`
			City        string `json:"city"`
			Town        string `json:"town"`
		} `json:"address"`
	}, requireKantorWord bool) {
		for _, r := range results {
			dn := strings.ToLower(r.DisplayName + " " + r.Name)
			if isJunkKantorText(dn) {
				continue
			}
			if requireKantorWord {
				if r.Type != "bureau_de_change" &&
					!strings.Contains(dn, "kantor") &&
					!strings.Contains(dn, "exchange") &&
					!strings.Contains(dn, "wymian") {
					continue
				}
			} else if r.Type != "bureau_de_change" && r.Type != "currency_exchange" {
				// Structured amenity search should only keep real exchange POIs.
				continue
			}
			cLat, _ := strconv.ParseFloat(r.Lat, 64)
			cLon, _ := strconv.ParseFloat(r.Lon, 64)
			cLat, cLon = NormalizeLatLon(cLat, cLon)
			if !InPoland(cLat, cLon) {
				continue
			}
			if lat != 0 || lon != 0 {
				if CalculateDistance(lat, lon, cLat, cLon) > cityDiscoverRadiusKm {
					continue
				}
			}
			key := fmt.Sprintf("%.4f,%.4f", cLat, cLon)
			if seen[key] {
				continue
			}
			seen[key] = true
			name := strings.TrimSpace(r.Name)
			if name == "" {
				name = strings.TrimSpace(strings.Split(r.DisplayName, ",")[0])
			}
			street := strings.TrimSpace(r.Address.Road)
			if street == "" {
				street = strings.TrimSpace(r.Address.Pedestrian)
			}
			if street == "" {
				parts := strings.Split(r.DisplayName, ",")
				if len(parts) > 1 {
					street = strings.TrimSpace(parts[1])
				}
			}
			if hn := strings.TrimSpace(r.Address.HouseNumber); hn != "" && street != "" && !strings.Contains(street, hn) {
				street = street + " " + hn
			}
			tags := map[string]string{"name": name}
			if street != "" {
				tags["addr:street"] = street
			}
			if r.Type == "bureau_de_change" || r.Type == "currency_exchange" {
				tags["amenity"] = r.Type
			}
			cand := osmCantorPOI{Lat: cLat, Lon: cLon, Tags: tags}
			if !isAcceptableKantorPOI(cand) {
				continue
			}
			out = append(out, cand)
		}
	}

	doGET := func(rawURL string) {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", UserAgentApp+" (discovery; github.com/Niutaq/Gix)")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		var results []struct {
			DisplayName string `json:"display_name"`
			Lat         string `json:"lat"`
			Lon         string `json:"lon"`
			Class       string `json:"class"`
			Type        string `json:"type"`
			Name        string `json:"name"`
			Address     struct {
				Road        string `json:"road"`
				Pedestrian  string `json:"pedestrian"`
				HouseNumber string `json:"house_number"`
				City        string `json:"city"`
				Town        string `json:"town"`
			} `json:"address"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			return
		}
		// amenity=… endpoints are already typed; free-text needs word filter.
		requireWord := !strings.Contains(rawURL, "amenity=bureau_de_change")
		appendResults(results, requireWord)
	}

	// 1) Fast structured amenity search in a tight city viewbox (~1 request).
	if lat != 0 || lon != 0 {
		const dLat, dLon = 0.11, 0.16 // ~12 km
		viewbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", lon-dLon, lat-dLat, lon+dLon, lat+dLat)
		u := "https://nominatim.openstreetmap.org/search?amenity=bureau_de_change&format=json&addressdetails=1&limit=15&countrycodes=pl&viewbox=" +
			url.QueryEscape(viewbox) + "&bounded=1"
		doGET(u)
	}

	// 2) At most one free-text query — only if viewbox was thin (Nominatim 1 req/s).
	strong := 0
	for _, c := range out {
		if isStrongKantorPOI(c) {
			strong++
		}
	}
	if strong < 3 && strings.TrimSpace(city) != "" {
		time.Sleep(1100 * time.Millisecond)
		u := "https://nominatim.openstreetmap.org/search?q=" + url.QueryEscape("kantor "+city) +
			"&format=json&addressdetails=1&limit=10&countrycodes=pl"
		doGET(u)
	}
	return out
}

var (
	cityDiscoverMu   sync.Mutex
	cityDiscoverGen  uint64
	activeCitySearch string
)

// DiscoverCityCantors finds physical cantors via Nominatim/Overpass and their URLs via DuckDuckGo.
func DiscoverCityCantors(city string, state *AppState, config AppConfig, window *app.Window) {
	city = strings.TrimSpace(city)
	if city == "" {
		return
	}

	cityDiscoverMu.Lock()
	cityDiscoverGen++
	gen := cityDiscoverGen
	activeCitySearch = city
	cityDiscoverMu.Unlock()

	isStale := func() bool {
		cityDiscoverMu.Lock()
		defer cityDiscoverMu.Unlock()
		return gen != cityDiscoverGen
	}

	state.CantorsMu.Lock()
	if state.UI.ScannedChunks == nil {
		state.UI.ScannedChunks = make(map[string]bool)
	}
	state.UI.ScannedChunks[strings.ToLower(city)] = true
	// Drop previous city OSM pins so the list isn't stuck on an empty city filter.
	for key, c := range state.Cantors {
		if c != nil && (strings.HasPrefix(key, "osm_") || strings.EqualFold(c.Strategy, "OSM")) {
			delete(state.Cantors, key)
		}
	}
	state.CantorsMu.Unlock()

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	// City search owns the focus — keep last GPS coords for distance, drop radius lock.
	state.UI.UserLocation.Active = false
	state.UI.MaxDistance = 0
	state.UI.DistanceSlider.Value = 0
	state.UI.MapFocus.CityName = ""
	state.UI.FilteredIDs = nil

	setSearchFeedback := func(msg string) {
		if isStale() {
			return
		}
		state.UI.SearchEditor.SetText(msg)
		state.UI.SearchText = msg
		if window != nil {
			window.Invalidate()
		}
	}
	clearSearchIfQuery := func() {
		if isStale() {
			return
		}
		if strings.EqualFold(strings.TrimSpace(state.UI.SearchEditor.Text()), city) {
			state.UI.SearchEditor.SetText("")
			state.UI.SearchText = ""
			if window != nil {
				window.Invalidate()
			}
		}
	}

	if window != nil {
		window.Invalidate()
	}

	finishLoading := func() {
		state.IsLoading.Store(false)
		if window != nil {
			window.Invalidate()
		}
	}

	log.Printf("Starting city discovery for: %s", city)

	// Nominatim-first path stays under ~10s; Overpass fallback is capped at 6s.
	client := &http.Client{Timeout: 10 * time.Second}

	// Step 1: Geocode city — Poland only (PLN kantor product, not global FX).
	geocodeURL := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1&countrycodes=pl",
		url.QueryEscape(city),
	)
	reqCity, _ := http.NewRequest("GET", geocodeURL, nil)
	reqCity.Header.Set("User-Agent", UserAgentApp+" (city search; github.com/Niutaq/Gix)")
	respCity, err := client.Do(reqCity)
	if err != nil {
		log.Printf("Geocode Error: %v", err)
		setSearchFeedback(GetTranslation(state.UI.Language, "err_api_connection"))
		finishLoading()
		return
	}

	var geoResults []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	decodeErr := json.NewDecoder(respCity.Body).Decode(&geoResults)
	_ = respCity.Body.Close()
	if decodeErr != nil || len(geoResults) == 0 {
		log.Printf("Could not find coordinates for city in Poland: %s", city)
		setSearchFeedback(GetTranslation(state.UI.Language, "err_city_not_poland"))
		finishLoading()
		return
	}

	lat, _ := strconv.ParseFloat(geoResults[0].Lat, 64)
	lon, _ := strconv.ParseFloat(geoResults[0].Lon, 64)
	lat, lon = NormalizeLatLon(lat, lon)
	if !InPoland(lat, lon) {
		log.Printf("Rejected city outside Poland: %s (%.4f, %.4f)", city, lat, lon)
		setSearchFeedback(GetTranslation(state.UI.Language, "err_city_not_poland"))
		finishLoading()
		return
	}

	state.UI.MapFocus.Latitude = lat
	state.UI.MapFocus.Longitude = lon
	state.UI.MapState.CenterLat = lat
	state.UI.MapState.CenterLon = lon
	// CityName + radius filter only after pins exist — otherwise the list flashes empty.
	state.UI.FilteredIDs = nil
	if window != nil {
		window.Invalidate()
	}

	if isStale() {
		return
	}

	// Nominatim fair-use: ≥1s between requests (geocode → amenity).
	time.Sleep(1100 * time.Millisecond)

	// Step 2: Nominatim first (fast). Overpass only if the city still looks empty.
	nominatim := fetchCantorsNominatim(client, city, lat, lon)
	log.Printf("Discovery: Nominatim %d POIs near %s", len(nominatim), city)
	cantors := mergeOSMCantors(lat, lon, nominatim)
	if len(cantors) < 3 {
		log.Printf("Discovery: thin Nominatim for %s — short Overpass fallback", city)
		overpass := fetchCantorsOverpass(client, lat, lon)
		log.Printf("Discovery: Overpass %d POIs near %s", len(overpass), city)
		cantors = mergeOSMCantors(lat, lon, cantors, overpass)
	}
	if isStale() {
		return
	}

	if len(cantors) == 0 {
		log.Printf("Discovery: no OSM cantors near %s", city)
		state.UI.MapFocus.CityName = city
		setSearchFeedback(GetTranslation(state.UI.Language, "err_no_cantors_found"))
		finishLoading()
		return
	}

	// Show pins on the map/list immediately (rates come later if a website is found).
	for i := range cantors {
		if cantors[i].Lat == 0 && cantors[i].Center.Lat != 0 {
			cantors[i].Lat = cantors[i].Center.Lat
			cantors[i].Lon = cantors[i].Center.Lon
		}
		name := osmDisplayName(cantors[i], city)
		if cantors[i].Tags == nil {
			cantors[i].Tags = map[string]string{}
		}
		cantors[i].Tags["name"] = name
		osmKey := fmt.Sprintf("osm_%.4f_%.4f", cantors[i].Lat, cantors[i].Lon)
		state.CantorsMu.Lock()
		if state.Cantors[osmKey] == nil {
			state.Cantors[osmKey] = &CantorInfo{
				DisplayName: name,
				Latitude:    cantors[i].Lat,
				Longitude:   cantors[i].Lon,
				Address:     city,
				Strategy:    "OSM",
			}
		} else {
			state.Cantors[osmKey].DisplayName = name
			state.Cantors[osmKey].Address = city
			state.Cantors[osmKey].Latitude = cantors[i].Lat
			state.Cantors[osmKey].Longitude = cantors[i].Lon
		}
		state.CantorsMu.Unlock()
	}
	state.UI.MapFocus.CityName = city
	state.UI.SortMode = "DIST"
	// nil → filterCantorList rebuilds each frame until non-empty; also picks up later Discover APIs.
	state.UI.FilteredIDs = nil
	clearSearchIfQuery()
	log.Printf("Discovery: pinned %d OSM cantors for %s on the list", len(cantors), city)
	finishLoading()
	if window != nil {
		window.Invalidate()
	}

	haversine := func(lat1, lon1, lat2, lon2 float64) float64 {
		const R = 6371.0
		dLat := (lat2 - lat1) * math.Pi / 180.0
		dLon := (lon2 - lon1) * math.Pi / 180.0
		lat1Rad := lat1 * math.Pi / 180.0
		lat2Rad := lat2 * math.Pi / 180.0
		a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
		c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
		return R * c
	}

	type CantorWithDistance struct {
		Element  osmCantorPOI
		Distance float64
	}
	var cantorsWithDist []CantorWithDistance
	for _, c := range cantors {
		dist := haversine(lat, lon, c.Lat, c.Lon)
		cantorsWithDist = append(cantorsWithDist, CantorWithDistance{Element: c, Distance: dist})
	}

	sort.Slice(cantorsWithDist, func(i, j int) bool {
		return cantorsWithDist[i].Distance < cantorsWithDist[j].Distance
	})

	// Take top 5 — website lookup runs in background (spinner already cleared).
	limit := 5
	if len(cantorsWithDist) < limit {
		limit = len(cantorsWithDist)
	}
	topCantors := cantorsWithDist[:limit]
	log.Printf("Discovery: found %d OSM cantors near %s — resolving websites…", limit, city)

	seenURLs := make(map[string]bool)
	var urlMu sync.Mutex
	var wg sync.WaitGroup
	for i, cw := range topCantors {
		if isStale() {
			break
		}
		c := cw.Element
		cLat := c.Lat
		cLon := c.Lon
		cName := ""
		if c.Tags != nil {
			cName = c.Tags["name"]
		}
		if cName == "" || strings.EqualFold(cName, "kantor") {
			cName = "Kantor " + city
		}

		var addrParts []string
		if c.Tags != nil {
			if c.Tags["addr:street"] != "" {
				addrParts = append(addrParts, c.Tags["addr:street"])
			}
			if c.Tags["addr:housenumber"] != "" {
				addrParts = append(addrParts, c.Tags["addr:housenumber"])
			}
			if c.Tags["addr:city"] != "" {
				addrParts = append(addrParts, c.Tags["addr:city"])
			}
		}
		cAddr := strings.Join(addrParts, ", ")
		if cAddr == "" {
			cAddr = cName
		}

		website := ""
		if c.Tags != nil {
			website = c.Tags["website"]
			if website == "" {
				website = c.Tags["contact:website"]
			}
		}

		if website != "" {
			if seenURLs[website] {
				continue
			}
			seenURLs[website] = true
			wg.Add(1)
			go func(urlStr, name string, lt, ln float64, addr string) {
				defer wg.Done()
				if isStale() {
					return
				}
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
						urlMu.Lock()
						already := seenURLs[decoded]
						if !already {
							seenURLs[decoded] = true
						}
						urlMu.Unlock()
						if already {
							return true
						}
						if isStale() {
							return false
						}
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

// LLMDiscoverCityCantors is kept for compatibility. Discovery is heuristic, not LLM-backed.
func LLMDiscoverCityCantors(city string, state *AppState, config AppConfig, window *app.Window) {
	DiscoverCityCantors(city, state, config, window)
}
