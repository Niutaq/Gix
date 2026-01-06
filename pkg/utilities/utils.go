package utilities

import (
	// Standard libraries
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
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// Gix utilities
	"github.com/Niutaq/Gix/pkg/reading_data"
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"google.golang.org/protobuf/proto"
)

// Global variables
var (
	modalCloseBtn widget.Clickable
)

// LayoutUI - Main application layout
func LayoutUI(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) {
	updateNotchAnimation(window, state)

	state.UI.SearchEditor.SingleLine = true
	state.UI.SearchEditor.Submit = true

	if state.Vault.Rates == nil {
		state.Vault.Rates = make(map[string]*CantorEntry)
	}

	drawPatternBackground(gtx)

	paint.Fill(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 35, A: 50})

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
								return LayoutSearchBar(gtx, window, theme, state)
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

		// LAYER 4: Dynamic Notch (Aesthetic Info)
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutNotch(gtx, theme, state)
		}),
	)
}

// updateNotchAnimation handles the fade-in/out logic for the dynamic notch.
func updateNotchAnimation(window *app.Window, state *AppState) {
	now := time.Now()

	if state.UI.NotchState.LastTime.IsZero() {
		state.UI.NotchState.LastTime = now
	}
	dt := now.Sub(state.UI.NotchState.LastTime).Seconds()
	if dt > 0.05 {
		dt = 0.05
	}
	state.UI.NotchState.LastTime = now

	if state.UI.HoverInfo.Active {
		state.UI.NotchState.LastHoverTime = now
	}

	shouldShow := state.UI.HoverInfo.Active || now.Sub(state.UI.NotchState.LastHoverTime) < 500*time.Millisecond

	targetAlpha := float32(0.0)
	if shouldShow {
		targetAlpha = 1.0
		if state.UI.HoverInfo.Active {
			state.UI.NotchState.LastContent = state.UI.HoverInfo
		}
	}

	speed := float32(8.0)
	change := speed * float32(dt)
	state.UI.NotchState.CurrentAlpha = moveTowards(state.UI.NotchState.CurrentAlpha, targetAlpha, change)

	if state.UI.NotchState.CurrentAlpha > 0.01 || shouldShow {
		window.Invalidate()
	}

	state.UI.HoverInfo = HoverInfo{Active: false}
}

// moveTowards moves current value towards target by at most maxDelta.
func moveTowards(current, target, maxDelta float32) float32 {
	if diff := target - current; diff < 0 {
		diff = -diff
		if diff <= maxDelta {
			return target
		}
		return current - maxDelta
	} else {
		if diff <= maxDelta {
			return target
		}
		return current + maxDelta
	}
}

// drawPatternBackground draws a procedural geometric background.
func drawPatternBackground(gtx layout.Context) {
	paint.Fill(gtx.Ops, color.NRGBA{R: 15, G: 15, B: 25, A: 255})
}

// fetchAllRates retrieves exchange rates for all cantors asynchronously
func fetchAllRates(window *app.Window, state *AppState, config AppConfig) {
	if config.APIRatesURL == "" {
		return
	}

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	state.Vault.Mu.Lock()
	state.Vault.Rates = make(map[string]*CantorEntry)
	state.Vault.Mu.Unlock()

	sem := make(chan struct{}, 8)

	go runFetchLoop(window, state, config, sem)
}

// runFetchLoop concurrently fetches exchange rates for all cantors and updates the application state accordingly.
func runFetchLoop(window *app.Window, state *AppState, config AppConfig, sem chan struct{}) {
	currency := state.UI.Currency

	for id, cantor := range state.Cantors {
		sem <- struct{}{}
		go fetchSingleCantor(window, state, config, sem, cantor.ID, id, currency)
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
}

// fetchSingleCantor fetches exchange rates from a specific cantor, updates the application state, and invalidates the window.
func fetchSingleCantor(window *app.Window, state *AppState, config AppConfig, sem chan struct{}, cID int, cKey, currency string) {
	defer func() { <-sem }()

	url := fmt.Sprintf("%s?cantor_id=%d&currency=%s", config.APIRatesURL, cID, currency)
	client := http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		state.Vault.Mu.Lock()
		state.Vault.Rates[cKey] = &CantorEntry{Error: "err_internal", LoadedAt: time.Now()}
		state.Vault.Mu.Unlock()
		return
	}
	req.Header.Set("Accept", "application/x-protobuf")

	resp, err := client.Do(req)

	entry := handleCantorResponse(resp, err)

	state.Vault.Mu.Lock()
	state.Vault.Rates[cKey] = entry
	state.Vault.Mu.Unlock()

	window.Invalidate()
}

// handleCantorResponse processes the HTTP response and error to construct a CantorEntry with exchange rates or error details.
func handleCantorResponse(resp *http.Response, err error) *CantorEntry {
	if err != nil {
		return &CantorEntry{Error: "err_api_connection", LoadedAt: time.Now()}
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &CantorEntry{Error: "err_api_response", LoadedAt: time.Now()}
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-protobuf") {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return &CantorEntry{Error: "err_api_parsing", LoadedAt: time.Now()}
		}
		var pbRate pb.RateResponse
		if err := proto.Unmarshal(bodyBytes, &pbRate); err != nil {
			return &CantorEntry{Error: "err_api_parsing", LoadedAt: time.Now()}
		}
		return &CantorEntry{
			Rate: ExchangeRates{
				BuyRate:  pbRate.BuyRate,
				SellRate: pbRate.SellRate,
			},
			LoadedAt: time.Now(),
		}
	}

	var rates ExchangeRates
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		return &CantorEntry{Error: "err_api_parsing", LoadedAt: time.Now()}
	}

	return &CantorEntry{Rate: rates, LoadedAt: time.Now()}
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
func layoutCantorSelection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	state.Vault.Mu.Lock()
	bestBuy, bestSell := calculateBestRates(state.Vault.Rates)
	filteredIDs := filterCantorList(state, state.UI.SearchText)
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

	list := &state.UI.CantorsList
	list.Axis = layout.Vertical

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(theme, list).Layout(gtx, len(filteredIDs),
			func(gtx layout.Context, i int) layout.Dimensions {
				rowCfg := CantorRowConfig{
					CantorID: filteredIDs[i],
					BestBuy:  bestBuy,
					BestSell: bestSell}
				return layoutCantorItem(gtx, window, theme, state, rowCfg)
			})
	})
}

// calculateBestRates determines the highest buy rate and lowest sell rate from the provided exchange rates map.
func calculateBestRates(rates map[string]*CantorEntry) (float64, float64) {
	bestBuy := -1.0
	bestSell := 999999.0

	for _, entry := range rates {
		if entry != nil && entry.Rate.BuyRate != "" {
			buy := parseRate(entry.Rate.BuyRate)
			sell := parseRate(entry.Rate.SellRate)
			if buy > bestBuy {
				bestBuy = buy
			}
			if sell > 0 && sell < bestSell {
				bestSell = sell
			}
		}
	}
	return bestBuy, bestSell
}

// filterCantorList filters and returns a list of cantor IDs from the state based on a case-insensitive search query.
func filterCantorList(state *AppState, searchText string) []string {
	searchText = strings.ToLower(searchText)
	var ids []string
	for id, cantor := range state.Cantors {
		if state.UI.UserLocation.Active && state.UI.MaxDistance > 0 {
			dist := CalculateDistance(
				state.UI.UserLocation.Latitude,
				state.UI.UserLocation.Longitude,
				cantor.Latitude,
				cantor.Longitude,
			)
			if dist > state.UI.MaxDistance {
				continue
			}
		}

		displayName := GetTranslation(state.UI.Language, cantor.DisplayName)
		if searchText == "" ||
			strings.Contains(strings.ToLower(id), searchText) ||
			strings.Contains(strings.ToLower(displayName), searchText) {
			ids = append(ids, id)
		}
	}
	return ids
}

// layoutCantorItem lays out a single Cantor item, displaying its name, buy/sell rates, and applying animations and styles.
func layoutCantorItem(
	gtx layout.Context, window *app.Window, theme *material.Theme,
	state *AppState, cfg CantorRowConfig) layout.Dimensions {
	cantorKey := cfg.CantorID
	cantor := state.Cantors[cantorKey]
	displayName := GetTranslation(state.UI.Language, cantor.DisplayName)

	state.Vault.Mu.Lock()
	entry := state.Vault.Rates[cantorKey]
	state.Vault.Mu.Unlock()

	alpha := getAnimationAlpha(window, entry)

	buyVal, sellVal, buyColor, sellColor := getCantorDisplayData(state, entry, cfg.BestBuy, cfg.BestSell)

	buyColor.A = alpha
	sellColor.A = alpha

	if cantor.Button.Hovered() {
		address := cantor.Address
		if address == "" {
			switch strings.ToLower(cantorKey) {
			case "supersam":
				address = "Adama Asnyka 12, 35-001 Rzeszów"
			case "tadek":
				address = "Gen. Okulickiego 1b, 37-450 Stalowa Wola"
			case "exchange":
				address = "Grottgera 20, 35-001 Rzeszów"
			default:
				address = GetTranslation(state.UI.Language, "location_unknown")
			}
		}

		distance := ""
		if state.UI.UserLocation.Active {
			dist := CalculateDistance(
				state.UI.UserLocation.Latitude,
				state.UI.UserLocation.Longitude,
				cantor.Latitude,
				cantor.Longitude,
			)
			distance = fmt.Sprintf("%.1f km", dist)
		}

		state.UI.HoverInfo = HoverInfo{
			Active:   true,
			Title:    displayName,
			Subtitle: address,
			Extra:    distance,
		}
		window.Invalidate()
	}

	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return cantor.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bgColor := color.NRGBA{R: 30, G: 30, B: 35, A: 150}

			if cantor.Button.Hovered() {
				bgColor = color.NRGBA{R: 45, G: 45, B: 55, A: 200}
			}

			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
					paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
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

// getAnimationAlpha calculates the alpha transparency for a smooth fade-in animation based on the entry's load time.
func getAnimationAlpha(window *app.Window, entry *CantorEntry) uint8 {
	if entry == nil || entry.LoadedAt.IsZero() {
		return 255
	}
	const animDuration = 600 * time.Millisecond
	elapsed := time.Since(entry.LoadedAt)

	if elapsed < animDuration {
		progress := float32(elapsed) / float32(animDuration)
		progress = 1.0 - (1.0-progress)*(1.0-progress) // Ease-out
		window.Invalidate()
		return uint8(255 * progress)
	}
	return 255
}

// getCantorDisplayData generates formatted buy/sell rates and corresponding colors based on the current state and thresholds.
func getCantorDisplayData(
	state *AppState, entry *CantorEntry, bestBuy, bestSell float64) (string, string, color.NRGBA, color.NRGBA) {
	defColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	if entry == nil {
		return "---", "---", defColor, defColor
	}

	if entry.Error != "" {
		errTxt := GetTranslation(state.UI.Language, "no_rate_label")
		return errTxt, errTxt, AppColors.Error, AppColors.Error
	}

	buyVal := entry.Rate.BuyRate + " zł"
	sellVal := entry.Rate.SellRate + " zł"
	buyColor := AppColors.Text
	sellColor := AppColors.Text

	currentBuy := parseRate(entry.Rate.BuyRate)
	currentSell := parseRate(entry.Rate.SellRate)

	if currentBuy >= bestBuy && bestBuy > 0 {
		buyColor = AppColors.Accent1
	}
	if currentSell <= bestSell && bestSell > 0 {
		sellColor = AppColors.Accent1
	}

	return buyVal, sellVal, buyColor, sellColor
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

	// Notch Logic for Language Button
	if state.UI.LangModalButton.Hovered() {
		state.UI.HoverInfo = HoverInfo{
			Active:   true,
			Title:    GetTranslation(state.UI.Language, "notch_lang_title"),
			Subtitle: GetTranslation(state.UI.Language, "notch_lang_desc"),
			Extra:    state.UI.Language,
		}
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
func LayoutSearchBar(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	state.UI.SearchText = state.UI.SearchEditor.Text()
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
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
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutLocateButton(gtx, window, theme, state)
				})
			}),
		)
	})
}

// layoutLocateButton handles the logic and rendering of the location toggle button.
func layoutLocateButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.LocateButton.Clicked(gtx) {
		if !state.UI.UserLocation.Active {
			// Default coordinates
			state.UI.UserLocation.Latitude = 50.5744
			state.UI.UserLocation.Longitude = 22.0528
			state.UI.UserLocation.Active = true

			// Asynchronously fetch real location (Native macOS or IP fallback)
			go func() {
				lat, lon, err := FetchUserLocation()
				if err == nil && lat != 0 && lon != 0 {
					state.UI.UserLocation.Latitude = lat
					state.UI.UserLocation.Longitude = lon
					window.Invalidate()
				}
			}()
			if state.UI.MaxDistance == 0 {
				state.UI.MaxDistance = 30.0
			}
			state.UI.DistanceSlider.Value = float32(state.UI.MaxDistance / 100.0)
		} else {
			state.UI.UserLocation.Active = false
		}
		window.Invalidate()
	}

	if state.UI.LocateButton.Hovered() {
		state.UI.HoverInfo = HoverInfo{
			Active:   true,
			Title:    GetTranslation(state.UI.Language, "notch_loc_title"),
			Subtitle: GetTranslation(state.UI.Language, "notch_loc_desc"),
			Extra:    "GPS",
		}
		window.Invalidate()
	}

	btnText := GetTranslation(state.UI.Language, "locate_button")
	btn := material.Button(theme, &state.UI.LocateButton, btnText)

	btn.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 10}
	btn.Color = AppColors.Accent1
	btn.CornerRadius = unit.Dp(8)
	btn.Inset = layout.UniformInset(unit.Dp(10))
	btn.TextSize = unit.Sp(14)

	if state.UI.UserLocation.Active {
		btn.Background = AppColors.Accent1
		btn.Color = color.NRGBA{R: 20, G: 20, B: 20, A: 255}

		newVal := float64(state.UI.DistanceSlider.Value) * 100.0
		if newVal != state.UI.MaxDistance {
			state.UI.MaxDistance = newVal
			window.Invalidate()
		}

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(100))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(150))

				slider := material.Slider(theme, &state.UI.DistanceSlider)
				slider.Color = AppColors.Accent1
				return slider.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(theme, fmt.Sprintf("%.0f km", state.UI.MaxDistance))
				label.Color = AppColors.Text
				return label.Layout(gtx)
			}),
		)
	}

	return btn.Layout(gtx)
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
			l.TextSize = unit.Sp(12)
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			v := material.Body2(theme, value)
			v.Color = txtColor
			v.TextSize = unit.Sp(18)
			v.Font.Weight = font.Bold
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
func LanguageModal(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	cfg := ModalConfig{
		Title:   GetTranslation(state.UI.Language, "select_lang_title"),
		Options: state.UI.LanguageOptions,
		Buttons: state.UI.LanguageOptionButtons,
		OnSelect: func(lang string) {
			state.UI.Language = lang
			state.UI.ModalOpen = ""
			window.Invalidate()
		},
	}
	return ModalDialog(gtx, window, theme, state, cfg)
}

// ModalDialog renders a modal dialog with a title, list of options, and action buttons, and executes a callback on selection.
func ModalDialog(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config ModalConfig) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layoutModalHeader(window, theme, config.Title, state),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			state.UI.ModalList.Axis = layout.Vertical
			cols := 3
			rows := (len(config.Options) + cols - 1) / cols

			return material.List(theme, &state.UI.ModalList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
				return layoutModalGridRow(gtx, theme, row, cols, config.Options, config.Buttons, config.OnSelect)
			})
		}),
	)
}

// layoutModalHeader creates a header layout for a modal dialog, including a title and a close button.
func layoutModalHeader(window *app.Window, theme *material.Theme, title string, state *AppState) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
				btn.Color = AppColors.Button
				btn.Inset = layout.UniformInset(unit.Dp(12))
				btn.TextSize = unit.Sp(18)
				return layout.Inset{Right: unit.Dp(8), Top: unit.Dp(8)}.Layout(gtx, btn.Layout)
			}),
		)
	})
}

// layoutModalGridRow arranges a row of buttons in a grid layout within a modal, handling button clicks and triggering a callback.
func layoutModalGridRow(gtx layout.Context, theme *material.Theme, row, cols int, options []string, buttons []widget.Clickable, onSelect func(string)) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		func() []layout.FlexChild {
			children := make([]layout.FlexChild, cols)
			for c := 0; c < cols; c++ {
				idx := row*cols + c
				if idx < len(options) {
					children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
					children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
				}
			}
			return children
		}()...,
	)
}

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

// layoutNotch renders a dynamic, aesthetic notification bubble at the top of the screen (Notch-like).
func layoutNotch(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	alphaVal := state.UI.NotchState.CurrentAlpha
	if alphaVal <= 0.01 {
		return layout.Dimensions{}
	}

	info := state.UI.NotchState.LastContent

	gtx.Constraints.Min.X = gtx.Constraints.Max.X

	scaleAlpha := func(c color.NRGBA, a float32) color.NRGBA {
		c.A = uint8(float32(c.A) * a)
		return c
	}

	return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				macro := op.Record(gtx.Ops)
				dims := layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if info.Extra != "" {
								return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									m := op.Record(gtx.Ops)
									lbl := material.Caption(theme, info.Extra)
									lbl.Color = scaleAlpha(color.NRGBA{R: 20, G: 20, B: 20, A: 255}, alphaVal)
									lbl.Font.Weight = font.Bold
									d := layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, lbl.Layout)
									c := m.Stop()

									rr := gtx.Dp(10)
									bg := scaleAlpha(AppColors.Accent1, alphaVal)
									paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: d.Size}, rr).Op(gtx.Ops))

									c.Add(gtx.Ops)
									return d
								})
							}
							return layout.Dimensions{}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(theme, info.Title)
									lbl.Color = scaleAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alphaVal)
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.Caption(theme, info.Subtitle)
									lbl.Color = scaleAlpha(color.NRGBA{R: 160, G: 160, B: 170, A: 255}, alphaVal)
									return lbl.Layout(gtx)
								}),
							)
						}),
					)
				})
				call := macro.Stop()

				rr := gtx.Dp(24)
				rect := image.Rectangle{Max: dims.Size}
				bgColor := scaleAlpha(color.NRGBA{R: 20, G: 20, B: 20, A: 245}, alphaVal)
				borderColor := scaleAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 40}, alphaVal)

				paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, rr).Op(gtx.Ops))

				widget.Border{
					Color:        borderColor,
					Width:        unit.Dp(1),
					CornerRadius: unit.Dp(24),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: dims.Size}
				})

				call.Add(gtx.Ops)

				return dims
			})
		})
	})
}
