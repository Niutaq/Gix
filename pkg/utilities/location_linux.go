//go:build linux

package utilities

import (
	// Standard libraries
	"errors"
	"os/exec"
)

// fetchNativeLocation tries to get location via 'geoclue' using gdbus or similar tools.
// Note: Linux location is tricky without CGO/D-Bus bindings. This is a basic attempt.
func fetchNativeLocation() (float64, float64, error) {

	path, err := exec.LookPath("where-am-i")
	if err == nil && path != "" {
		cmd := exec.Command("where-am-i")
		out, err := cmd.Output()
		if err == nil {
			fmt.Printf("Native Location: %s\n", out)
		}
	}

	return 0, 0, errors.New("linux native location not implemented (use IP fallback)")
}
