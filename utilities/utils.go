package utilities

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	// "sync"
	// "sync/atomic"
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

// Abs returns the absolute value of a float32.
func Abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// FormatRate converts a string representation of a rate (assumed to be in smallest currency units)
// into a formatted string with three decimal places (e.g., "1.234").
// It divides the input rate by 100 before formatting.
func FormatRate(rate string) (string, error) {
	floatRate, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return "", fmt.Errorf("Error: %v", err)
	}

	formattedRate := floatRate / 100

	return fmt.Sprintf("%.3f", formattedRate), nil
}

// DrawProgressBar renders a progress bar based on the loading state in AppState.
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

// LayoutVaultLinks displays the fetched exchange rates or an error message from the CantorVault.
// It also shows a progress bar if rates are being fetched (BuyRate is empty and no error).
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
