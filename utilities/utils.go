package utilities

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
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

// FormatRate converts a string representation of a rate.
func FormatRate(rate string, NeedsRateFormatting bool) (string, error) {
	normalizedRate := strings.Replace(rate, ",", ".", 1)
	floatRate, err := strconv.ParseFloat(normalizedRate, 64)
	if err != nil {
		return rate, fmt.Errorf("FormatRate: error parsing rate '%s': %v", rate, err)
	}

	if NeedsRateFormatting {
		floatRate = floatRate / 100
	}

	return fmt.Sprintf("%.3f", floatRate), nil
}

// DrawProgressBar renders a progress bar based on the loading state in AppState.
func DrawProgressBar(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	elapsed := time.Since(state.IsLoadingStart).Seconds()

	var totalDuration float32 = 5.0
	if state.SelectedCantor != "" {
		for _, c := range state.Cantors {
			if c.ID == state.SelectedCantor {
				totalDuration = float32(c.DefaultTimeout.Seconds())
				break
			}
		}
	}

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
		color.NRGBA{R: 255, G: 255, B: 0, A: alpha},
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

	if entry == nil || state.SelectedCantor == "" {
		return layout.Dimensions{}
	}

	if entry.Error != "" {
		errorMsg := GetTranslation(state.Language, "errorPrefix") + ": " + entry.Error
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

	var buyRateStr, sellRateStr string
	var errBuy, errSell error

	needsDivision := false
	for _, c := range state.Cantors {
		if c.ID == state.SelectedCantor {
			needsDivision = c.NeedsRateFormatting
			break
		}
	}

	buyRateStr, errBuy = FormatRate(entry.Rate.BuyRate, needsDivision)
	sellRateStr, errSell = FormatRate(entry.Rate.SellRate, needsDivision)

	if errBuy != nil || errSell != nil {
		errorMsg := GetTranslation(state.Language, "errorPrefix") + ": " + GetTranslation(state.Language, "invalidRateFormat")
		errorText := material.Body1(theme, errorMsg)
		errorText.Color = AppColors.Error
		errorText.TextSize = unit.Sp(18)
		errorText.Alignment = text.Middle
		return layout.Center.Layout(gtx, errorText.Layout)
	}

	buyRateFloat, errB := strconv.ParseFloat(strings.ReplaceAll(buyRateStr, ",", "."), 64)
	sellRateFloat, errS := strconv.ParseFloat(strings.ReplaceAll(sellRateStr, ",", "."), 64)

	if errB != nil || errS != nil {
		errorMsg := GetTranslation(state.Language, "errorPrefix") + ": " + GetTranslation(state.Language, "internalRateError")
		errorText := material.Body1(theme, errorMsg)
		return layout.Center.Layout(gtx, errorText.Layout)
	}

	buyLabel := GetTranslation(state.Language, "buyLabel")
	sellLabel := GetTranslation(state.Language, "sellLabel")

	rateTextBuy := material.H5(theme, fmt.Sprintf("%s: %.3f %s",
		buyLabel,
		buyRateFloat,
		state.Currency,
	))

	rateTextBuy.Color = AppColors.Success
	rateTextBuy.Alignment = text.Middle

	rateTextSell := material.H5(theme, fmt.Sprintf("%s: %.3f %s",
		sellLabel,
		sellRateFloat,
		state.Currency,
	))

	rateTextSell.Color = AppColors.Error
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

// GUI Elements creation - function
func LayoutUI(gtx layout.Context, theme *material.Theme, state *AppState) {
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(layout.Spacer{Height: unit.Dp(30)}.Layout),
				// Title
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					title := material.H1(theme, GetTranslation(state.Language, "title"))
					title.Alignment = text.Middle
					title.TextSize = unit.Sp(120)
					title.Font.Weight = font.Bold
					title.Color = AppColors.Title
					return title.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				// Info
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					info := material.Body1(theme, GetTranslation(state.Language, "info"))
					info.Alignment = text.Middle
					info.TextSize = unit.Sp(17)
					info.Color = AppColors.Text
					info.Font.Weight = font.Normal
					if state.Language == "UA" || state.Language == "BU" {
						info.Font.Weight = font.SemiBold
					}
					return info.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(15)}.Layout),

				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis:      layout.Horizontal,
						Spacing:   layout.SpaceAround, // Zachowaj dla odstępów między przyciskami
						Alignment: layout.Middle,
					}.Layout(gtx,
						// Przycisk Języka
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Zamiast Flexed
							// Opcjonalnie: Ogranicz maksymalną szerokość pojedynczego przycisku
							// buttonMaxWidth := gtx.Dp(unit.Dp(120)) // np. maksymalnie 120 Dp szerokości
							// if gtx.Constraints.Max.X > buttonMaxWidth {
							// 	gtx.Constraints.Max.X = buttonMaxWidth
							// }

							btn := material.Button(theme, &state.LangModalButton, state.Language)
							btn.Color = AppColors.Accent1      // Zmieniono na Accent1 dla żółtego napisu
							btn.Background = color.NRGBA{A: 0} // Przezroczyste tło
							btn.CornerRadius = unit.Dp(6)
							btn.TextSize = unit.Sp(16) // Dostosuj rozmiar tekstu
							// Zmniejszony padding, aby przycisk był węższy
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}
							if state.LangModalButton.Clicked(gtx) {
								state.ModalOpen = "language"
							}
							return btn.Layout(gtx)
						}),

						// Przycisk Waluty
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { // Zamiast Flexed
							// Opcjonalnie: Ogranicz maksymalną szerokość pojedynczego przycisku
							// buttonMaxWidth := gtx.Dp(unit.Dp(120))
							// if gtx.Constraints.Max.X > buttonMaxWidth {
							// 	gtx.Constraints.Max.X = buttonMaxWidth
							// }

							btn := material.Button(theme, &state.CurrencyModalButton, state.Currency)
							btn.Color = AppColors.Accent1      // Żółty napis
							btn.Background = color.NRGBA{A: 0} // Przezroczyste tło
							btn.CornerRadius = unit.Dp(6)
							btn.TextSize = unit.Sp(16)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}
							if state.CurrencyModalButton.Clicked(gtx) {
								state.ModalOpen = "currency"
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

				return material.List(theme, &list).Layout(gtx, len(state.Cantors),
					func(gtx layout.Context, i int) layout.Dimensions {
						cantor := state.Cantors[i]
						displayName := GetTranslation(state.Language, cantor.Displayname)
						if displayName == cantor.Displayname {
							displayName = cantor.ID
						}

						return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								button := material.Button(theme, &cantor.Button, displayName)
								button.Background = color.NRGBA{R: 25, G: 25, B: 25, A: 50}
								button.Color = AppColors.Text
								button.TextSize = unit.Sp(16)
								button.Inset = layout.UniformInset(unit.Dp(12))
								button.CornerRadius = unit.Dp(8)

								if state.SelectedCantor == cantor.ID {
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

		// Sekcja Paska Postępu (jeśli ładuje)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.IsLoading.Load() && state.SelectedCantor != "" {
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

	if state.ModalOpen != "" {
		var modalContent layout.Widget
		switch state.ModalOpen {
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

// LanguageModal and CurrencyModal are the modal dialogs for selecting language and currency respectively.
func LanguageModal(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.Language, "↓")
	if lbl := GetTranslation(state.Language, "selectLanguageTitle"); lbl != "selectLanguageTitle" {
		title = lbl
	}
	return ModalOverlay(gtx, theme, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			title,
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
	title := GetTranslation(state.Currency, "↓")
	if lbl := GetTranslation(state.Currency, "selectCurrencyTitle"); lbl != "selectCurrencyTitle" {
		title = lbl
	}
	return ModalOverlay(gtx, theme, state, func(gtx layout.Context) layout.Dimensions {
		return ModalDialog(
			gtx,
			theme,
			title,
			state.CurrencyOptions,
			state.CurrencyOptionButtons,
			func(currency string) {
				state.Currency = currency
				state.ModalOpen = ""
			},
		)
	})
}

// Draws the modal overlay
func ModalOverlay(gtx layout.Context, theme *material.Theme, state *AppState, content layout.Widget) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{A: 210})

	if state.ModalClick.Clicked(gtx) {
		state.ModalOpen = ""
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return state.ModalClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			modalMaxWidth := gtx.Dp(unit.Dp(300))
			modalMaxHeight := gtx.Dp(unit.Dp(300))

			availableWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(40))
			availableHeight := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(80))

			if modalMaxWidth > availableWidth {
				modalMaxWidth = availableWidth
			}
			if modalMaxHeight > availableHeight {
				modalMaxHeight = availableHeight
			}

			// Ustaw ograniczenia dla wewnętrznego kontenera modala
			constrainedGtx := gtx
			constrainedGtx.Constraints.Max.X = modalMaxWidth
			constrainedGtx.Constraints.Max.Y = modalMaxHeight

			return widget.Border{
				Color:        AppColors.Accent1,
				Width:        unit.Dp(1.5),
				CornerRadius: unit.Dp(10),
			}.Layout(constrainedGtx, func(gtx layout.Context) layout.Dimensions {
				max := gtx.Constraints.Max
				rrect := clip.UniformRRect(image.Rectangle{Max: max}, gtx.Dp(10-1.5))
				defer rrect.Push(gtx.Ops).Pop()
				paint.Fill(gtx.Ops, AppColors.Background)

				return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
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

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		// Tytuł Modala
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H6(theme, title)
			lbl.Color = AppColors.Title
			lbl.Alignment = text.Middle
			lbl.TextSize = unit.Sp(17)
			return layout.Inset{Bottom: unit.Dp(10), Top: unit.Dp(5)}.Layout(gtx, lbl.Layout)
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			list := widget.List{}
			list.Axis = layout.Vertical

			return material.List(theme, &list).Layout(gtx, len(options),
				func(gtx layout.Context, i int) layout.Dimensions {
					if i >= len(buttons) {
						return layout.Dimensions{}
					}
					option := options[i]
					btnWidget := &buttons[i]

					if btnWidget.Clicked(gtx) {
						onSelect(option)
					}

					button := material.Button(theme, btnWidget, option)
					button.Background = AppColors.Button
					button.Color = AppColors.Text
					button.Inset = layout.UniformInset(unit.Dp(8))
					button.TextSize = unit.Sp(15)
					button.CornerRadius = unit.Dp(6)

					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, button.Layout)
				})
		}),
	)
}
