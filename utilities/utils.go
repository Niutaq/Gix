package utilities

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Structs
type ExchangeRates struct {
	BuyRate  string
	SellRate string
}
type CantorEntry struct {
	URL   string
	Rate  ExchangeRates
	Error string
}

type CantorVault struct {
	Mu        sync.Mutex
	LastEntry *CantorEntry
}

type AppState struct {
	// Main Vault
	Vault *CantorVault

	// Language widgets
	Language string
	EnButton widget.Clickable
	PlButton widget.Clickable
	DeButton widget.Clickable

	// Currency widgets
	Currency  string
	EurButton widget.Clickable
	UsdButton widget.Clickable
	GbpButton widget.Clickable

	// Exchange currency widgets
	TadekButton    widget.Clickable
	KwadratButton  widget.Clickable
	SupersamButton widget.Clickable
	SelectedCantor string

	// Erros, indicators, etc.
	LastInvalidation time.Time
	IsLoading        atomic.Bool
	IsLoadingStart   time.Time
	LastFrameTime    time.Time

	// Gradient
	GradientOffset float32
}

// Colors
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
	Background: color.NRGBA{R: 18, G: 18, B: 18, A: 255},    // Dark background
	Text:       color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // White text
	Error:      color.NRGBA{R: 255, G: 230, B: 20, A: 255},  // Error color
	Success:    color.NRGBA{R: 255, G: 250, B: 10, A: 255},  // Success color
	Title:      color.NRGBA{R: 255, G: 255, B: 0, A: 255},   // Yellow title
	Button:     color.NRGBA{R: 80, G: 80, B: 80, A: 255},    // White button
	Info:       color.NRGBA{R: 0, G: 191, B: 255, A: 255},   // DeepSkyBlue info
	Warning:    color.NRGBA{R: 255, G: 165, B: 0, A: 255},   // Orange warning
	Primary:    color.NRGBA{R: 0, G: 123, B: 255, A: 255},   // Blue primary
	Secondary:  color.NRGBA{R: 108, G: 117, B: 125, A: 255}, // Gray secondary
	Light:      color.NRGBA{R: 248, G: 249, B: 250, A: 255}, // LightGray light
	Dark:       color.NRGBA{R: 0, G: 0, B: 0, A: 255},       // Dark accent
	Accent1:    color.NRGBA{R: 255, G: 255, B: 0, A: 255},   // Yellow accent
	Accent2:    color.NRGBA{R: 255, G: 245, B: 0, A: 255},   // Yellow/Orange accent
	Accent3:    color.NRGBA{R: 255, G: 235, B: 0, A: 255},   // Yellow/Orange v2 accent
	Accent4:    color.NRGBA{R: 100, G: 100, B: 0, A: 255},   // Gradient accent (piece of it)
}

// Language options
var translations = map[string]map[string]string{
	"EN": {
		"title":         "Gix",
		"languageLabel": "Language",
		"subtitle":      "Exchange rates from local currency exchange offices",
		"buyLabel":      "Buy",
		"sellLabel":     "Sell",
	},
	"PL": {
		"title":         "Gix",
		"languageLabel": "Język",
		"subtitle":      "Kursy wymiany walut od lokalnych kantorów",
		"buyLabel":      "Kupno",
		"sellLabel":     "Sprzedaż",
		"errorPrefix":   "Błąd",
	},
	"DE": {
		"title":         "Gix",
		"languageLabel": "Sprache",
		"subtitle":      "Die Wechselkurse der örtlichen Wechselstuben an",
		"buyLabel":      "Kaufen",
		"sellLabel":     "Verkaufen",
		"errorPrefix":   "Fehler",
	},
}

// Abs function
func Abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Function fot exchange rates formatting
func FormatRate(rate string) (string, error) {
	floatRate, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return "", fmt.Errorf("Upss: ", err)
	}

	formattedRate := floatRate / 100

	return fmt.Sprintf("%.3f", formattedRate), nil
}

// Function for loading progress bar
func DrawProgressBar(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	elapsed := time.Since(state.IsLoadingStart).Seconds()
	progress := float32(elapsed) / 0.6
	if progress > 1 {
		progress = 1
	}

	barHeight := gtx.Dp(unit.Dp(12))
	margin := gtx.Dp(unit.Dp(15))
	maxWidth := gtx.Constraints.Max.X - margin*2

	if maxWidth < 1 {
		maxWidth = 1
	}

	bgRadius := barHeight / 2
	bg := clip.RRect{
		Rect: image.Rect(margin, 0, margin+maxWidth, barHeight),
		NE:   bgRadius, NW: bgRadius, SE: bgRadius, SW: bgRadius,
	}
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 50, G: 50, B: 50, A: 200},
		bg.Op(gtx.Ops),
	)

	fillWidth := int(float32(maxWidth) * progress)
	alpha := uint8(255)
	if progress < 0.3 {
		alpha = uint8(255 * (progress / 0.3))
	}

	fill := clip.RRect{
		Rect: image.Rect(margin, 0, margin+fillWidth, barHeight),
		NE:   bgRadius, NW: bgRadius, SE: bgRadius, SW: bgRadius,
	}
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 255, G: 255, B: 0, A: alpha},
		fill.Op(gtx.Ops),
	)

	return layout.Dimensions{
		Size:     image.Point{X: gtx.Constraints.Max.X, Y: barHeight + margin},
		Baseline: 0,
	}
}

// Adding link to links container + sorting
func LayoutVaultLinks(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()

	if state.Vault.LastEntry == nil {
		return layout.Dimensions{}
	}

	entry := *state.Vault.LastEntry
	lang := state.Language
	t := translations[lang]

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			urlText := material.Body1(theme, "")
			urlText.Color = AppColors.Accent1
			urlText.TextSize = unit.Sp(18)
			urlText.Alignment = text.Middle
			return layout.Center.Layout(gtx, urlText.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if entry.Error != "" {
				errorText := material.Body2(theme, t["errorPrefix"]+": "+entry.Error)
				errorText.Color = AppColors.Error
				errorText.TextSize = unit.Sp(16)
				errorText.Alignment = text.Middle
				return layout.Center.Layout(gtx, errorText.Layout)
			} else {
				var buyRate, sellRate string

				if entry.URL == "https://kantory-rzeszow.pl/tabela.htm" || entry.URL == "http://www.kantorsupersam.pl/" {
					buyRate = entry.Rate.BuyRate
					sellRate = entry.Rate.SellRate
				} else {
					buyRate, _ = FormatRate(entry.Rate.BuyRate)
					sellRate, _ = FormatRate(entry.Rate.SellRate)
				}

				buyRateFloat, _ := strconv.ParseFloat(strings.Replace(buyRate, ",", ".", 1), 64)
				sellRateFloat, _ := strconv.ParseFloat(strings.Replace(sellRate, ",", ".", 1), 64)

				rateTextBuy := material.Body2(theme, fmt.Sprintf("%s: %.3f PLN",
					t["buyLabel"],
					buyRateFloat,
				))

				rateTextBuy.TextSize = unit.Sp(28)
				rateTextBuy.Color = AppColors.Success
				rateTextBuy.Alignment = text.Middle

				rateTextSell := material.Body2(theme, fmt.Sprintf("%s: %.3f PLN",
					t["sellLabel"],
					sellRateFloat,
				))

				rateTextSell.TextSize = unit.Sp(28)
				rateTextSell.Color = AppColors.Error
				rateTextSell.Alignment = text.Middle
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Spacer{Height: unit.Dp(50)}.Layout(gtx)
						}),
						layout.Rigid(rateTextBuy.Layout),
						layout.Rigid(rateTextSell.Layout),
					)
				})
			}
		}),
	)
}

// GUI Elements creation - function
func LayoutUI(gtx layout.Context, theme *material.Theme, input *widget.Editor, addButton *widget.Clickable, state *AppState) {
	lang := state.Language
	t := translations[lang]

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
					Spacing:   layout.SpaceSides,
				}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Metric.Dp(unit.Dp(50))
						return EnButtonLayout(gtx, theme, state)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Metric.Dp(unit.Dp(50))
						return PlButtonLayout(gtx, theme, state)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Metric.Dp(unit.Dp(50))
						return DeButtonLayout(gtx, theme, state)
					}),
				)
			})
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 15, Bottom: 15, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Currency buttons
						btn := material.Button(theme, &state.EurButton, "EUR €")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "EUR" {
							btn.Background = AppColors.Accent1
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.UsdButton, "USD $")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "USD" {
							btn.Background = AppColors.Accent2
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.GbpButton, "GBP £")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "GBP" {
							btn.Background = AppColors.Accent3
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
				)
			})
		}),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 100, Bottom: 15, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := material.H1(theme, t["title"])
				title.Alignment = text.Middle
				title.TextSize = unit.Sp(90)
				title.Font.Weight = font.Bold
				title.Color = AppColors.Title
				return title.Layout(gtx)
			})
		}),
		// Subtitle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			subtitle := material.Body1(theme, t["subtitle"])
			subtitle.Alignment = text.Middle
			subtitle.TextSize = unit.Sp(18)
			subtitle.Color = AppColors.Text
			subtitle.Font.Weight = font.Normal
			return subtitle.Layout(gtx)
		}),
		// Subtitle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			subtitle := material.Body1(theme, "\n")
			subtitle.Alignment = text.Middle
			subtitle.TextSize = unit.Sp(22)
			subtitle.Color = AppColors.Text
			subtitle.Font.Weight = font.Normal
			return subtitle.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				button := material.Button(theme, &state.TadekButton, "Kantor Tadek (Stalowa Wola / Rzeszów)")
				button.Background = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
				button.Color = AppColors.Text
				button.TextSize = unit.Sp(16)
				button.Inset = layout.UniformInset(unit.Dp(2))

				if state.SelectedCantor == "tadek" {
					return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Border{
							Color:        AppColors.Accent1,
							Width:        unit.Dp(2),
							CornerRadius: unit.Dp(4),
						}.Layout(gtx, button.Layout)
					})
				}
				return layout.Inset{Bottom: 10}.Layout(gtx, button.Layout)
			}
		}),

		// Analogicznie dla pozostałych przycisków
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				button := material.Button(theme, &state.KwadratButton, "Kantor Kwadrat (Rzeszów)")
				button.Background = color.NRGBA{R: 5, G: 5, B: 5, A: 255}
				button.Color = AppColors.Text
				button.TextSize = unit.Sp(16)
				button.Inset = layout.UniformInset(unit.Dp(2))

				if state.SelectedCantor == "kwadrat" {
					return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Border{
							Color:        AppColors.Accent2,
							Width:        unit.Dp(2),
							CornerRadius: unit.Dp(4),
						}.Layout(gtx, button.Layout)
					})
				}
				return layout.Inset{Bottom: 10}.Layout(gtx, button.Layout)
			}
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				button := material.Button(theme, &state.SupersamButton, "Kantor SuperSam (Rzeszów)")
				button.Background = color.NRGBA{R: 10, G: 10, B: 10, A: 255}
				button.Color = AppColors.Text
				button.TextSize = unit.Sp(16)
				button.Inset = layout.UniformInset(unit.Dp(2))

				if state.SelectedCantor == "supersam" {
					return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Border{
							Color:        AppColors.Accent3,
							Width:        unit.Dp(2),
							CornerRadius: unit.Dp(4),
						}.Layout(gtx, button.Layout)
					})
				}
				return layout.Inset{Bottom: 10}.Layout(gtx, button.Layout)
			}
		}),
	}

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return LayoutVaultLinks(gtx, theme, state)
	}))

	layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)
}

func EnButtonLayout(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	btn := material.Button(theme, &state.EnButton, "EN")
	btn.TextSize = unit.Sp(14)
	btn.Background = AppColors.Dark

	dimensions := btn.Layout(gtx)

	if state.Language == "EN" {
		border := image.Rect(0, 0, dimensions.Size.X, dimensions.Size.Y)

		navy := color.NRGBA{R: 0, G: 35, B: 105, A: 255}
		red := color.NRGBA{R: 200, G: 16, B: 46, A: 255}
		white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

		paint.FillShape(gtx.Ops, navy, clip.Rect(border).Op())

		whiteDiagonalWidth := 15.0
		path := new(clip.Path)
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(4, 4))
		path.LineTo(f32.Pt(float32(border.Max.X)-4, float32(border.Max.Y)-4))
		paint.FillShape(gtx.Ops, white, clip.Stroke{
			Path:  path.End(),
			Width: float32(whiteDiagonalWidth),
		}.Op())

		path2 := new(clip.Path)
		path2.Begin(gtx.Ops)
		path2.MoveTo(f32.Pt(float32(border.Max.X)-4, 4))
		path2.LineTo(f32.Pt(4, float32(border.Max.Y)-4))
		paint.FillShape(gtx.Ops, white, clip.Stroke{
			Path:  path2.End(),
			Width: float32(whiteDiagonalWidth),
		}.Op())

		redDiagonalWidth := 8.0
		path3 := new(clip.Path)
		path3.Begin(gtx.Ops)
		path3.MoveTo(f32.Pt(4, 4))
		path3.LineTo(f32.Pt(float32(border.Max.X)-4, float32(border.Max.Y)-4))
		paint.FillShape(gtx.Ops, red, clip.Stroke{
			Path:  path3.End(),
			Width: float32(redDiagonalWidth),
		}.Op())

		path4 := new(clip.Path)
		path4.Begin(gtx.Ops)
		path4.MoveTo(f32.Pt(float32(border.Max.X)-4, 4))
		path4.LineTo(f32.Pt(4, float32(border.Max.Y)-4))
		paint.FillShape(gtx.Ops, red, clip.Stroke{
			Path:  path4.End(),
			Width: float32(redDiagonalWidth),
		}.Op())

		crossWidth := 15
		cross := clip.Rect{
			Min: image.Pt(border.Max.X/2-crossWidth/2, 0),
			Max: image.Pt(border.Max.X/2+crossWidth/2, border.Max.Y),
		}
		paint.FillShape(gtx.Ops, white, cross.Op())

		verticalCross := clip.Rect{
			Min: image.Pt(0, border.Max.Y/2-crossWidth/2),
			Max: image.Pt(border.Max.X, border.Max.Y/2+crossWidth/2),
		}
		paint.FillShape(gtx.Ops, white, verticalCross.Op())

		innerCrossWidth := 8
		innerCross := clip.Rect{
			Min: image.Pt(border.Max.X/2-innerCrossWidth/2, 0),
			Max: image.Pt(border.Max.X/2+innerCrossWidth/2, border.Max.Y),
		}
		paint.FillShape(gtx.Ops, red, innerCross.Op())

		innerVerticalCross := clip.Rect{
			Min: image.Pt(0, border.Max.Y/2-innerCrossWidth/2),
			Max: image.Pt(border.Max.X, border.Max.Y/2+innerCrossWidth/2),
		}
		paint.FillShape(gtx.Ops, red, innerVerticalCross.Op())

		btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 0}
		btn.Background = color.NRGBA{A: 0}
	} else {
		btn.Background = AppColors.Dark
		btn.Color = AppColors.Text
	}

	return dimensions
}

func PlButtonLayout(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	btn := material.Button(theme, &state.PlButton, "PL")
	btn.TextSize = unit.Sp(14)
	btn.Background = AppColors.Dark

	dimensions := btn.Layout(gtx)

	if state.Language == "PL" {
		border := image.Rect(0, 0, dimensions.Size.X, dimensions.Size.Y)

		white := clip.Rect{
			Min: border.Min,
			Max: image.Pt(border.Max.X, border.Max.Y/2),
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, white.Op())

		red := clip.Rect{
			Min: image.Pt(0, border.Max.Y/2),
			Max: border.Max,
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 0, B: 0, A: 255}, red.Op())

		paint.FillShape(gtx.Ops, color.NRGBA{A: 100},
			clip.Stroke{
				Path:  clip.Rect(border).Path(),
				Width: 6,
			}.Op(),
		)

		btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 0}
		btn.Background = color.NRGBA{A: 0}
	} else {
		btn.Background = AppColors.Dark
		btn.Color = AppColors.Text
	}
	return dimensions
}

func DeButtonLayout(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	btn := material.Button(theme, &state.DeButton, "DE")
	btn.TextSize = unit.Sp(14)
	btn.Background = AppColors.Dark

	dimensions := btn.Layout(gtx)

	if state.Language == "DE" {
		border := image.Rect(0, 0, dimensions.Size.X, dimensions.Size.Y)

		black := clip.Rect{
			Min: border.Min,
			Max: image.Pt(border.Max.X, border.Max.Y/3),
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 255}, black.Op())

		red := clip.Rect{
			Min: image.Pt(0, border.Max.Y/3),
			Max: image.Pt(border.Max.X, 2*border.Max.Y/3),
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 0, B: 0, A: 255}, red.Op())

		gold := clip.Rect{
			Min: image.Pt(0, 2*border.Max.Y/3),
			Max: border.Max,
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 204, B: 0, A: 255}, gold.Op())

		paint.FillShape(gtx.Ops, color.NRGBA{A: 100},
			clip.Stroke{
				Path:  clip.Rect(border).Path(),
				Width: 6,
			}.Op(),
		)

		btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 0}
		btn.Background = color.NRGBA{A: 0}
	} else {
		btn.Background = AppColors.Dark
		btn.Color = AppColors.Text
	}

	return dimensions
}
