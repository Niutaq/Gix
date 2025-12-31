package utilities

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	//"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// Gix utilities
	"github.com/Niutaq/Gix/pkg/reading_data"
)

// Global variables
//
////go:embed res/background_2k.png
//var backgroundImageFSF embed.FS - disabled for now

var (
	modalCloseBtn widget.Clickable
)

// LayoutUI - Main application layout
func LayoutUI(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) {
	state.UI.SearchEditor.SingleLine = true
	state.UI.SearchEditor.Submit = true

	if state.Vault.Rates == nil {
		state.Vault.Rates = make(map[string]*CantorEntry)
	}

	paint.Fill(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 35, A: 255})

	layout.Stack{Alignment: layout.NW}.Layout(gtx,
		// LAYER 1: Application Content
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				// LEFT: Currency bar
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return LayoutVerticalCurrencyBar(gtx, window, theme, state, config)
				}),
				// CENTER: Dashboard
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Left:  unit.Dp(24),
						Right: unit.Dp(24),
						Top:   unit.Dp(16),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutHeader(gtx, window, theme, state)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return LayoutSearchBar(gtx, theme, state)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layoutCantorSelection(gtx, window, theme, state)
							}),
						)
					})
				}),
				// RIGHT: Analysis Panel
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return LayoutRightPanel(gtx, theme, state)
				}),
			)
		}),

		// LAYER 2: Loading Bar
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutLoadingBar(gtx, window, state)
			})
		}),

		// LAYER 3: Modal
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if state.UI.ModalOpen != "" {
				return layoutModal(gtx, window, theme, state)
			}
			return layout.Dimensions{}
		}),

		//// LAYER 4: Notifications
		//layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		//	return LayoutNotification(gtx, theme, state)
		//}), - disabled for now
	)
}

// fetchAllRates retrieves exchange rates for all cantors and updates the application state asynchronously.
func fetchAllRates(window *app.Window, state *AppState, config AppConfig) {
	if config.APIRatesURL == "" {
		return
	}

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	state.Vault.Mu.Lock()
	state.Vault.Rates = make(map[string]*CantorEntry)
	state.Vault.Mu.Unlock()

	currency := state.UI.Currency

	sem := make(chan struct{}, 8)

	go func() {
		for id, cantor := range state.Cantors {
			sem <- struct{}{}

			cantorID := cantor.ID
			cantorKey := id

			go func(cID int, cKey string) {
				defer func() { <-sem }()

				url := fmt.Sprintf("%s?cantor_id=%d&currency=%s", config.APIRatesURL, cID, currency)
				client := http.Client{Timeout: 5 * time.Second}
				resp, err := client.Get(url)

				var entry *CantorEntry

				if err != nil {
					entry = &CantorEntry{Error: "err_api_connection", LoadedAt: time.Now()}

					//if state.Notifications == nil {
					//	ShowToast(state, window, GetTranslation(state.UI.Language, "err_api_connection"), "error")
					//} - disabled for now
				} else {
					defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
					if resp.StatusCode == http.StatusOK {
						var rates ExchangeRates
						if err := json.NewDecoder(resp.Body).Decode(&rates); err == nil {
							entry = &CantorEntry{Rate: rates, LoadedAt: time.Now()}
						} else {
							entry = &CantorEntry{Error: "err_api_parsing", LoadedAt: time.Now()}

							//if state.Notifications == nil {
							//	ShowToast(state, window, GetTranslation(state.UI.Language, "err_api_parsing"), "error")
							//} - disabled for now
						}
					} else {
						entry = &CantorEntry{Error: "err_api_response", LoadedAt: time.Now()}

						//if state.Notifications == nil {
						//	ShowToast(state, window, GetTranslation(state.UI.Language, "err_api_response"), "error")
						//} - disabled for now
					}
				}

				state.Vault.Mu.Lock()
				state.Vault.Rates[cKey] = entry
				state.Vault.Mu.Unlock()

				window.Invalidate()
			}(cantorID, cantorKey)
		}

		// Wait loop
		for {
			if len(sem) == 0 {
				state.IsLoading.Store(false)
				window.Invalidate()
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

// LayoutVerticalCurrencyBar creates a vertical sidebar for currency selection with a given layout, window, theme, state, and config.
func LayoutVerticalCurrencyBar(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	sidebarWidth := gtx.Dp(unit.Dp(70))
	gtx.Constraints.Min.X = sidebarWidth
	gtx.Constraints.Max.X = sidebarWidth
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y

	paint.FillShape(gtx.Ops, color.NRGBA{R: 12, G: 12, B: 18, A: 240}, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(15), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(theme, GetTranslation(state.UI.Language, "sidebar_currency_label"))
					lbl.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
					lbl.TextSize = unit.Sp(10)
					return lbl.Layout(gtx)
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				list := &state.UI.CurrencyList
				list.Axis = layout.Vertical
				return material.List(theme, list).Layout(gtx, len(state.UI.CurrencyOptions),
					func(gtx layout.Context, i int) layout.Dimensions {
						currency := state.UI.CurrencyOptions[i]
						btn := &state.UI.CurrencyOptionButtons[i]
						isSelected := state.UI.Currency == currency

						if btn.Clicked(gtx) {
							state.UI.Currency = currency
							fetchAllRates(window, state, config)
							window.Invalidate()
						}

						return layout.Inset{Bottom: unit.Dp(8), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(
							gtx, func(gtx layout.Context) layout.Dimensions {
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									bgColor := color.NRGBA{A: 0}
									txtColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
									if isSelected {
										bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: 10}
										txtColor = AppColors.Accent1
									} else if btn.Hovered() {
										bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: 5}
									}
									return layout.Stack{}.Layout(gtx,
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
											paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
											return layout.Dimensions{Size: gtx.Constraints.Min}
										}),
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														lbl := material.Body2(theme, currency)
														lbl.Color = txtColor
														lbl.Font.Weight = font.Bold
														return lbl.Layout(gtx)
													}),
												)
											})
										}),
									)
								})
							})
					})
			})
		}),
	)
}

// parseRate parses a string containing a numeric value, replaces commas with dots, trims spaces, and converts it to float64.
func parseRate(s string) float64 {
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " zł", "")
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}
	return f
}

// layoutCantorSelection creates and displays a filtered list of cantors, with Best Buy/sell rate highlights, based on search input.
func layoutCantorSelection(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {
	list := &state.UI.CantorsList
	list.Axis = layout.Vertical

	searchText := strings.ToLower(state.UI.SearchText)
	var filteredIDs []string

	var bestBuyRate = -1.0
	var bestSellRate = 999999.0

	state.Vault.Mu.Lock()
	for _, entry := range state.Vault.Rates {
		if entry != nil && entry.Rate.BuyRate != "" {
			buy := parseRate(entry.Rate.BuyRate)
			sell := parseRate(entry.Rate.SellRate)
			if buy > bestBuyRate {
				bestBuyRate = buy
			}
			if sell > 0 && sell < bestSellRate {
				bestSellRate = sell
			}
		}
	}

	for id, cantor := range state.Cantors {
		displayName := GetTranslation(state.UI.Language, cantor.DisplayName)
		if searchText == "" || strings.Contains(strings.ToLower(id), searchText) ||
			strings.Contains(strings.ToLower(displayName), searchText) {
			filteredIDs = append(filteredIDs, id)
		}
	}
	state.Vault.Mu.Unlock()
	sort.Strings(filteredIDs)

	if len(filteredIDs) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				msg := material.Body1(theme, GetTranslation(state.UI.Language, "no_cantor_found"))
				msg.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
				return msg.Layout(gtx)
			})
		})
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(theme, list).Layout(gtx, len(filteredIDs),
			func(gtx layout.Context, i int) layout.Dimensions {
				return layoutCantorItem(gtx, window, theme, state, filteredIDs, i, bestBuyRate, bestSellRate)
			})
	})
}

// layoutCantorItem - Renders a single cantor row with name and rates
func layoutCantorItem(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState,
	cantorIDs []string, i int, bestBuy, bestSell float64) layout.Dimensions {
	cantorKey := cantorIDs[i]
	cantor := state.Cantors[cantorKey]
	displayName := GetTranslation(state.UI.Language, cantor.DisplayName)

	state.Vault.Mu.Lock()
	entry := state.Vault.Rates[cantorKey]
	state.Vault.Mu.Unlock()

	buyVal := "---"
	sellVal := "---"

	baseBuyColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	baseSellColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}

	var alpha uint8 = 255

	if entry != nil {
		if !entry.LoadedAt.IsZero() {
			const animDuration = 600 * time.Millisecond
			elapsed := time.Since(entry.LoadedAt)

			if elapsed < animDuration {
				progress := float32(elapsed) / float32(animDuration)
				progress = 1.0 - (1.0-progress)*(1.0-progress)
				alpha = uint8(255 * progress)

				window.Invalidate()
			}
		}

		if entry.Error != "" {
			noRateText := GetTranslation(state.UI.Language, "no_rate_label")
			buyVal = noRateText
			sellVal = noRateText
			baseBuyColor = AppColors.Error
			baseSellColor = AppColors.Error
		} else {
			buyVal = entry.Rate.BuyRate + " zł"
			sellVal = entry.Rate.SellRate + " zł"

			currentBuy := parseRate(entry.Rate.BuyRate)
			currentSell := parseRate(entry.Rate.SellRate)

			if currentBuy >= bestBuy && bestBuy > 0 {
				baseBuyColor = AppColors.Accent1
			} else {
				baseBuyColor = AppColors.Text
			}
			if currentSell <= bestSell && bestSell > 0 {
				baseSellColor = AppColors.Accent1
			} else {
				baseSellColor = AppColors.Text
			}
		}
	}

	buyColor := baseBuyColor
	buyColor.A = alpha
	sellColor := baseSellColor
	sellColor.A = alpha

	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return cantor.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bgColor := color.NRGBA{R: 30, G: 30, B: 35, A: 150}
			if cantor.Button.Hovered() {
				bgColor = color.NRGBA{R: 40, G: 40, B: 45, A: 200}
			}

			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
					paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal,
							Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(theme, displayName)
								lbl.Color = AppColors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := GetTranslation(state.UI.Language, "buy_col")
										return layoutMarketValue(gtx, theme, label, buyVal, buyColor)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(30)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := GetTranslation(state.UI.Language, "sell_col")
										return layoutMarketValue(gtx, theme, label, sellVal, sellColor)
									}),
								)
							}),
						)
					})
				}),
			)
		})
	})
}

// layoutHeader renders a header layout with a title, subtitle, and a language selection button, styled using the provided theme.
func layoutHeader(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						h2 := material.H5(theme, GetTranslation(state.UI.Language, "market_title"))
						h2.Color = AppColors.Text
						h2.Font.Weight = font.Bold
						return h2.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						caption := material.Caption(theme, GetTranslation(state.UI.Language, "market_subtitle"))
						caption.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
						return caption.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutLanguageButton(gtx, window, theme, state)
			}),
		)
	})
}

// layoutLanguageButton handles the layout and rendering of the language selection button within the application's UI.
func layoutLanguageButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.LangModalButton.Clicked(gtx) {
		state.UI.ModalOpen = "language"
		window.Invalidate()
	}
	btn := material.Button(theme, &state.UI.LangModalButton, state.UI.Language)
	btn.Color = AppColors.Accent1
	btn.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 10}
	btn.CornerRadius = unit.Dp(8)
	btn.TextSize = unit.Sp(14)
	btn.Inset = layout.UniformInset(unit.Dp(10))
	return btn.Layout(gtx)
}

// LayoutSearchBar renders the search bar within a specified layout context using the provided UI state and theme.
func LayoutSearchBar(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	state.UI.SearchText = state.UI.SearchEditor.Text()
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				bgColor := color.NRGBA{R: 25, G: 25, B: 30, A: 200}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(14))
				paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
				borderColor := color.NRGBA{R: 255, G: 255, B: 255, A: 20}
				if len(state.UI.SearchText) > 0 {
					borderColor = AppColors.Accent1
					borderColor.A = 150
				}
				return widget.Border{Color: borderColor, Width: unit.Dp(1), CornerRadius: unit.Dp(14)}.Layout(
					gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					})
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					hint := GetTranslation(state.UI.Language, "search_placeholder")
					ed := material.Editor(theme, &state.UI.SearchEditor, hint)
					ed.Color = AppColors.Text
					ed.HintColor = color.NRGBA{R: 120, G: 120, B: 130, A: 255}
					ed.TextSize = unit.Sp(16)
					return ed.Layout(gtx)
				})
			}),
		)
	})
}

// LayoutRightPanel lays out the right-side panel of the application, featuring titles, subtitles, charts, and descriptive text.
func LayoutRightPanel(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	panelWidth := gtx.Dp(unit.Dp(320))
	gtx.Constraints.Min.X = panelWidth
	gtx.Constraints.Max.X = panelWidth
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, color.NRGBA{R: 18, G: 18, B: 22, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Inset{Top: unit.Dp(20), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(
		gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					h := material.H6(theme, GetTranslation(state.UI.Language, "ai_title"))
					h.Color = AppColors.Accent1
					h.Font.Weight = font.Bold
					return h.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(5)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					txt := material.Caption(theme, GetTranslation(state.UI.Language, "ai_subtitle"))
					txt.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
					return txt.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(30)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(200))
					return layout.Stack{}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							shape := clip.UniformRRect(image.Rectangle{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}, 5)
							paint.FillShape(gtx.Ops, color.NRGBA{R: 40, G: 20, B: 20, A: 255}, shape.Op(gtx.Ops))
							return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: height}}
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								msg := material.Body2(theme, GetTranslation(state.UI.Language, "ai_chart_placeholder"))
								msg.Color = AppColors.Error
								return layout.Inset{Top: unit.Dp(80)}.Layout(gtx, msg.Layout)
							})
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					txt := material.Body2(theme, GetTranslation(state.UI.Language, "ai_desc"))
					txt.Color = color.NRGBA{R: 80, G: 80, B: 90, A: 255}
					return txt.Layout(gtx)
				}),
			)
		})
}

// layoutMarketValue lays out a market value displaying a label and its corresponding value with customizable text color.
func layoutMarketValue(
	gtx layout.Context, theme *material.Theme,
	label, value string, txtColor color.NRGBA) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(theme, label)
			l.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
			l.TextSize = unit.Sp(10)
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			v := material.Body2(theme, value)
			v.Color = txtColor
			return v.Layout(gtx)
		}),
	)
}

// layoutModal renders a modal overlay based on the application's current modal state and selected modal type.
func layoutModal(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.ModalOpen == "" {
		return layout.Dimensions{}
	}
	return ModalOverlay(gtx, window, state, func(gtx layout.Context) layout.Dimensions {
		if state.UI.ModalOpen == "language" {
			return LanguageModal(gtx, window, theme, state)
		}
		return layout.Dimensions{}
	})
}

// ModalOverlay renders a centered modal overlay with a semi-transparent background and specified content widget.
func ModalOverlay(
	gtx layout.Context, window *app.Window,
	state *AppState, content layout.Widget) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{A: 210})
	if state.UI.ModalClick.Clicked(gtx) {
		state.UI.ModalOpen = ""
		window.Invalidate()
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return state.UI.ModalClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				modalMaxWidth := gtx.Dp(unit.Dp(350))
				modalMaxHeight := gtx.Dp(unit.Dp(500))
				gtx.Constraints.Max.X = modalMaxWidth
				gtx.Constraints.Max.Y = modalMaxHeight
				return layout.Stack{}.Layout(gtx,
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						rect := image.Rectangle{Max: gtx.Constraints.Max}
						rRect := clip.UniformRRect(rect, gtx.Dp(10))
						paint.FillShape(gtx.Ops, AppColors.Background, rRect.Op(gtx.Ops))
						return layout.Dimensions{Size: gtx.Constraints.Max}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return widget.Border{Color: AppColors.Accent1Dark,
							Width: unit.Dp(2), CornerRadius: unit.Dp(10)}.Layout(
							gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{Size: gtx.Constraints.Max}
							})
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
					}),
				)
			})
		}),
	)
}

// LanguageModal renders a modal dialog for selecting a language and updates the application state upon selection.
func LanguageModal(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.UI.Language, "select_lang_title")
	return ModalDialog(gtx, window, theme, title,
		state.UI.LanguageOptions, state.UI.LanguageOptionButtons, func(lang string) {
			state.UI.Language = lang
			state.UI.ModalOpen = ""
			window.Invalidate()
		}, state)
}

// ModalDialog renders a modal dialog with a title, list of options, and action buttons, and executes a callback on selection.
func ModalDialog(
	gtx layout.Context, window *app.Window,
	theme *material.Theme, title string, options []string,
	buttons []widget.Clickable, onSelect func(string), state *AppState) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					h6 := material.H6(theme, title)
					h6.Color = AppColors.Title
					return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(16), Bottom: unit.Dp(10)}.Layout(gtx, h6.Layout)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if modalCloseBtn.Clicked(gtx) {
						state.UI.ModalOpen = ""
						window.Invalidate()
					}
					btn := material.Button(theme, &modalCloseBtn, "x")
					btn.Background = color.NRGBA{A: 0}
					btn.Color = AppColors.Accent3
					btn.Inset = layout.UniformInset(unit.Dp(12))
					btn.TextSize = unit.Sp(18)
					return layout.Inset{Right: unit.Dp(8), Top: unit.Dp(8)}.Layout(gtx, btn.Layout)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			state.UI.ModalList.Axis = layout.Vertical
			cols := 3
			rows := (len(options) + cols - 1) / cols
			return material.List(theme, &state.UI.ModalList).Layout(
				gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						func() []layout.FlexChild {
							children := make([]layout.FlexChild, cols)
							for c := 0; c < cols; c++ {
								idx := row*cols + c
								if idx < len(options) {
									children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(4)).Layout(
											gtx, func(gtx layout.Context) layout.Dimensions {
												if buttons[idx].Clicked(gtx) {
													onSelect(options[idx])
												}
												btn := material.Button(theme, &buttons[idx], options[idx])
												btn.Background = AppColors.Button
												btn.Color = AppColors.Text
												btn.Inset = layout.UniformInset(unit.Dp(10))
												gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(50))
												return btn.Layout(gtx)
											})
									})
								} else {
									children[c] = layout.Flexed(1, func(
										gtx layout.Context) layout.Dimensions {
										return layout.Dimensions{}
									})
								}
							}
							return children
						}()...,
					)
				})
		}),
	)
}

//// ShowToast displays a notification with a message and type that expires after 2 seconds, then triggers a UI update.
//func ShowToast(state *AppState, window *app.Window, message string, notifType string) {
//	state.Notifications = &Notification{
//		Message: message,
//		Type:    notifType,
//		Timeout: time.Now().Add(2 * time.Second),
//	}
//	go func() {
//		time.Sleep(2100 * time.Millisecond)
//		window.Invalidate()
//	}()
//}
//
//// LayoutNotification renders a notification banner if present, or clears it if expired, and adjusts its appearance based on type.
//func LayoutNotification(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
//	if state.Notifications == nil {
//		return layout.Dimensions{}
//	}
//	if time.Now().After(state.Notifications.Timeout) {
//		state.Notifications = nil
//		return layout.Dimensions{}
//	}
//	notif := state.Notifications
//
//	bgColor := color.NRGBA{R: 50, G: 50, B: 50, A: 240}
//
//	if notif.Type == "error" {
//		bgColor = color.NRGBA{R: 200, G: 40, B: 40, A: 220}
//	} else if notif.Type == "success" {
//		bgColor = color.NRGBA{R: 40, G: 180, B: 40, A: 220}
//	}
//
//	return layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
//		return layout.Inset{Top: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
//			macro := op.Record(gtx.Ops)
//			dims := layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
//				msg := material.Body1(theme, notif.Message)
//				msg.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
//				return msg.Layout(gtx)
//			})
//			call := macro.Stop()
//			paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(8)).Op(gtx.Ops))
//			call.Add(gtx.Ops)
//			return dims
//		})
//	})
//} - disabled for now

// DrawProgressBar draws a loading progress bar based on elapsed time relative to a fixed total duration.
func DrawProgressBar(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	elapsed := time.Since(state.IsLoadingStart).Seconds()
	var totalDuration float32 = 1.0

	progress := float32(elapsed) / totalDuration
	if progress > 1 {
		progress = 1
	} else {
		window.Invalidate()
	}

	barHeight := gtx.Dp(unit.Dp(2))
	barWidth := gtx.Constraints.Max.X

	fillWidth := int(float32(barWidth) * progress)
	fillRect := image.Rect(0, 0, fillWidth, barHeight)

	fillColor := AppColors.Accent1
	fillColor.A = 150

	paint.FillShape(gtx.Ops, fillColor, clip.Rect(fillRect).Op())

	return layout.Dimensions{Size: image.Point{X: barWidth, Y: barHeight}}
}

// layoutLoadingBar displays a loading bar if the application's loading state is active.
func layoutLoadingBar(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if !state.IsLoading.Load() {
		return layout.Dimensions{}
	}
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return DrawProgressBar(gtx, window, state)
	})
}

// LoadFontCollection loads and returns a collection of font faces. Returns an error if the loading or parsing fails.
func LoadFontCollection() ([]font.FontFace, error) {
	face, _ := reading_data.LoadAndParseFont("fonts/NotoSans-Regular.ttf")
	return []font.FontFace{face}, nil
}
