package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Duration is a custom type that wraps time.Duration for JSON unmarshaling
type Duration struct {
	time.Duration
}

// UnmarshalJSON implements json.Unmarshaler interface
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid duration type: %T", value)
	}
}

// CantorConfig represents the static configuration for a single cantor.
type CantorConfig struct {
	ID                  string   `json:"id"`
	DisplayName         string   `json:"displayname"`
	URL                 string   `json:"url"`
	DefaultTimeout      Duration `json:"defaultTimeout"`
	NeedsRateFormatting bool     `json:"needsRateFormatting"`
}

// LoadCantorConfig reads a JSON file and unmarshal it into a slice of CantorConfig.
func LoadCantorConfig(filePath string) ([]CantorConfig, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read cantor config file %s: %w", filePath, err)
	}

	var configs []CantorConfig
	if err := json.Unmarshal(fileData, &configs); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal cantor config from %s: %w", filePath, err)
	}

	return configs, nil
}

// LoadCantorConfigFromBytes unmarshalls byte data into a slice of CantorConfig.
func LoadCantorConfigFromBytes(data []byte) ([]CantorConfig, error) {
	var configs []CantorConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal cantor config from bytes: %w", err)
	}
	return configs, nil
}
