package utilities

import _ "embed"

//go:embed res/appicon.png
var AppIconPNG []byte

// ApplyAppIcon sets the OS dock/taskbar icon when supported.
func ApplyAppIcon() {
	SetDockIcon(AppIconPNG)
}
