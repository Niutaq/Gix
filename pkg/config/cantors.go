package config

import (
	// Standard libraries
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a custom type that wraps time.Duration for JSON unmarshaling
type Duration struct {
	time.Duration
}

// CantorConfig represents the static configuration for a single cantor.
type CantorConfig struct {
	ID                  string   `json:"id"`
	DisplayName         string   `json:"displayname"`
	URL                 string   `json:"url"`
	DefaultTimeout      Duration `json:"defaultTimeout"`
	NeedsRateFormatting bool     `json:"needsRateFormatting"`
	Latitude            float64  `json:"latitude"`
	Longitude           float64  `json:"longitude"`
}

// UnmarshalJSON implements json.Unmarshaler interface
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
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
