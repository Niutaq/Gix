package utilities

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
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

	// Modal widgets
	ModalOpen             string
	LangModalButton       widget.Clickable
	CurrencyModalButton   widget.Clickable
	ModalClick            widget.Clickable
	LanguageOptions       []string
	CurrencyOptions       []string
	LanguageOptionButtons []widget.Clickable
	CurrencyOptionButtons []widget.Clickable

	// Language widgets
	Language string
	/*EnButton widget.Clickable
	PlButton widget.Clickable
	DeButton widget.Clickable*/

	// Currency widgets
	Currency string
	/*EurButton widget.Clickable
	UsdButton widget.Clickable
	GbpButton widget.Clickable*/

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
	Background: color.NRGBA{R: 0, G: 0, B: 0, A: 255},       // Dark background
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
	Accent4:    color.NRGBA{R: 20, G: 20, B: 0, A: 255},     // Gradient accent (piece of it)
}

// Language options
var translations = map[string]map[string]string{
	"EN": {
		"info":          "Select the language and the currency",
		"title":         "Gix",
		"languageLabel": "Language",
		"buyLabel":      "Buy",
		"sellLabel":     "Sell",
	},
	"PL": {
		"info":          "Wybierz język oraz walutę",
		"title":         "Gix",
		"languageLabel": "Język",
		"buyLabel":      "Kupno",
		"sellLabel":     "Sprzedaż",
		"errorPrefix":   "Błąd",
	},
	"DE": {
		"info":          "Wählen Sie die Sprache und die Währung",
		"title":         "Gix",
		"languageLabel": "Sprache",
		"buyLabel":      "Kaufen",
		"sellLabel":     "Verkaufen",
		"errorPrefix":   "Fehler",
	},
	"DA": {
		"info":          "Vælg sprog og valuta",
		"title":         "Gix",
		"languageLabel": "Sprog",
		"buyLabel":      "Køb",
		"sellLabel":     "Sælg",
	},
	"NO": {
		"info":          "Velg språk og valuta",
		"title":         "Gix",
		"languageLabel": "Språk",
		"buyLabel":      "Kjøp",
		"sellLabel":     "Selg",
	},
	"FR": {
		"info":        "Sélectionnez la langue et la devise",
		"titre":       "Gix",
		"langueLabel": "Langue",
		"buyLabel":    "Acheter",
		"sellLabel":   "Vendre",
	},
	"SW": {
		"info":          "Välj språk och valuta",
		"title":         "Gix",
		"languageLabel": "Språk",
		"buyLabel":      "Köp",
		"sellLabel":     "Sälj",
	},
	"CZ": {
		"info":          "Vyberte jazyk a měnu",
		"title":         "Gix",
		"languageLabel": "Jazyk",
		"buyLabel":      "Koupit",
		"sellLabel":     "Prodat",
	},
	"HR": {
		"info":          "Odaberite jezik i valutu",
		"title":         "Gix",
		"languageLabel": "Jezik",
		"buyLabel":      "Kupi",
		"sellLabel":     "Prodaja",
	},
	"HU": {
		"info":          "Odaberite jezik i valutu",
		"title":         "Gix",
		"languageLabel": "Jezik",
		"buyLabel":      "Kupi",
		"sellLabel":     "Prodaja",
	},
	"UA": {
		"info":          "Виберіть мову та валюту",
		"title":         "Gix",
		"languageLabel": "Мова",
		"buyLabel":      "Купити",
		"sellLabel":     "Продати",
	},
	"BU": {
		"info":          "Изберете език и валута",
		"title":         "Gix",
		"languageLabel": "Език",
		"buyLabel":      "Купете",
		"sellLabel":     "Продавам",
	},
	"RO": {
		"info":          "Selectați limba și moneda",
		"title":         "Gix",
		"languageLabel": "Limba",
		"buyLabel":      "Cumpără",
		"sellLabel":     "Vând",
	},
	"AL": {
		"info":          "Zgjidh gjuhën dhe monedhën",
		"title":         "Gix",
		"languageLabel": "Gjuha",
		"buyLabel":      "Bli",
		"sellLabel":     "Shitet",
	},
	"TR": {
		"info":          "Dil ve para birimini seçin",
		"title":         "Gix",
		"languageLabel": "Dil",
		"buyLabel":      "Satın Al",
		"sellLabel":     "Sat",
	},
	"IC": {
		"info":          "Veldu tungumál og gjaldmiðil",
		"title":         "Gix",
		"languageLabel": "Tungumál",
		"buyLabel":      "Kaupa",
		"sellLabel":     "Selja",
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

	barHeight := gtx.Dp(unit.Dp(8))
	barWidth := gtx.Dp(unit.Dp(200))
	margin := gtx.Dp(unit.Dp(10))

	startX := (gtx.Constraints.Max.X - barWidth) / 2
	if startX < 0 {
		startX = 0
	}

	startY := gtx.Constraints.Max.Y - barHeight - margin

	bgRadius := barHeight / 2
	bgRect := image.Rect(startX, startY, startX+barWidth, startY+barHeight)
	bg := clip.RRect{
		Rect: bgRect,
		NE:   bgRadius, NW: bgRadius, SE: bgRadius, SW: bgRadius,
	}
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 15, G: 15, B: 15, A: 100},
		bg.Op(gtx.Ops),
	)

	fillWidth := int(float32(barWidth) * progress)
	alpha := uint8(255)
	if progress < 0.3 {
		alpha = uint8(255 * (progress / 0.3))
	}

	fillRect := image.Rect(startX, startY, startX+fillWidth, startY+barHeight)
	fill := clip.RRect{
		Rect: fillRect,
		NE:   bgRadius, NW: bgRadius, SE: bgRadius, SW: bgRadius,
	}
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 255, G: 255, B: 0, A: alpha},
		fill.Op(gtx.Ops),
	)

	return layout.Dimensions{
		Size:     image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y},
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
				if entry.Rate.BuyRate == "" {
					return DrawProgressBar(gtx, theme, state)
				}
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

		// Subtitle empty
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			subtitle := material.Body1(theme, "\n")
			subtitle.Alignment = text.Middle
			subtitle.TextSize = unit.Sp(5)
			subtitle.Color = AppColors.Text
			subtitle.Font.Weight = font.Normal
			return subtitle.Layout(gtx)
		}),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 50, Bottom: 15, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := material.H1(theme, t["title"])
				title.Alignment = text.Middle
				title.TextSize = unit.Sp(90)
				title.Font.Weight = font.Bold
				title.Color = AppColors.Title
				return title.Layout(gtx)
			})
		}),

		// Info
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			info := material.Body1(theme, t["info"])
			info.Alignment = text.Middle
			info.TextSize = unit.Sp(18)
			info.Color = AppColors.Text
			info.Font.Weight = font.Normal
			if lang == "UA" || lang == "BU" {
				info.Font.Weight = font.SemiBold
				info.Font.Style = font.Regular
			}
			return info.Layout(gtx)
		}),

		// Buttons for language and currency choose
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:    layout.Horizontal,
					Spacing: layout.SpaceSides,
				}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.LangModalButton, ""+state.Language)
						btn.Color = AppColors.Text
						btn.Background = AppColors.Background
						if state.LangModalButton.Clicked(gtx) {
							state.ModalOpen = "language"
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(11)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.CurrencyModalButton, ""+state.Currency)
						btn.Color = AppColors.Text
						btn.Background = AppColors.Background
						if state.CurrencyModalButton.Clicked(gtx) {
							state.ModalOpen = "currency"
						}
						return btn.Layout(gtx)
					}),
				)
			})
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				return layout.Inset{Top: unit.Dp(120), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxWidth := gtx.Dp(unit.Dp(600))
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxWidth)

					button := material.Button(theme, &state.TadekButton, "Kantor Tadek (Stalowa Wola / Rzeszów)")
					button.Background = color.NRGBA{R: 0, G: 0, B: 0, A: 230}
					button.Color = AppColors.Text
					button.TextSize = unit.Sp(16)
					button.Inset = layout.UniformInset(unit.Dp(10))

					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if state.SelectedCantor == "tadek" {
							return widget.Border{
								Color:        AppColors.Accent1,
								Width:        unit.Dp(2),
								CornerRadius: unit.Dp(4),
							}.Layout(gtx, button.Layout)
						}
						return button.Layout(gtx)
					})
				})
			}
		}),

		// For other cantors - the same pattern
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				return layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxWidth := gtx.Dp(unit.Dp(600))
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxWidth)

					button := material.Button(theme, &state.KwadratButton, "Kantor Kwadrat (Rzeszów)")
					button.Background = color.NRGBA{R: 0, G: 0, B: 0, A: 230}
					button.Color = AppColors.Text
					button.TextSize = unit.Sp(16)
					button.Inset = layout.UniformInset(unit.Dp(10))

					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if state.SelectedCantor == "kwadrat" {
							return widget.Border{
								Color:        AppColors.Accent1,
								Width:        unit.Dp(2),
								CornerRadius: unit.Dp(4),
							}.Layout(gtx, button.Layout)
						}
						return button.Layout(gtx)
					})
				})
			}
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() {
				return DrawProgressBar(gtx, theme, state)
			} else {
				return layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					maxWidth := gtx.Dp(unit.Dp(600))
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, maxWidth)

					button := material.Button(theme, &state.SupersamButton, "Kantor SuperSam (Rzeszów)")
					button.Background = color.NRGBA{R: 0, G: 0, B: 0, A: 230}
					button.Color = AppColors.Text
					button.TextSize = unit.Sp(16)
					button.Inset = layout.UniformInset(unit.Dp(10))

					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if state.SelectedCantor == "supersam" {
							return widget.Border{
								Color:        AppColors.Accent1,
								Width:        unit.Dp(2),
								CornerRadius: unit.Dp(4),
							}.Layout(gtx, button.Layout)
						}
						return button.Layout(gtx)
					})
				})
			}
		}),
	}

	layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if state.ModalOpen != "" {
				switch state.ModalOpen {
				case "language":
					return LanguageModal(gtx, theme, state)
				case "currency":
					return CurrencyModal(gtx, theme, state)
				}
			}
			return layout.Dimensions{}
		}),
	)

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return LayoutVaultLinks(gtx, theme, state)
	}))

	layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)

}

func LanguageModal(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	return ModalOverlay(gtx, theme, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			"↓",
			state.LanguageOptions,
			state.LanguageOptionButtons,
			func(lang string) {
				state.Language = lang
				state.ModalOpen = ""
			},
		)
	})
}

func CurrencyModal(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	return ModalOverlay(gtx, theme, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			"↓",
			state.CurrencyOptions,
			state.CurrencyOptionButtons,
			func(currency string) {
				state.Currency = currency
				state.ModalOpen = ""
			},
		)
	})
}

func ModalOverlay(gtx layout.Context, theme *material.Theme, state *AppState, content layout.Widget) layout.Dimensions {
	if state.ModalClick.Clicked(gtx) {
		state.ModalOpen = ""
	}

	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 150})

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return state.ModalClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{
					Color:        AppColors.Accent1,
					Width:        unit.Dp(0),
					CornerRadius: unit.Dp(6),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
				})
			})
		}),
	)
}

func ModalDialog(
	gtx layout.Context,
	theme *material.Theme,
	title string,
	options []string,
	buttons []widget.Clickable,
	onSelect func(string),
) layout.Dimensions {
	var widgets []layout.FlexChild

	widgets = append(widgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		lbl := material.H6(theme, title)
		lbl.Color = AppColors.Text
		return lbl.Layout(gtx)
	}))

	for i := range options {
		if i >= len(buttons) {
			break
		}

		i := i
		option := options[i]
		btn := &buttons[i]

		widgets = append(widgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if btn.Clicked(gtx) {
				onSelect(option)
			}

			button := material.Button(theme, btn, option)
			button.Background = color.NRGBA{R: 25, G: 25, B: 25, A: 255}
			button.Color = AppColors.Text
			button.Inset = layout.UniformInset(unit.Dp(5))
			button.TextSize = unit.Sp(16)

			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5)}.Layout(gtx, button.Layout)
		}))
	}

	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceEnd,
	}.Layout(gtx, widgets...)
}
