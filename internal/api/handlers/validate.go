package handlers

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxCantorNameLen    = 120
	maxCantorAddressLen = 200
)

// sanitizeDisplayName strips control chars and bounds length (NetSecOps ingest).
func sanitizeDisplayName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > maxCantorNameLen {
		s = s[:maxCantorNameLen]
	}
	return s
}

func sanitizeAddressField(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > maxCantorAddressLen {
		s = s[:maxCantorAddressLen]
	}
	return s
}

func validLatLon(lat, lon float64) bool {
	if lat == 0 && lon == 0 {
		return true // unknown / optional
	}
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

func validateDiscoverMeta(name, addr string, lat, lon float64) error {
	if !validLatLon(lat, lon) {
		return fmt.Errorf("invalid coordinates")
	}
	if len(name) > maxCantorNameLen || len(addr) > maxCantorAddressLen {
		return fmt.Errorf("metadata too long")
	}
	return nil
}
