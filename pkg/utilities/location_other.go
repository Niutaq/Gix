//go:build !darwin && !windows && !linux

package utilities

// Standard libraries
import "errors"

// fetchNativeLocation is a stub for unsupported systems.
func fetchNativeLocation() (float64, float64, error) {
	return 0, 0, errors.New("not supported on this OS")
}
