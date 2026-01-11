//go:build linux

package utilities

import (
	// Standard libraries
	"errors"
	"fmt"
	"os/exec"
)

// fetchNativeLocation tries to get location via 'geoclue' using gdbus or similar tools.
// Note: Linux location is tricky without CGO/D-Bus bindings. This is a basic attempt.
func fetchNativeLocation() (float64, float64, error) {

	// SonarQube: where-am-i is an optional system tool.
	path, err := exec.LookPath("where-am-i")
	if err == nil && path != "" {
		cmd := exec.Command(path)
		out, err := cmd.Output()
		if err == nil {
			fmt.Printf("Native Location: %s\n", out)
		}
	}

	return 0, 0, errors.New("linux native location not implemented (use IP fallback)")
}
