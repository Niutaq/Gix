package utilities

import (
	// Standard libraries
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// CachedData represents the cached application state.
type CachedData struct {
	Cantors map[string]*CantorInfo  `json:"cantors"`
	Rates   map[string]*CantorEntry `json:"rates"`
	SavedAt time.Time               `json:"savedAt"`
}

// GetCachePath returns the path to the cache file.
func GetCachePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "gix_cache.json"
	}
	appDir := filepath.Join(configDir, "gix")
	_ = os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "cache.json")
}

// SaveCache saves the current application state (cantors and rates) to a local JSON file.
func SaveCache(state *AppState) {
	state.CantorsMu.RLock()
	cantors := state.Cantors
	state.CantorsMu.RUnlock()

	state.Vault.Mu.Lock()
	rates := state.Vault.Rates
	state.Vault.Mu.Unlock()

	data := CachedData{
		Cantors: cantors,
		Rates:   rates,
		SavedAt: time.Now(),
	}

	path := GetCachePath()
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to create cache file at %s: %v", path, err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing cache file (save): %v", err)
		}
	}()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		log.Printf("Failed to encode cache: %v", err)
	} else {
		log.Printf("Cache saved to %s", path)
	}
}

// LoadCache loads the application state from a local JSON file.
func LoadCache(state *AppState) {
	path := GetCachePath()
	file, err := os.Open(path)
	if err != nil {
		log.Println("No cache found or failed to open:", err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing cache file (load): %v", err)
		}
	}()

	var data CachedData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		log.Printf("Failed to decode cache: %v", err)
		return
	}

	// Filter out 'alex' (legacy/performance issue)
	delete(data.Cantors, "alex")
	delete(data.Rates, "alex")

	state.CantorsMu.Lock()
	state.Cantors = data.Cantors
	state.CantorsMu.Unlock()

	state.Vault.Mu.Lock()
	state.Vault.Rates = data.Rates
	state.Vault.Mu.Unlock()

	log.Printf("Cache loaded successfully (SavedAt: %s)", data.SavedAt.Format(time.RFC3339))
}
