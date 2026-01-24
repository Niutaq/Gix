package utilities

// Standard libraries
import "image/color"

// AppColors Colors
// It defines the color palette used throughout the application.
var AppColors = struct {
	Background  color.NRGBA
	Text        color.NRGBA
	Error       color.NRGBA
	Success     color.NRGBA
	Title       color.NRGBA
	Button      color.NRGBA
	Info        color.NRGBA
	Warning     color.NRGBA
	Primary     color.NRGBA
	Secondary   color.NRGBA
	Light       color.NRGBA
	Dark        color.NRGBA
	Accent1     color.NRGBA
	Accent2     color.NRGBA
	Accent3     color.NRGBA
	Accent4     color.NRGBA
	Accent1Dark color.NRGBA
	Spread      color.NRGBA
}{
	Background:  color.NRGBA{R: 20, G: 20, B: 20, A: 155},
	Text:        color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Error:       color.NRGBA{R: 225, G: 50, B: 50, A: 255},
	Success:     color.NRGBA{R: 76, G: 175, B: 80, A: 255},
	Title:       color.NRGBA{R: 255, G: 184, B: 0, A: 255},
	Button:      color.NRGBA{R: 100, G: 100, B: 100, A: 165},
	Info:        color.NRGBA{R: 0, G: 190, B: 255, A: 255},
	Warning:     color.NRGBA{R: 255, G: 184, B: 0, A: 255},
	Primary:     color.NRGBA{R: 0, G: 125, B: 255, A: 255},
	Secondary:   color.NRGBA{R: 110, G: 115, B: 125, A: 255},
	Light:       color.NRGBA{R: 20, G: 18, B: 25, A: 255},
	Dark:        color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Accent1:     color.NRGBA{R: 255, G: 184, B: 0, A: 255},
	Accent2:     color.NRGBA{R: 255, G: 184, B: 0, A: 255},
	Accent3:     color.NRGBA{R: 255, G: 184, B: 0, A: 255},
	Accent4:     color.NRGBA{R: 20, G: 20, B: 0, A: 255},
	Accent1Dark: color.NRGBA{R: 180, G: 130, B: 0, A: 255},
	Spread:      color.NRGBA{R: 255, G: 200, B: 0, A: 135},
}
