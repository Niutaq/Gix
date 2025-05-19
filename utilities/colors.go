package utilities

import "image/color"

// Colors
// It defines the color palette used throughout the application.
var AppColors = struct {
	Background color.NRGBA
	Text       color.NRGBA
	Error      color.NRGBA
	Success    color.NRGBA
	Title      color.NRGBA
	Button     color.NRGBA
	Info       color.NRGBA
	Warning    color.NRGBA
	Primary    color.NRGBA
	Secondary  color.NRGBA
	Light      color.NRGBA
	Dark       color.NRGBA
	Accent1    color.NRGBA
	Accent2    color.NRGBA
	Accent3    color.NRGBA
	Accent4    color.NRGBA
}{
	Background: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Text:       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Error:      color.NRGBA{R: 255, G: 230, B: 20, A: 255},
	Success:    color.NRGBA{R: 255, G: 250, B: 130, A: 255},
	Title:      color.NRGBA{R: 255, G: 255, B: 0, A: 255},
	Button:     color.NRGBA{R: 80, G: 80, B: 80, A: 255},
	Info:       color.NRGBA{R: 0, G: 191, B: 255, A: 255},
	Warning:    color.NRGBA{R: 255, G: 165, B: 0, A: 255},
	Primary:    color.NRGBA{R: 0, G: 123, B: 255, A: 255},
	Secondary:  color.NRGBA{R: 108, G: 117, B: 125, A: 255},
	Light:      color.NRGBA{R: 248, G: 249, B: 250, A: 255},
	Dark:       color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Accent1:    color.NRGBA{R: 255, G: 255, B: 0, A: 255},
	Accent2:    color.NRGBA{R: 255, G: 245, B: 0, A: 255},
	Accent3:    color.NRGBA{R: 255, G: 235, B: 0, A: 255},
	Accent4:    color.NRGBA{R: 20, G: 20, B: 0, A: 255},
}
