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
	Cantors map[string]*CantorInfo            `json:"cantors"`
	Rates   map[string]map[string]*CantorEntry `json:"rates"`
	SavedAt time.Time                         `json:"savedAt"`
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
	content, err := os.ReadFile(path)
	if err != nil {
		log.Println("No cache found or failed to open:", err)
		return
	}

	var data CachedData
	if err := json.Unmarshal(content, &data); err != nil {
		// Migration: try to load old format
		type oldCachedData struct {
			Cantors map[string]*CantorInfo  `json:"cantors"`
			Rates   map[string]*CantorEntry `json:"rates"`
			SavedAt time.Time               `json:"savedAt"`
		}
		var oldData oldCachedData
		if err2 := json.Unmarshal(content, &oldData); err2 == nil {
			log.Println("Migrating old cache format...")
			data.Cantors = oldData.Cantors
			data.SavedAt = oldData.SavedAt
			data.Rates = make(map[string]map[string]*CantorEntry)
			// Put old rates under EUR as a guess, or just leave empty and wait for first save
			data.Rates["EUR"] = oldData.Rates
		} else {
			log.Printf("Failed to decode cache (new and old format): %v", err)
			return
		}
	}

	if data.Rates == nil {
		data.Rates = make(map[string]map[string]*CantorEntry)
	}

	// Filter out 'alex' (legacy/performance issue)
	delete(data.Cantors, "alex")
	for curr := range data.Rates {
		if data.Rates[curr] == nil {
			data.Rates[curr] = make(map[string]*CantorEntry)
		}
		delete(data.Rates[curr], "alex")
	}

	state.CantorsMu.Lock()
	state.Cantors = data.Cantors
	state.CantorsMu.Unlock()

	state.Vault.Mu.Lock()
	state.Vault.Rates = data.Rates
	// Reset animations so they "fly in" on startup
	for _, currMap := range state.Vault.Rates {
		for _, entry := range currMap {
			entry.AppearanceSpring.Current = 0
			entry.AppearanceSpring.Target = 1
			entry.AppearanceSpring.Velocity = 0
			entry.AppearanceSpring.Tension = 150
			entry.AppearanceSpring.Friction = 22
		}
	}
	state.Vault.Mu.Unlock()

	log.Printf("Cache loaded successfully (SavedAt: %s). Cantors: %d, Currencies in cache: %d",
		data.SavedAt.Format(time.RFC3339), len(data.Cantors), len(data.Rates))
}
