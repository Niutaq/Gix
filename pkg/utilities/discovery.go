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

const (
	osmTagStreet      = "addr:street"
	osmTagHouseNumber = "addr:housenumber"
	kantorPrefix      = "Kantor "
	kantorDotPrefix   = "Kantor · "
)

type osmCantorPOI struct {
	Lat    float64           `json:"lat"`
	Lon    float64           `json:"lon"`
	Center struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Tags map[string]string `json:"tags"`
}

type overpassResp struct {
	Elements []osmCantorPOI `json:"elements"`
}

// buildOverpassBody builds the form-encoded Overpass query for exchange POIs around a point.
func buildOverpassBody(lat, lon float64) string {
	// Amenity-only — name~[Kk]antor pulled lombards and random shops.
	query := fmt.Sprintf(`[out:json][timeout:5];
(
  nwr["amenity"="bureau_de_change"](around:12000,%f,%f);
  nwr["amenity"="currency_exchange"](around:12000,%f,%f);
);
out center tags;`, lat, lon, lat, lon)
	formData := url.Values{}
	formData.Set("data", query)
	return formData.Encode()
}

// filterOverpassElements normalizes coordinates and keeps only POIs located in Poland.
func filterOverpassElements(elements []osmCantorPOI) []osmCantorPOI {
	out := make([]osmCantorPOI, 0, len(elements))
	for _, el := range elements {
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
	return out
}

// queryOverpassEndpoint runs the query against a single Overpass mirror.
func queryOverpassEndpoint(ctx context.Context, client *http.Client, endpoint, body string) []osmCantorPOI {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(UserAgentHeader, UserAgentApp+" (discovery; github.com/Niutaq/Gix)")
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil
	}
	var parsed overpassResp
	err = json.NewDecoder(resp.Body).Decode(&parsed)
	_ = resp.Body.Close()
	if err != nil || len(parsed.Elements) == 0 {
		return nil
	}
	return filterOverpassElements(parsed.Elements)
}

// fetchCantorsOverpass races a couple of mirrors with a short budget (fallback only).
func fetchCantorsOverpass(client *http.Client, lat, lon float64) []osmCantorPOI {
	endpoints := []string{
		"https://overpass-api.de/api/interpreter",
		"https://lz4.overpass-api.de/api/interpreter",
	}
	body := buildOverpassBody(lat, lon)

	ch := make(chan []osmCantorPOI, len(endpoints))
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	for _, endpoint := range endpoints {
		go func(ep string) {
			ch <- queryOverpassEndpoint(ctx, client, ep, body)
		}(endpoint)
	}

	for range endpoints {
		if elements := <-ch; len(elements) > 0 {
			cancel() // stop siblings
			return elements
		}
	}
	return nil
}

func osmDisplayName(c osmCantorPOI, city string) string {
	name := ""
	street := ""
	if c.Tags != nil {
		name = strings.TrimSpace(c.Tags["name"])
		street = strings.TrimSpace(c.Tags[osmTagStreet])
		if hn := strings.TrimSpace(c.Tags[osmTagHouseNumber]); hn != "" && street != "" {
			street = street + " " + hn
		}
	}
	// Unnamed / street-as-name POIs → "Kantor · …" (localized later).
	if name == "" || strings.EqualFold(name, "kantor") || looksLikeStreetLabel(name) {
		if street != "" {
			return kantorDotPrefix + street
		}
		if name != "" && looksLikeStreetLabel(name) {
			return kantorDotPrefix + name
		}
		return kantorDotPrefix + city
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

type scoredPOI struct {
	c      osmCantorPOI
	dist   float64
	strong bool
}

// withinCityRadius reports whether dist is inside the discovery radius (unset city centre disables the check).
func withinCityRadius(cityLat, cityLon, dist float64) bool {
	if cityLat == 0 && cityLon == 0 {
		return true
	}
	return dist <= cityDiscoverRadiusKm
}

// normalizePOICoords returns the POI position (falling back to its center) with lat/lon normalized.
func normalizePOICoords(c osmCantorPOI) (float64, float64) {
	lat, lon := c.Lat, c.Lon
	if lat == 0 && lon == 0 {
		lat, lon = c.Center.Lat, c.Center.Lon
	}
	return NormalizeLatLon(lat, lon)
}

// scoreOSMCandidate validates a POI (in Poland, acceptable, inside the radius) and scores it.
func scoreOSMCandidate(cityLat, cityLon float64, c osmCantorPOI) (scoredPOI, bool) {
	lat, lon := normalizePOICoords(c)
	if !InPoland(lat, lon) {
		return scoredPOI{}, false
	}
	c.Lat, c.Lon = lat, lon
	if !isAcceptableKantorPOI(c) {
		return scoredPOI{}, false
	}
	dist := CalculateDistance(cityLat, cityLon, lat, lon)
	if !withinCityRadius(cityLat, cityLon, dist) {
		return scoredPOI{}, false
	}
	return scoredPOI{c: c, dist: dist, strong: isStrongKantorPOI(c)}, true
}

// rankOSMCantors dedupes and scores candidate POIs from all sources.
func rankOSMCantors(cityLat, cityLon float64, parts [][]osmCantorPOI) []scoredPOI {
	seen := make(map[string]bool)
	var ranked []scoredPOI
	for _, list := range parts {
		for _, c := range list {
			cand, ok := scoreOSMCandidate(cityLat, cityLon, c)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%.4f,%.4f", cand.c.Lat, cand.c.Lon)
			if seen[key] {
				continue
			}
			seen[key] = true
			ranked = append(ranked, cand)
		}
	}
	return ranked
}

// sortScoredPOIs puts strong (named) POIs first, then orders by distance.
func sortScoredPOIs(ranked []scoredPOI) {
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].strong != ranked[j].strong {
			return ranked[i].strong // strong first
		}
		return ranked[i].dist < ranked[j].dist
	})
}

// pickTopRanked keeps at most maxKeep POIs, dropping street-only fillers when enough named ones exist.
func pickTopRanked(ranked []scoredPOI) []osmCantorPOI {
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

func mergeOSMCantors(cityLat, cityLon float64, parts ...[]osmCantorPOI) []osmCantorPOI {
	ranked := rankOSMCantors(cityLat, cityLon, parts)
	sortScoredPOIs(ranked)
	return pickTopRanked(ranked)
}

type nominatimPlace struct {
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

// isExchangeAmenityType reports whether an OSM type denotes a real currency exchange POI.
func isExchangeAmenityType(t string) bool {
	return t == "bureau_de_change" || t == "currency_exchange"
}

// nominatimPlaceMatches filters out junk and (depending on the query kind) non-exchange places.
func nominatimPlaceMatches(r nominatimPlace, requireKantorWord bool) bool {
	dn := strings.ToLower(r.DisplayName + " " + r.Name)
	if isJunkKantorText(dn) {
		return false
	}
	if requireKantorWord {
		return r.Type == "bureau_de_change" ||
			strings.Contains(dn, "kantor") ||
			strings.Contains(dn, "exchange") ||
			strings.Contains(dn, "wymian")
	}
	// Structured amenity search should only keep real exchange POIs.
	return isExchangeAmenityType(r.Type)
}

// nominatimStreet derives a "street number" label from a Nominatim place.
func nominatimStreet(r nominatimPlace) string {
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
	return street
}

// buildNominatimCandidate converts a Nominatim place into an OSM-style POI.
func buildNominatimCandidate(r nominatimPlace, lat, lon float64) osmCantorPOI {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = strings.TrimSpace(strings.Split(r.DisplayName, ",")[0])
	}
	tags := map[string]string{"name": name}
	if street := nominatimStreet(r); street != "" {
		tags[osmTagStreet] = street
	}
	if isExchangeAmenityType(r.Type) {
		tags["amenity"] = r.Type
	}
	return osmCantorPOI{Lat: lat, Lon: lon, Tags: tags}
}

// nominatimCollector accumulates deduplicated exchange POIs from Nominatim responses.
type nominatimCollector struct {
	client   *http.Client
	lat, lon float64
	seen     map[string]bool
	out      []osmCantorPOI
}

// placeCoords parses a place position and checks it lies in Poland and inside the city radius.
func (n *nominatimCollector) placeCoords(r nominatimPlace) (float64, float64, bool) {
	cLat, _ := strconv.ParseFloat(r.Lat, 64)
	cLon, _ := strconv.ParseFloat(r.Lon, 64)
	cLat, cLon = NormalizeLatLon(cLat, cLon)
	if !InPoland(cLat, cLon) {
		return 0, 0, false
	}
	if !withinCityRadius(n.lat, n.lon, CalculateDistance(n.lat, n.lon, cLat, cLon)) {
		return 0, 0, false
	}
	return cLat, cLon, true
}

func (n *nominatimCollector) appendResults(results []nominatimPlace, requireKantorWord bool) {
	for _, r := range results {
		if !nominatimPlaceMatches(r, requireKantorWord) {
			continue
		}
		cLat, cLon, ok := n.placeCoords(r)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%.4f,%.4f", cLat, cLon)
		if n.seen[key] {
			continue
		}
		n.seen[key] = true
		cand := buildNominatimCandidate(r, cLat, cLon)
		if !isAcceptableKantorPOI(cand) {
			continue
		}
		n.out = append(n.out, cand)
	}
}

// fetchPlaces performs a Nominatim GET and decodes the result list.
func (n *nominatimCollector) fetchPlaces(rawURL string) ([]nominatimPlace, bool) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", UserAgentApp+" (discovery; github.com/Niutaq/Gix)")
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	var results []nominatimPlace
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, false
	}
	return results, true
}

func (n *nominatimCollector) doGET(rawURL string) {
	results, ok := n.fetchPlaces(rawURL)
	if !ok {
		return
	}
	// amenity=… endpoints are already typed; free-text needs word filter.
	requireWord := !strings.Contains(rawURL, "amenity=bureau_de_change")
	n.appendResults(results, requireWord)
}

// strongCount returns how many collected POIs have a real business name.
func (n *nominatimCollector) strongCount() int {
	strong := 0
	for _, c := range n.out {
		if isStrongKantorPOI(c) {
			strong++
		}
	}
	return strong
}

// nominatimViewboxURL builds the structured amenity search URL for a tight city viewbox (~12 km).
func nominatimViewboxURL(lat, lon float64) string {
	const dLat, dLon = 0.11, 0.16 // ~12 km
	viewbox := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", lon-dLon, lat-dLat, lon+dLon, lat+dLat)
	return "https://nominatim.openstreetmap.org/search?amenity=bureau_de_change&format=json&addressdetails=1&limit=15&countrycodes=pl&viewbox=" +
		url.QueryEscape(viewbox) + "&bounded=1"
}

func fetchCantorsNominatim(client *http.Client, city string, lat, lon float64) []osmCantorPOI {
	n := &nominatimCollector{client: client, lat: lat, lon: lon, seen: make(map[string]bool)}

	// 1) Fast structured amenity search in a tight city viewbox (~1 request).
	if lat != 0 || lon != 0 {
		n.doGET(nominatimViewboxURL(lat, lon))
	}

	// 2) At most one free-text query — only if viewbox was thin (Nominatim 1 req/s).
	if n.strongCount() < 3 && strings.TrimSpace(city) != "" {
		time.Sleep(1100 * time.Millisecond)
		n.doGET("https://nominatim.openstreetmap.org/search?q=" + url.QueryEscape("kantor "+city) +
			"&format=json&addressdetails=1&limit=10&countrycodes=pl")
	}
	return n.out
}

var (
	cityDiscoverMu   sync.Mutex
	cityDiscoverGen  uint64
	activeCitySearch string
)

// citySearch carries the state of a single DiscoverCityCantors run.
type citySearch struct {
	city   string
	state  *AppState
	config AppConfig
	window *app.Window
	client *http.Client
	gen    uint64
}

// discoveryTarget is a resolved OSM cantor that still needs a website / rates lookup.
type discoveryTarget struct {
	name    string
	addr    string
	website string
	lat     float64
	lon     float64
}

type cantorWithDistance struct {
	Element  osmCantorPOI
	Distance float64
}

// seenURLSet is a concurrency-safe set used to avoid scraping the same URL twice.
type seenURLSet struct {
	mu   sync.Mutex
	seen map[string]bool
}

// claim marks the URL as seen and reports whether this call was the first to do so.
func (s *seenURLSet) claim(u string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[u] {
		return false
	}
	s.seen[u] = true
	return true
}

// excludedDiscoveryDomains lists aggregators, directories, maps, social media, and crypto exchanges to skip.
var excludedDiscoveryDomains = []string{
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

// isExcludedDiscoveryURL reports whether the URL points at a domain we never scrape rates from.
func isExcludedDiscoveryURL(u string) bool {
	lower := strings.ToLower(u)
	for _, domain := range excludedDiscoveryDomains {
		if strings.Contains(lower, domain) {
			return true
		}
	}
	return false
}

// haversineKm returns the great-circle distance in kilometres.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// newCitySearch registers a new search generation (invalidating any running one).
func newCitySearch(city string, state *AppState, config AppConfig, window *app.Window) *citySearch {
	cityDiscoverMu.Lock()
	cityDiscoverGen++
	gen := cityDiscoverGen
	activeCitySearch = city
	cityDiscoverMu.Unlock()

	// Nominatim-first path stays under ~10s; Overpass fallback is capped at 6s.
	return &citySearch{
		city:   city,
		state:  state,
		config: config,
		window: window,
		client: &http.Client{Timeout: 10 * time.Second},
		gen:    gen,
	}
}

func (s *citySearch) isStale() bool {
	cityDiscoverMu.Lock()
	defer cityDiscoverMu.Unlock()
	return s.gen != cityDiscoverGen
}

func (s *citySearch) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (s *citySearch) setSearchFeedback(msg string) {
	if s.isStale() {
		return
	}
	s.state.UI.SearchEditor.SetText(msg)
	s.state.UI.SearchText = msg
	s.invalidate()
}

func (s *citySearch) clearSearchIfQuery() {
	if s.isStale() {
		return
	}
	if strings.EqualFold(strings.TrimSpace(s.state.UI.SearchEditor.Text()), s.city) {
		s.state.UI.SearchEditor.SetText("")
		s.state.UI.SearchText = ""
		s.invalidate()
	}
}

func (s *citySearch) finishLoading() {
	s.state.IsLoading.Store(false)
	s.invalidate()
}

// fail shows a localized error in the search box and stops the loading spinner.
func (s *citySearch) fail(translationKey string) {
	s.setSearchFeedback(GetTranslation(s.state.UI.Language, translationKey))
	s.finishLoading()
}

// resetForSearch clears previous city pins and UI filters before a new city search.
func (s *citySearch) resetForSearch() {
	state := s.state
	state.CantorsMu.Lock()
	if state.UI.ScannedChunks == nil {
		state.UI.ScannedChunks = make(map[string]bool)
	}
	state.UI.ScannedChunks[strings.ToLower(s.city)] = true
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

	s.invalidate()
}

// geocodeCity resolves the city to Poland-only coordinates; on failure it reports the error to the UI.
func (s *citySearch) geocodeCity() (float64, float64, bool) {
	// Step 1: Geocode city — Poland only (PLN kantor product, not global FX).
	geocodeURL := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1&countrycodes=pl",
		url.QueryEscape(s.city),
	)
	reqCity, _ := http.NewRequest("GET", geocodeURL, nil)
	reqCity.Header.Set("User-Agent", UserAgentApp+" (city search; github.com/Niutaq/Gix)")
	respCity, err := s.client.Do(reqCity)
	if err != nil {
		log.Printf("Geocode Error: %v", err)
		s.fail("err_api_connection")
		return 0, 0, false
	}

	var geoResults []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	decodeErr := json.NewDecoder(respCity.Body).Decode(&geoResults)
	_ = respCity.Body.Close()
	if decodeErr != nil || len(geoResults) == 0 {
		log.Printf("Could not find coordinates for city in Poland: %s", s.city)
		s.fail("err_city_not_poland")
		return 0, 0, false
	}

	lat, _ := strconv.ParseFloat(geoResults[0].Lat, 64)
	lon, _ := strconv.ParseFloat(geoResults[0].Lon, 64)
	lat, lon = NormalizeLatLon(lat, lon)
	if !InPoland(lat, lon) {
		log.Printf("Rejected city outside Poland: %s (%.4f, %.4f)", s.city, lat, lon)
		s.fail("err_city_not_poland")
		return 0, 0, false
	}
	return lat, lon, true
}

// focusMap centers the map on the geocoded city.
func (s *citySearch) focusMap(lat, lon float64) {
	s.state.UI.MapFocus.Latitude = lat
	s.state.UI.MapFocus.Longitude = lon
	s.state.UI.MapState.CenterLat = lat
	s.state.UI.MapState.CenterLon = lon
	// CityName + radius filter only after pins exist — otherwise the list flashes empty.
	s.state.UI.FilteredIDs = nil
	s.invalidate()
}

// collectCantors finds exchange POIs: Nominatim first (fast), Overpass only if the city still looks empty.
func (s *citySearch) collectCantors(lat, lon float64) []osmCantorPOI {
	nominatim := fetchCantorsNominatim(s.client, s.city, lat, lon)
	log.Printf("Discovery: Nominatim %d POIs near %s", len(nominatim), s.city)
	cantors := mergeOSMCantors(lat, lon, nominatim)
	if len(cantors) < 3 {
		log.Printf("Discovery: thin Nominatim for %s — short Overpass fallback", s.city)
		overpass := fetchCantorsOverpass(s.client, lat, lon)
		log.Printf("Discovery: Overpass %d POIs near %s", len(overpass), s.city)
		cantors = mergeOSMCantors(lat, lon, cantors, overpass)
	}
	return cantors
}

// pinCantor adds (or refreshes) the map/list entry for a single OSM POI.
func (s *citySearch) pinCantor(c *osmCantorPOI) {
	if c.Lat == 0 && c.Center.Lat != 0 {
		c.Lat = c.Center.Lat
		c.Lon = c.Center.Lon
	}
	name := osmDisplayName(*c, s.city)
	if c.Tags == nil {
		c.Tags = map[string]string{}
	}
	c.Tags["name"] = name
	osmKey := fmt.Sprintf("osm_%.4f_%.4f", c.Lat, c.Lon)

	s.state.CantorsMu.Lock()
	defer s.state.CantorsMu.Unlock()
	info := s.state.Cantors[osmKey]
	if info == nil {
		s.state.Cantors[osmKey] = &CantorInfo{
			DisplayName: name,
			Latitude:    c.Lat,
			Longitude:   c.Lon,
			Address:     s.city,
			Strategy:    "OSM",
		}
		return
	}
	info.DisplayName = name
	info.Address = s.city
	info.Latitude = c.Lat
	info.Longitude = c.Lon
}

// pinCantors shows pins on the map/list immediately (rates come later if a website is found).
func (s *citySearch) pinCantors(cantors []osmCantorPOI) {
	for i := range cantors {
		s.pinCantor(&cantors[i])
	}
	s.state.UI.MapFocus.CityName = s.city
	s.state.UI.SortMode = "DIST"
	// nil → filterCantorList rebuilds each frame until non-empty; also picks up later Discover APIs.
	s.state.UI.FilteredIDs = nil
	s.clearSearchIfQuery()
	log.Printf("Discovery: pinned %d OSM cantors for %s on the list", len(cantors), s.city)
	s.finishLoading()
}

// nearestCantors returns up to limit POIs sorted by distance from the city centre.
func nearestCantors(cantors []osmCantorPOI, lat, lon float64, limit int) []cantorWithDistance {
	var withDist []cantorWithDistance
	for _, c := range cantors {
		withDist = append(withDist, cantorWithDistance{Element: c, Distance: haversineKm(lat, lon, c.Lat, c.Lon)})
	}
	sort.Slice(withDist, func(i, j int) bool {
		return withDist[i].Distance < withDist[j].Distance
	})
	if len(withDist) < limit {
		limit = len(withDist)
	}
	return withDist[:limit]
}

// newDiscoveryTarget extracts name, address and website of an OSM POI.
func newDiscoveryTarget(c osmCantorPOI, city string) discoveryTarget {
	name := c.Tags["name"]
	if name == "" || strings.EqualFold(name, "kantor") {
		name = kantorPrefix + city
	}

	var addrParts []string
	for _, tag := range []string{osmTagStreet, osmTagHouseNumber, "addr:city"} {
		if v := c.Tags[tag]; v != "" {
			addrParts = append(addrParts, v)
		}
	}
	addr := strings.Join(addrParts, ", ")
	if addr == "" {
		addr = name
	}

	website := c.Tags["website"]
	if website == "" {
		website = c.Tags["contact:website"]
	}
	return discoveryTarget{name: name, addr: addr, website: website, lat: c.Lat, lon: c.Lon}
}

// ddgSearchTerm builds the DuckDuckGo query term for a cantor.
func ddgSearchTerm(name, addr, cityName string) string {
	if name != "" {
		return name + " " + cityName
	}
	if addr == "" {
		return kantorPrefix + cityName
	}
	return kantorPrefix + addr + " " + cityName
}

// decodeDDGHref unwraps DuckDuckGo redirect links (…/l/?uddg=<encoded>) and protocol-relative URLs.
func decodeDDGHref(href string) string {
	if strings.Contains(href, "uddg=") {
		parts := strings.Split(href, "uddg=")
		if len(parts) > 1 {
			decoded, _ := url.QueryUnescape(strings.Split(parts[1], "&")[0])
			return decoded
		}
		return ""
	}
	// Sometimes DDG returns direct URLs
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	return href
}

// fetchDDGResultHrefs returns the result links of a DuckDuckGo HTML search.
func (s *citySearch) fetchDDGResultHrefs(searchQuery string) []string {
	ddgReq, _ := http.NewRequest("GET", "https://html.duckduckgo.com/html/?q="+searchQuery, nil)
	ddgReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	ddgResp, err := s.client.Do(ddgReq)
	if err != nil {
		return nil
	}
	defer func() { _ = ddgResp.Body.Close() }()

	doc, err := goquery.NewDocumentFromReader(ddgResp.Body)
	if err != nil {
		return nil
	}

	var hrefs []string
	doc.Find("a.result__url").Each(func(_ int, sel *goquery.Selection) {
		if href, exists := sel.Attr("href"); exists {
			hrefs = append(hrefs, href)
		}
	})
	return hrefs
}

// discoverViaSearch finds a cantor website through DuckDuckGo and runs heuristic discovery on it.
func (s *citySearch) discoverViaSearch(t discoveryTarget, index int, urls *seenURLSet) {
	time.Sleep(time.Duration(index) * 2 * time.Second) // Stagger requests to avoid DDG limits

	searchQuery := url.QueryEscape(fmt.Sprintf("%s kursy walut", ddgSearchTerm(t.name, t.addr, s.city)))
	for _, href := range s.fetchDDGResultHrefs(searchQuery) {
		// The href might look like "//duckduckgo.com/l/?uddg=https%3A%2F%2Fkantor..."
		decoded := decodeDDGHref(href)
		if decoded == "" || isExcludedDiscoveryURL(decoded) {
			continue
		}
		if !urls.claim(decoded) {
			continue
		}
		if s.isStale() {
			return
		}
		if triggerHeuristicDiscovery(decoded, t.name, t.lat, t.lon, t.addr, s.state, s.config, s.window) {
			return // Stop at the first plausible specific URL that worked
		}
	}
}

// resolveWebsites looks up websites for the closest cantors in the background and scrapes their rates.
func (s *citySearch) resolveWebsites(top []cantorWithDistance) {
	urls := &seenURLSet{seen: make(map[string]bool)}
	var wg sync.WaitGroup
	for i, cw := range top {
		if s.isStale() {
			break
		}
		t := newDiscoveryTarget(cw.Element, s.city)
		if t.website == "" { // Find via DDG using precise cantor name
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				s.discoverViaSearch(t, index, urls)
			}(i)
			continue
		}
		if !urls.claim(t.website) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.isStale() {
				return
			}
			triggerHeuristicDiscovery(t.website, t.name, t.lat, t.lon, t.addr, s.state, s.config, s.window)
		}()
	}
	wg.Wait()
}

// DiscoverCityCantors finds physical cantors via Nominatim/Overpass and their URLs via DuckDuckGo.
func DiscoverCityCantors(city string, state *AppState, config AppConfig, window *app.Window) {
	city = strings.TrimSpace(city)
	if city == "" {
		return
	}

	s := newCitySearch(city, state, config, window)
	s.resetForSearch()
	log.Printf("Starting city discovery for: %s", city)

	lat, lon, ok := s.geocodeCity()
	if !ok {
		return
	}
	s.focusMap(lat, lon)
	if s.isStale() {
		return
	}

	// Nominatim fair-use: ≥1s between requests (geocode → amenity).
	time.Sleep(1100 * time.Millisecond)

	cantors := s.collectCantors(lat, lon)
	if s.isStale() {
		return
	}
	if len(cantors) == 0 {
		log.Printf("Discovery: no OSM cantors near %s", city)
		state.UI.MapFocus.CityName = city
		s.fail("err_no_cantors_found")
		return
	}

	s.pinCantors(cantors)
	s.invalidate()

	// Take top 5 — website lookup runs in background (spinner already cleared).
	top := nearestCantors(cantors, lat, lon, 5)
	log.Printf("Discovery: found %d OSM cantors near %s — resolving websites…", len(top), city)
	s.resolveWebsites(top)
}

// LLMDiscoverCityCantors is kept for compatibility. Discovery is heuristic, not LLM-backed.
func LLMDiscoverCityCantors(city string, state *AppState, config AppConfig, window *app.Window) {
	DiscoverCityCantors(city, state, config, window)
}
