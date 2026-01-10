package utilities

import (
	// Standard libraries
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// CalculateDistance returns the distance (in km) between two coordinates using the Haversine formula.
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Radius of the Earth in km

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

// FetchUserLocation attempts to retrieve the user's location.
// On macOS, it tries to use CoreLocation via a temporary Swift script.
// Otherwise, it falls back to an IP-based location.
func FetchUserLocation() (float64, float64, error) {
	if envCoords := os.Getenv("GIX_DEV_COORDS"); envCoords != "" {
		parts := strings.Split(envCoords, ",")
		if len(parts) == 2 {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil {
				return lat, lon, nil
			}
		}
	}

	lat, lon, err := fetchNativeLocation()
	if err == nil {
		return lat, lon, nil
	}
	fmt.Printf("Native Location Error: %v\n", err)

	return fetchIPLocation()
}

// fetchIPLocation retrieves location based on IP address.
func fetchIPLocation() (float64, float64, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	// Use HTTP because the free tier of ip-api.com does not support SSL (HTTPS).
	// SonarQube: Allowing clear-text is safe here as no sensitive data is transmitted.
	resp, err := client.Get("http://ip-api.com/json/")
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
