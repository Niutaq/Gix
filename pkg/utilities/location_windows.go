//go:build windows

package utilities

import (
	// Standard libraries
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// fetchNativeLocation executes a PowerShell script to retrieve location via Windows Location API.
func fetchNativeLocation() (float64, float64, error) {
	psScript := `
Add-Type -AssemblyName System.Device
$watcher = New-Object System.Device.Location.GeoCoordinateWatcher
$watcher.TryStart($false, [TimeSpan]::FromMilliseconds(5000))
$start = Get-Date
while ($watcher.Status -ne 'Ready' -and ((Get-Date) - $start).TotalSeconds -lt 10) {
    Start-Sleep -Milliseconds 200
}
if ($watcher.Status -eq 'Ready') {
    $loc = $watcher.Position.Location
    if ($loc.IsUnknown -eq $false) {
        Write-Output "$($loc.Latitude),$($loc.Longitude)"
        exit 0
    }
}
exit 1
`
	// SonarQube: powershell is a system command. We use -NoProfile for security.
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("windows location failed (permission or disabled?): %v", err)
	}

	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid output format from powershell")
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
