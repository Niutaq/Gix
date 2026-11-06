package utilities

import (
	// Standard libraries
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
)

// InPoland reports whether lat/lon fall inside a padded PL bounding box (Gix scope).
func InPoland(lat, lon float64) bool {
	return lat >= 49.0 && lat <= 55.1 && lon >= 14.07 && lon <= 24.25
}

// NormalizeLatLon fixes swapped PL coordinates (lon in the latitude slot).
func NormalizeLatLon(lat, lon float64) (float64, float64) {
	// Poland: lat ≈ 49–55, lon ≈ 14–24. Swapped pairs look like lat≈14–24, lon≈49–55.
	if lat >= 14.0 && lat <= 25.0 && lon >= 48.5 && lon <= 55.5 {
		return lon, lat
	}
	return lat, lon
}

// CalculateDistance returns the distance (in km) between two coordinates using the Haversine formula.
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Radius of the Earth in km
	lat1, lon1 = NormalizeLatLon(lat1, lon1)
	lat2, lon2 = NormalizeLatLon(lat2, lon2)

	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	lat1Rad := lat1 * (math.Pi / 180.0)
	lat2Rad := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// GeoLocationResponse matches the response structure from ip-api.com
type GeoLocationResponse struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// FetchUserLocation attempts to retrieve the user's real-world location.
// Order: native OS location → IP geolocation → optional GIX_DEV_COORDS
// (only when GIX_USE_DEV_COORDS=1, for local testing overrides).
func FetchUserLocation() (float64, float64, error) {
	lat, lon, err := fetchNativeLocation()
	if err == nil && lat != 0 && lon != 0 {
		lat, lon = NormalizeLatLon(lat, lon)
		return lat, lon, nil
	}
	if err != nil {
		fmt.Printf("Native Location Error: %v\n", err)
	}

	lat, lon, err = fetchIPLocation()
	if err == nil && lat != 0 && lon != 0 {
		lat, lon = NormalizeLatLon(lat, lon)
		fmt.Printf("IP location fallback: %f, %f (may differ from real GPS)\n", lat, lon)
		return lat, lon, nil
	}
	if err != nil {
		fmt.Printf("IP Location Error: %v\n", err)
	}

	if os.Getenv("GIX_USE_DEV_COORDS") == "1" {
		if lat, lon, ok := parseDevCoords(os.Getenv("GIX_DEV_COORDS")); ok {
			lat, lon = NormalizeLatLon(lat, lon)
			fmt.Printf("Using GIX_DEV_COORDS override: %f, %f\n", lat, lon)
			return lat, lon, nil
		}
	}

	if err != nil {
		return 0, 0, err
	}
	return 0, 0, fmt.Errorf("location unavailable")
}

func parseDevCoords(envCoords string) (float64, float64, bool) {
	parts := strings.Split(envCoords, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// DefaultStartupMaxDistanceKm is the geo filter radius applied after auto-locate on launch.
const DefaultStartupMaxDistanceKm = 50.0

// ApplyUserLocation recenters the map on lat/lon for sorting/distance.
// Pass maxDistanceKm == 0 to keep location active without hiding distant cantors.
// Clears city-search focus — Locate and city search are mutually exclusive owners of the map center.
func ApplyUserLocation(state *AppState, lat, lon float64, maxDistanceKm float64) {
	if lat == 0 && lon == 0 {
		return
	}
	lat, lon = NormalizeLatLon(lat, lon)

	state.UI.UserLocation.Latitude = lat
	state.UI.UserLocation.Longitude = lon
	state.UI.UserLocation.Active = true
	state.UI.MaxDistance = maxDistanceKm
	state.UI.DistanceSlider.Value = 0
	state.UI.MapFocus.CityName = ""
	state.UI.MapState.CenterLat = lat
	state.UI.MapState.CenterLon = lon
	state.UI.MapFocus.Latitude = lat
	state.UI.MapFocus.Longitude = lon
	if state.UI.MapState.Zoom == 0 {
		state.UI.MapState.Zoom = 12
	}
	state.UI.FilteredIDs = nil // force list re-filter
}

// RelaxDistanceFilterIfEmpty widens/clears the radius when the current filter hides every cantor.
func RelaxDistanceFilterIfEmpty(state *AppState) {
	if !state.UI.UserLocation.Active || state.UI.MaxDistance <= 0 {
		return
	}
	if len(filterCantorList(state, state.UI.SearchEditor.Text())) > 0 {
		return
	}

	fmt.Printf("No cantors within %.0fkm — clearing GPS radius filter\n", state.UI.MaxDistance)
	state.UI.MaxDistance = 0
	state.UI.DistanceSlider.Value = 0
	state.UI.FilteredIDs = nil
}

// InitLocationOnStartup resolves the current device location in the background
// so every fresh launch starts centered on where the user actually is.
func InitLocationOnStartup(window *app.Window, state *AppState) {
	go func() {
		lat, lon, err := FetchUserLocation()
		if err != nil {
			fmt.Printf("Startup location failed: %v\n", err)
			return
		}
		lat, lon = NormalizeLatLon(lat, lon)
		// City search already owns the view — keep coords for distance, don't steal the map.
		if state.UI.MapFocus.CityName != "" {
			state.UI.UserLocation.Latitude = lat
			state.UI.UserLocation.Longitude = lon
			fmt.Printf("Startup location: %f, %f (kept city search focus %q)\n", lat, lon, state.UI.MapFocus.CityName)
			window.Invalidate()
			return
		}
		// Center on the user; do not hide distant seeded cantors (MaxDistance=0).
		ApplyUserLocation(state, lat, lon, 0)
		state.UI.SortMode = "DIST"
		RelaxDistanceFilterIfEmpty(state)
		fmt.Printf("Startup location: %f, %f (map centered, showing all cantors by distance)\n", lat, lon)
		window.Invalidate()
	}()
}

// fetchIPLocation retrieves location based on IP address.
func fetchIPLocation() (float64, float64, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	// Use HTTP because the free tier of ip-api.com does not support SSL (HTTPS).
	// SonarQube: Allowing clear-text is safe here as no sensitive data is transmitted.
	resp, err := client.Get("http://ip-api.com/json/") //NOSONAR
	if err != nil {
		return 0, 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}(resp.Body)

	var loc GeoLocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&loc); err != nil {
		return 0, 0, err
	}

	return loc.Lat, loc.Lon, nil
}

// NominatimResponse matches the OSM geocoding response
type NominatimResponse struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// GeocodeCity turns a city name into coordinates using OpenStreetMap Nominatim API
func GeocodeCity(city string) (float64, float64, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return 0, 0, fmt.Errorf("empty city name")
	}

	// Poland-only product: Gix is a PLN kantor monitor, not a global FX atlas.
	q := url.QueryEscape(city)
	endpoint := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1&countrycodes=pl", q)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", UserAgentApp)

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var results []NominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, err
	}

	if len(results) == 0 {
		return 0, 0, fmt.Errorf("city not found in Poland")
	}

	lat, _ := strconv.ParseFloat(results[0].Lat, 64)
	lon, _ := strconv.ParseFloat(results[0].Lon, 64)
	if !InPoland(lat, lon) {
		return 0, 0, fmt.Errorf("city outside Poland")
	}

	return lat, lon, nil
}
