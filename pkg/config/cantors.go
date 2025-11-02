package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CantorConfig represents the static configuration for a single cantor.
type CantorConfig struct {
	ID                  string        `json:"id"`
	Displayname         string        `json:"displayname"`
	URL                 string        `json:"url"`
	DefaultTimeout      time.Duration `json:"defaultTimeout"`
	NeedsRateFormatting bool          `json:"needsRateFormatting"`
}

// LoadCantorConfig reads a JSON file and unmarshals it into a slice of CantorConfig.
func LoadCantorConfig(filePath string) ([]CantorConfig, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cantor config file %s: %w", filePath, err)
	}

	var configs []CantorConfig
	if err := json.Unmarshal(fileData, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cantor config from %s: %w", filePath, err)
	}

	return configs, nil
}

// LoadCantorConfigFromBytes unmarshals byte data into a slice of CantorConfig.
func LoadCantorConfigFromBytes(data []byte) ([]CantorConfig, error) {
	var configs []CantorConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cantor config from bytes: %w", err)
	}
	return configs, nil
}
