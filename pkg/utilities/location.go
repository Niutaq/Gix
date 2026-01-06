package utilities

import (
	// Standard libraries
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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
	if runtime.GOOS == "darwin" {
		lat, lon, err := fetchMacOSLocation()
		if err == nil {
			return lat, lon, nil
		}
		// If native fetch fails, do NOT fallback to IP as it may be inaccurate (e.g. Kalisz vs Stalowa Wola).
		// Return the error so the UI keeps the default coordinates.
		return 0, 0, err
	}

	return fetchIPLocation()
}

// fetchIPLocation retrieves location based on IP address.
func fetchIPLocation() (float64, float64, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

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

// fetchMacOSLocation executes a Swift script to get the location from CoreLocation.
func fetchMacOSLocation() (float64, float64, error) {
	const swiftScript = `
import CoreLocation
import Foundation

class LocationDelegate: NSObject, CLLocationManagerDelegate {
    let manager = CLLocationManager()
    
    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
    }
    
    func start() {
        manager.requestWhenInUseAuthorization()
        manager.startUpdatingLocation()
    }
    
    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        if let location = locations.first {
            print("\(location.coordinate.latitude),\(location.coordinate.longitude)")
            exit(0)
        }
    }
    
    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        exit(1)
    }
}

let delegate = LocationDelegate()
delegate.start()
RunLoop.main.run(until: Date(timeIntervalSinceNow: 5))
exit(1)
`
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "gix_loc_*.swift")
	if err != nil {
		return 0, 0, err
	}
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			fmt.Printf("Error removing temp file: %v\n", err)
		}
	}(tmpFile.Name())

	if _, err := tmpFile.WriteString(swiftScript); err != nil {
		return 0, 0, err
	}
	if err := tmpFile.Close(); err != nil {
		return 0, 0, err
	}

	// Run swift
	cmd := exec.Command("swift", tmpFile.Name())
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	// Parse output "lat,lon"
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid output format")
	}

	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, err
	}
	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}
