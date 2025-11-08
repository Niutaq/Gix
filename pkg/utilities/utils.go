package utilities

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"sort"

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

// DrawProgressBar renders a progress bar based on the loading state in AppState.
func DrawProgressBar(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	elapsed := time.Since(state.IsLoadingStart).Seconds()

	var totalDuration float32 = 10.0

	progress := float32(elapsed) / totalDuration
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
		color.NRGBA{R: 230, G: 135, B: 0, A: alpha},
		fill.Op(gtx.Ops),
	)

	return layout.Dimensions{
		Size:     image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y},
		Baseline: 0,
	}
}

// LayoutVaultLinks displays the fetched exchange rates or an error message from the CantorVault.
func LayoutVaultLinks(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	state.Vault.Mu.Lock()
	entry := state.Vault.LastEntry
	state.Vault.Mu.Unlock()

	if entry == nil || state.UI.SelectedCantor == "" {
		return layout.Dimensions{}
	}

	if entry.Error != "" {
		errorPrefix := GetTranslation(state.UI.Language, "errorPrefix")

		translatedError := GetTranslation(state.UI.Language, entry.Error)

		if translatedError == entry.Error {
			translatedError = GetTranslation(state.UI.Language, "err_unknown")
		}

		errorMsg := errorPrefix + ": " + translatedError

		errorText := material.Body1(theme, errorMsg)
		errorText.Color = AppColors.Error
		errorText.TextSize = unit.Sp(18)
		errorText.Alignment = text.Middle

		return layout.Inset{Top: unit.Dp(30), Bottom: unit.Dp(20)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, errorText.Layout)
			},
		)
	}

	if entry.Rate.BuyRate == "" && entry.Rate.SellRate == "" {
		return layout.Dimensions{}
	}

	buyRateStr := entry.Rate.BuyRate
	sellRateStr := entry.Rate.SellRate

	buyRateFloat, errB := strconv.ParseFloat(strings.ReplaceAll(buyRateStr, ",", "."), 64)
	sellRateFloat, errS := strconv.ParseFloat(strings.ReplaceAll(sellRateStr, ",", "."), 64)

	if errB != nil || errS != nil {
		errorMsg := GetTranslation(state.UI.Language, "errorPrefix") + ": " + GetTranslation(state.UI.Language, "invalidRateFormat")
		errorText := material.Body1(theme, errorMsg)
		errorText.Color = AppColors.Error
		errorText.TextSize = unit.Sp(18)
		errorText.Alignment = text.Middle
		return layout.Center.Layout(gtx, errorText.Layout)
	}

	// ... (reszta kodu wyświetlania bez zmian) ...
	buyLabel := GetTranslation(state.UI.Language, "buyLabel")
	sellLabel := GetTranslation(state.UI.Language, "sellLabel")

	rateTextBuy := material.H4(theme, fmt.Sprintf("%s: %.3f %s",
		buyLabel,
		buyRateFloat,
		state.UI.Currency,
	))

	rateTextBuy.Color = AppColors.Accent2
	rateTextBuy.Alignment = text.Middle

	rateTextSell := material.H4(theme, fmt.Sprintf("%s: %.3f %s",
		sellLabel,
		sellRateFloat,
		state.UI.Currency,
	))

	rateTextSell.Color = AppColors.Accent3
	rateTextSell.Alignment = text.Middle

	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(layout.Spacer{Height: unit.Dp(15)}.Layout),
			layout.Rigid(rateTextBuy.Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(rateTextSell.Layout),
		)
	})
}

// LayoutUI - GUI Elements creation - function
func LayoutUI(gtx layout.Context, theme *material.Theme, state *AppState) {
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(layout.Spacer{Height: unit.Dp(30)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					title := material.H1(theme, GetTranslation(state.UI.Language, "title"))
					title.Alignment = text.Middle
					title.TextSize = unit.Sp(140)
					title.Font.Weight = font.Bold
					title.Color = AppColors.Title
					return title.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					info := material.Body1(theme, GetTranslation(state.UI.Language, "info"))
					info.Alignment = text.Middle
					info.TextSize = unit.Sp(17)
					info.Color = AppColors.Text
					info.Font.Weight = font.Normal
					if state.UI.Language == "UA" || state.UI.Language == "BU" {
						info.Font.Weight = font.SemiBold
					}
					return info.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(15)}.Layout),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Spacing:   layout.SpaceAround,
						Alignment: layout.Middle,
					}.Layout(gtx,
						// Language button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {

							btn := material.Button(theme, &state.UI.LangModalButton, state.UI.Language)
							btn.Color = AppColors.Accent1
							btn.Background = color.NRGBA{A: 0}
							btn.CornerRadius = unit.Dp(6)
							btn.TextSize = unit.Sp(16)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8),
								Right: unit.Dp(8)}
							if state.UI.LangModalButton.Clicked(gtx) {
								state.UI.ModalOpen = "language"
							}
							return btn.Layout(gtx)
						}),

						// Currency button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {

							btn := material.Button(theme, &state.UI.CurrencyModalButton, state.UI.Currency)
							btn.Color = AppColors.Accent1
							btn.Background = color.NRGBA{A: 0}
							btn.CornerRadius = unit.Dp(6)
							btn.TextSize = unit.Sp(16)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8),
								Right: unit.Dp(8)}
							if state.UI.CurrencyModalButton.Clicked(gtx) {
								state.UI.ModalOpen = "currency"
							}
							return btn.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
			)
		}),

		// Section for Cantor selection
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			list := widget.List{}
			list.Axis = layout.Vertical

			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				maxWidth := gtx.Dp(unit.Dp(400))
				if maxWidth > gtx.Constraints.Max.X-gtx.Dp(unit.Dp(20)) {
					maxWidth = gtx.Constraints.Max.X - gtx.Dp(unit.Dp(20))
				}
				gtx.Constraints.Max.X = maxWidth

				cantorIDs := make([]string, 0, len(state.Cantors))
				for id := range state.Cantors {
					cantorIDs = append(cantorIDs, id)
				}
				sort.Strings(cantorIDs)

				return material.List(theme, &list).Layout(gtx, len(cantorIDs),
					func(gtx layout.Context, i int) layout.Dimensions {
						cantorKey := cantorIDs[i]
						cantor := state.Cantors[cantorKey]

						displayName := GetTranslation(state.UI.Language, cantor.DisplayName)

						return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X

								button := material.Button(theme, &cantor.Button, displayName)
								button.Background = color.NRGBA{R: 25, G: 25, B: 25, A: 50}
								button.Color = AppColors.Text
								button.TextSize = unit.Sp(16)
								button.Inset = layout.UniformInset(unit.Dp(12))
								button.CornerRadius = unit.Dp(8)

								if state.UI.SelectedCantor == cantorKey {
									return widget.Border{
										Color:        AppColors.Accent1,
										Width:        unit.Dp(2.5),
										CornerRadius: button.CornerRadius,
									}.Layout(gtx, button.Layout)
								}
								return button.Layout(gtx)
							})
					})
			})
		}),

		// Section for loading progress bar
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() && state.UI.SelectedCantor != "" {
				return layout.Inset{Top: unit.Dp(15), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return DrawProgressBar(gtx, theme, state)
					})
				})
			}
			return layout.Dimensions{}
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return LayoutVaultLinks(gtx, theme, state)
			})
		}),
	)

	if state.UI.ModalOpen != "" {
		var modalContent layout.Widget
		switch state.UI.ModalOpen {
		case "language":
			modalContent = func(gtx layout.Context) layout.Dimensions {
				return LanguageModal(gtx, theme, state)
			}
		case "currency":
			modalContent = func(gtx layout.Context) layout.Dimensions {
				return CurrencyModal(gtx, theme, state)
			}
		}
		if modalContent != nil {
			modalContent(gtx)
		}
	}
}

// LanguageModal - Modal for language selection
func LanguageModal(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.UI.Language, "===")
	if lbl := GetTranslation(state.UI.Language, "selectLanguageTitle"); lbl != "selectLanguageTitle" {
		title = lbl
	}
	return ModalOverlay(gtx, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			title,
			state.UI.LanguageOptions,
			state.UI.LanguageOptionButtons,
			func(lang string) {
				state.UI.Language = lang
				state.UI.ModalOpen = ""
			},
			state,
		)
	})
}

// CurrencyModal - Modal for currency selection
func CurrencyModal(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.UI.Currency, "===")
	if lbl := GetTranslation(state.UI.Currency, "selectCurrencyTitle"); lbl != "selectCurrencyTitle" {
		title = lbl
	}
	return ModalOverlay(gtx, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			title,
			state.UI.CurrencyOptions,
			state.UI.CurrencyOptionButtons,
			func(currency string) {
				state.UI.Currency = currency
				state.UI.ModalOpen = ""
			},
			state,
		)
	})
}

// ModalOverlay - Modal overlay with click outside to close
func ModalOverlay(gtx layout.Context, state *AppState, content layout.Widget) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{A: 210})

	clickedOutside := state.UI.ModalClick.Clicked(gtx)

	result := layout.Stack{Alignment: layout.S}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return state.UI.ModalClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(50)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {

				var modalMaxWidth, modalMaxHeight int
				switch state.UI.ModalOpen {
				case "language":
					modalMaxWidth = gtx.Dp(unit.Dp(350))
					modalMaxHeight = gtx.Dp(unit.Dp(500))
				case "currency":
					modalMaxWidth = gtx.Dp(unit.Dp(350))
					modalMaxHeight = gtx.Dp(unit.Dp(500))
				default:
					modalMaxWidth = gtx.Dp(unit.Dp(350))
					modalMaxHeight = gtx.Dp(unit.Dp(450))
				}

				constrainedGtx := gtx
				constrainedGtx.Constraints.Max.X = modalMaxWidth
				constrainedGtx.Constraints.Max.Y = modalMaxHeight

				return layout.Stack{}.Layout(constrainedGtx,
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						rect := image.Rectangle{Max: image.Point{X: modalMaxWidth, Y: modalMaxHeight}}
						rrect := clip.UniformRRect(rect, gtx.Dp(10))
						defer rrect.Push(gtx.Ops).Pop()
						paint.Fill(gtx.Ops, AppColors.Background)
						return layout.Dimensions{Size: image.Point{X: modalMaxWidth, Y: modalMaxHeight}}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = modalMaxWidth
						gtx.Constraints.Max.Y = modalMaxHeight
						return widget.Border{
							Color:        AppColors.Accent1Dark,
							Width:        unit.Dp(3),
							CornerRadius: unit.Dp(10),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Point{X: modalMaxWidth, Y: modalMaxHeight}}
						})
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = modalMaxWidth
						gtx.Constraints.Max.Y = modalMaxHeight
						return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
					}),
				)
			})
		}),
	)

	if clickedOutside {
		state.UI.ModalOpen = ""
	}

	return result
}

func ModalDialog(
	gtx layout.Context,
	theme *material.Theme,
	title string,
	options []string,
	buttons []widget.Clickable,
	onSelect func(string),
	state *AppState,
) layout.Dimensions {

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H6(theme, title)
			lbl.Color = AppColors.Title
			lbl.Alignment = text.Middle
			lbl.TextSize = unit.Sp(16)
			return layout.Inset{Bottom: unit.Dp(10), Top: unit.Dp(5)}.Layout(gtx, lbl.Layout)
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Scrollbar parameters
				state.UI.ModalList.Axis = layout.Vertical

				listStyle := material.List(theme, &state.UI.ModalList)

				listStyle.Indicator.MajorMinLen = unit.Dp(40)
				listStyle.Indicator.MinorWidth = unit.Dp(8)
				listStyle.Indicator.Color = AppColors.Accent1
				listStyle.Indicator.HoverColor = AppColors.Accent1Dark
				listStyle.Indicator.CornerRadius = unit.Dp(4)

				return listStyle.Layout(gtx, len(options),
					func(gtx layout.Context, i int) layout.Dimensions {
						if i >= len(buttons) {
							return layout.Dimensions{}
						}
						option := options[i]
						btnWidget := &buttons[i]

						if btnWidget.Clicked(gtx) {
							onSelect(option)
						}

						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(4), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X

							button := material.Button(theme, btnWidget, option)
							button.Background = AppColors.Button
							button.Color = AppColors.Text
							button.Inset = layout.UniformInset(unit.Dp(8))
							button.TextSize = unit.Sp(15)
							button.CornerRadius = unit.Dp(6)

							return button.Layout(gtx)
						})
					})
			})
		}),
	)
}
