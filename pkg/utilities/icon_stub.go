//go:build !darwin

package utilities

// SetDockIcon is a no-op outside macOS.
func SetDockIcon(png []byte) {}
