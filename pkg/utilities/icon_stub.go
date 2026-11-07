//go:build !darwin

package utilities

// SetDockIcon is a no-op outside macOS.
func SetDockIcon(png []byte) {
	// Non-darwin builds have no dock/taskbar icon API here.
	_ = png
}
