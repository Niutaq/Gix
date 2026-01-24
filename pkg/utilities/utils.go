package utilities

import (
	// Standard libraries
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/pkg/reading_data"
	"google.golang.org/protobuf/proto"
)

var (
	modalCloseBtn  widget.Clickable
	xProtoBufConst = "application/x-protobuf"
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
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layoutContent(gtx, window, theme, state, config)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layoutLoadingOverlay(gtx, window, theme, state)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutModal(gtx, window, theme, state)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutNotch(gtx, theme, state)
		}),
	)
}

func layoutContent(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	isMobile := gtx.Constraints.Max.X < gtx.Dp(unit.Dp(800))
	state.UI.IsMobile = isMobile

	if isMobile {
		return layoutContentMobile(gtx, window, theme, state, config)
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return LayoutVerticalCurrencyBar(gtx, window, theme, state, config)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutCenterPanel(gtx, window, theme, state, config)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return LayoutRightPanel(gtx, window, theme, state)
		}),
	)
}

func layoutContentMobile(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layoutCenterPanel(gtx, window, theme, state, config)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutMobileMenuOverlay(gtx, window, theme, state, config)
		}),
	)
}

// layoutCenterPanel lays out the main content panel, including the header, search bar, and cantor selection area.
func layoutCenterPanel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
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
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.UI.IsMobile {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							// Chart toggle buttons
							return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layoutChartToggle(gtx, theme, state)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layoutChartSection(gtx, window, theme, state)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
					)
				}
				return layout.Dimensions{}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layoutCantorSelection(gtx, window, theme, state, config)
			}),
		)
	})
}

// LayoutRightPanel lays out the right-side panel of the application.
func LayoutRightPanel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	panelWidth := gtx.Dp(unit.Dp(400))
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
				layout.Rigid(layout.Spacer{Height: unit.Dp(15)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutChartToggle(gtx, theme, state)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(15)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutChartSection(gtx, window, theme, state)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
				//layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				//	txt := material.Body2(theme, GetTranslation(state.UI.Language, "ai_desc"))
				//	txt.Color = color.NRGBA{R: 80, G: 80, B: 90, A: 255}
				//	return txt.Layout(gtx)
				//}),
			)
		})
}

// layoutChartSection lays out a chart section with a background and chart data using the provided theme and application state.
func layoutChartSection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(200))
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			shape := clip.UniformRRect(image.Rectangle{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}, 5)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 25, G: 25, B: 30, A: 255}, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: height}}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			data, timestamps, startLabel := prepareChartData(state)
			chart := LineChart{
				Data:       data,
				Timestamps: timestamps,
				StartLabel: startLabel,
				Tag:        &state.UI.ChartHoverTag,
			}

			chartAlpha := getChartAlpha(window, state)

			return layout.Inset{
				Top:    unit.Dp(10),
				Bottom: unit.Dp(10),
				Left:   unit.Dp(10),
				Right:  unit.Dp(10),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = height - gtx.Dp(unit.Dp(20))
				gtx.Constraints.Max.Y = height - gtx.Dp(unit.Dp(20))
				return chart.Layout(gtx, theme, chartAlpha, &state.UI)
			})
		}),
	)
}

// prepareChartData processes historical or generated data points for chart rendering, returning the data and a start label.
func prepareChartData(state *AppState) ([]float64, []int64, string) {
	if state.History != nil && len(state.History.Points) > 1 {
		return extractHistoryData(state.History, state.UI.ChartMode)
	}

	basePrice := getBasePrice(state)
	if basePrice > 0 {
		return generateMockChartData(state, basePrice)
	}
	return nil, nil, ""
}

func extractHistoryData(history *pb.HistoryResponse, mode string) ([]float64, []int64, string) {
	var data []float64
	var timestamps []int64
	for _, p := range history.Points {
		if mode == "SELL" {
			data = append(data, p.SellRate)
		} else {
			data = append(data, p.BuyRate)
		}
		timestamps = append(timestamps, p.Time)
	}
	startTime := time.Unix(history.Points[0].Time, 0)
	return data, timestamps, startTime.Format("02 Jan")
}

func getBasePrice(state *AppState) float64 {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()

	if state.UI.SelectedCantor != "" {
		if entry, ok := state.Vault.Rates[state.UI.SelectedCantor]; ok {
			return getRateFromEntry(entry, state.UI.ChartMode)
		}
	} else {
		for _, entry := range state.Vault.Rates {
			val := getRateFromEntry(entry, state.UI.ChartMode)
			if val > 0 {
				return val
			}
		}
	}
	return 0
}

func generateMockChartData(state *AppState, basePrice float64) ([]float64, []int64, string) {
	seedStr := state.UI.Currency + state.UI.SelectedCantor + state.UI.ChartMode
	var seed int64
	for _, char := range seedStr {
		seed += int64(char)
	}
	
	data := GenerateFakeData(100, basePrice, seed)
	timestamps := make([]int64, 100)
	now := time.Now().Unix()
	step := int64(3600 * 24 * 7 / 100) // Spread 7 days over 100 points
	
	for i := 0; i < 100; i++ {
		timestamps[i] = now - int64(100-i)*step
	}
	
	return data, timestamps, GetTranslation(state.UI.Language, "chart_7d_ago")
}

// getRateFromEntry extracts and parses the buy or sell rate from a CantorEntry based on the specified mode ("BUY" or "SELL").
func getRateFromEntry(entry *CantorEntry, mode string) float64 {
	if entry == nil {
		return 0
	}
	raw := entry.Rate.BuyRate
	if mode == "SELL" {
		raw = entry.Rate.SellRate
	}
	clean := strings.ReplaceAll(raw, ",", ".")
	val, err := strconv.ParseFloat(clean, 64)
	if err == nil {
		return val
	}
	return 0
}

func getChartAlpha(window *app.Window, state *AppState) uint8 {
	if !state.ChartAnimStart.IsZero() {
		elapsed := time.Since(state.ChartAnimStart)
		const duration = 600 * time.Millisecond
		if elapsed < duration {
			progress := float32(elapsed) / float32(duration)
			progress = 1.0 - (1.0-progress)*(1.0-progress)
			window.Invalidate()
			return uint8(255 * progress)
		}
	}
	return 255
}

// layoutChartToggle renders the buy/sell toggle buttons.
func layoutChartToggle(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutToggleButton(gtx, theme, &state.UI.ChartModeButtons[0],
				GetTranslation(state.UI.Language, "buy_col"),
				state.UI.ChartMode == "BUY",
				func() {
					if state.UI.ChartMode != "BUY" {
						state.UI.ChartMode = "BUY"
						state.ChartAnimStart = time.Now()
					}
				})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutToggleButton(gtx, theme, &state.UI.ChartModeButtons[1],
				GetTranslation(state.UI.Language, "sell_col"),
				state.UI.ChartMode == "SELL",
				func() {
					if state.UI.ChartMode != "SELL" {
						state.UI.ChartMode = "SELL"
						state.ChartAnimStart = time.Now()
					}
				})
		}),
	)
}

// layoutToggleButton creates a toggleable button with specified text and styling based on its active state.
func layoutToggleButton(gtx layout.Context, theme *material.Theme, btn *widget.Clickable, text string, active bool, onClick func()) layout.Dimensions {
	if btn.Clicked(gtx) {
		log.Println("Toggle clicked:", text)
		onClick()
	}

	b := material.Button(theme, btn, text)
	b.TextSize = unit.Sp(10)
	b.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
	b.CornerRadius = unit.Dp(4)

	if active {
		b.Background = AppColors.Accent1
		b.Color = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	} else {
		b.Background = color.NRGBA{R: 30, G: 30, B: 35, A: 255}
		b.Color = color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	}

	return b.Layout(gtx)
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

// moveTowards linearly interpolates from the current value toward the target by a maximum change of maxDelta.
func moveTowards(current, target, maxDelta float32) float32 {
	if diff := target - current; diff < 0 {
		diff = -diff
		if diff <= maxDelta {
			return target
		}
		return current - maxDelta
	}
	if diff := target - current; diff <= maxDelta {
		return target
	}
	return current + maxDelta
}

// drawPatternBackground fills the background with a dark color for the application UI using the provided layout context.
func drawPatternBackground(gtx layout.Context) {
	paint.Fill(gtx.Ops, color.NRGBA{R: 15, G: 15, B: 25, A: 255})
}

// FetchAllRates initiates the concurrent fetching of exchange rates.
func FetchAllRates(window *app.Window, state *AppState, config AppConfig) {
	if config.APIRatesURL == "" {
		return
	}

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	state.Vault.Mu.Lock()
	state.Vault.Rates = make(map[string]*CantorEntry)
	state.Vault.Mu.Unlock()

	go FetchAllRatesRPC(window, state, config.APICantorsURL)
	fetchHistory(window, state, config)
}

// fetchHistory retrieves historical currency data from the API and updates the application's state with the response.
func fetchHistory(window *app.Window, state *AppState, config AppConfig) {
	if config.APIHistoryURL == "" {
		return
	}

	currency := state.UI.Currency
	cantorID := getSelectedCantorID(state)

	go performHistoryFetch(window, state, config, currency, cantorID)
}

func getSelectedCantorID(state *AppState) int {
	if state.UI.SelectedCantor != "" {
		if c, ok := state.Cantors[state.UI.SelectedCantor]; ok {
			return c.ID
		}
	}
	return 0
}

func performHistoryFetch(window *app.Window, state *AppState, config AppConfig, curr string, cID int) {
	url := buildHistoryURL(config.APIHistoryURL, curr, cID)

	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating history request: %v", err)
		return
	}
	req.Header.Set("Accept", xProtoBufConst)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error fetching history: %v", err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing history response body: %v", err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading history body: %v", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Server returned error %d: %s", resp.StatusCode, string(body))
		return
	}

	var history pb.HistoryResponse
	if err := proto.Unmarshal(body, &history); err != nil {
		log.Printf("Error unmarshalling history: %v", err)
		return
	}

	if len(history.Points) > 0 {
		log.Printf("Received %d history points for %s (CantorID: %d)", len(history.Points), history.Currency, cID)
	} else {
		log.Printf("Received empty history for %s (CantorID: %d)", history.Currency, cID)
	}

	state.History = &history
	state.ChartAnimStart = time.Now()
	window.Invalidate()
}

func buildHistoryURL(baseURL, curr string, cID int) string {
	url := fmt.Sprintf("%s?currency=%s&days=7", baseURL, curr)
	if cID > 0 {
		url += fmt.Sprintf("&cantor_id=%d", cID)
	}
	return url
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
							state.ChartAnimStart = time.Now()
							FetchAllRates(window, state, config)
							window.Invalidate()
						}

						return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								// Minimalist style
								txtColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
								if isSelected {
									txtColor = AppColors.Accent1
								} else if btn.Hovered() {
									txtColor = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
								}

																	return layout.Stack{}.Layout(gtx,

																		// Hover/Select Background (bardzo subtelne)

																		layout.Expanded(func(gtx layout.Context) layout.Dimensions {

																			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))

																			if isSelected {

																				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 5}, shape.Op(gtx.Ops))

																			} else if btn.Hovered() {

																				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 2}, shape.Op(gtx.Ops))

																			}

																			return layout.Dimensions{Size: gtx.Constraints.Min}

																		}),

								
									layout.Stacked(func(gtx layout.Context) layout.Dimensions {
										// Accent Bar (pionowa kreska)
										if isSelected {
											barRect := image.Rect(0, 12, 3, 38) // Stała wysokość paska dla estetyki
											paint.FillShape(gtx.Ops, AppColors.Accent1, clip.UniformRRect(barRect, 1).Op(gtx.Ops))
										}
										
										// Content
										return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												lbl := material.Body2(theme, currency)
												lbl.Color = txtColor
												lbl.Font.Weight = font.Bold
												lbl.TextSize = unit.Sp(13) // Nieco większy tekst
												return lbl.Layout(gtx)
											})
										})
									}),
								)
							})
						})
					})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutLanguageButton(gtx, window, theme, state)
				})
			})
		}),
	)
}

// parseRate converts a string representing a rate (e.g., "123,45 zł") to a float64, removing commas, "zł", and whitespace.
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

// layoutCantorSelection renders a filtered and sorted list of cantors, handling animations and layout logic for the UI interface.
func layoutCantorSelection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
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
					BestSell: bestSell,
				}
				return layoutCantorItem(gtx, window, theme, state, config, rowCfg)
			})
	})
}

// calculateBestRates identifies the Best Buy and sell rates from the provided map of CantorEntry data.
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

// filterCantorList filters a list of cantors based on search text and user location preferences in the application state.
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

// layoutCantorItem renders a single row representing a cantor item within the layout, including animations and interactions.
func layoutCantorItem(
	gtx layout.Context, window *app.Window, theme *material.Theme,
	state *AppState, config AppConfig, cfg CantorRowConfig) layout.Dimensions {
	cantorKey := cfg.CantorID
	cantor := state.Cantors[cantorKey]
	displayName := GetTranslation(state.UI.Language, cantor.DisplayName)

	state.Vault.Mu.Lock()
	entry := state.Vault.Rates[cantorKey]
	state.Vault.Mu.Unlock()

	alpha := getAnimationAlpha(window, entry)
	buyVal, sellVal, spreadVal, buyColor, sellColor, change := getCantorDisplayData(state, entry, cfg.BestBuy, cfg.BestSell)
	buyColor.A = alpha
	sellColor.A = alpha

	handleCantorHover(window, state, cantor, cantorKey, displayName)
	handleCantorClick(gtx, window, state, config, cantorKey, cantor)

	isSelected := state.UI.SelectedCantor == cantorKey
	return renderCantorItem(gtx, theme, state, CantorItemArgs{
		Cantor:      cantor,
		DisplayName: displayName,
		BuyVal:      buyVal,
		SellVal:     sellVal,
		SpreadVal:   spreadVal,
		BuyColor:    buyColor,
		SellColor:   sellColor,
		Change24h:   change,
		IsSelected:  isSelected,
	})
}

// handleCantorHover updates the UI hover information based on cantor hover state, user location, and application language.
func handleCantorHover(window *app.Window, state *AppState, cantor *CantorInfo, cantorKey, displayName string) {
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
			case "alex":
				address = "Al. Tadeusza Rejtana 65, 35-310 Rzeszów"
			case "grosz":
				address = "Sławkowska 4, 31-014 Kraków"
			case "centrum":
				address = "Świdnicka 3, 50-064 Wrocław"
			case "lider":
				address = "Wolności 1, 41-800 Zabrze"
			case "baks":
				address = "Marszałkowska 85, 00-683 Warszawa"
			case "waluciarz":
				address = "Szewska 21, 31-009 Kraków"
			case "joker":
				address = "Piłsudskiego 34, 35-001 Rzeszów"
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
}

// handleCantorClick handles the click event for a cantor item, toggling its selection and initiating chart data updates.
func handleCantorClick(gtx layout.Context, window *app.Window, state *AppState, config AppConfig, cantorKey string, cantor *CantorInfo) {
	if cantor.Button.Clicked(gtx) {
		if state.UI.SelectedCantor == cantorKey {
			state.UI.SelectedCantor = ""
		} else {
			state.UI.SelectedCantor = cantorKey
		}
		state.ChartAnimStart = time.Now()
		fetchHistory(window, state, config)
	}
}

type CantorItemArgs struct {
	Cantor      *CantorInfo
	DisplayName string
	BuyVal      string
	SellVal     string
	SpreadVal   string
	BuyColor    color.NRGBA
	SellColor   color.NRGBA
	Change24h   float64
	IsSelected  bool
}

// renderCantorItem renders a single interactive UI element to represent a cantor, including labels, values, and highlight state.
func renderCantorItem(gtx layout.Context, theme *material.Theme, state *AppState, args CantorItemArgs) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return args.Cantor.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bgColor := color.NRGBA{R: 30, G: 30, B: 35, A: 150}
			if args.IsSelected {
				bgColor = color.NRGBA{R: 90, G: 65, B: 25, A: 255}
			} else if args.Cantor.Button.Hovered() {
				bgColor = color.NRGBA{R: 45, G: 45, B: 55, A: 200}
			}

			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(6))
					paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Body1(theme, args.DisplayName)
								lbl.Color = AppColors.Text
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := GetTranslation(state.UI.Language, "buy_col")
										return layoutMarketValue(gtx, theme, label, args.BuyVal, args.BuyColor, args.Change24h)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(30)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := GetTranslation(state.UI.Language, "sell_col")
										return layoutMarketValue(gtx, theme, label, args.SellVal, args.SellColor, 0)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(30)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := GetTranslation(state.UI.Language, "spread_col")
										return layoutMarketValue(gtx, theme, label, args.SpreadVal, AppColors.Spread, 0)
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

// ... (getAnimationAlpha, getCantorDisplayData remain same)

// layoutMarketValue lays out a market value displaying a label and its corresponding value with customizable text color.
func layoutMarketValue(
	gtx layout.Context, theme *material.Theme,
	label, value string, txtColor color.NRGBA, change float64) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Caption(theme, label)
			l.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
			l.TextSize = unit.Sp(12)
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					v := material.Body2(theme, value)
					v.Color = txtColor
					v.TextSize = unit.Sp(18)
					v.Font.Typeface = "Kanit"
					v.Font.Weight = font.Bold
					return v.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if change == 0 {
						return layout.Dimensions{}
					}
					col := AppColors.Success
					txt := fmt.Sprintf(" +%.2f%% (24h)", change)
					if change < 0 {
						col = AppColors.Error
						txt = fmt.Sprintf(" %.2f%% (24h)", change)
					}
					c := material.Caption(theme, txt)
					c.Color = col
					c.TextSize = unit.Sp(10)
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, c.Layout)
				}),
			)
		}),
	)
}

// getAnimationAlpha calculates an alpha value for animations, based on the entry's load time, with an ease-out effect.
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

// getCantorDisplayData generates display values for buy and sell rates along with their respective colors based on conditions.
func getCantorDisplayData(
	state *AppState, entry *CantorEntry, bestBuy, bestSell float64) (string, string, string, color.NRGBA, color.NRGBA, float64) {
	defColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	if entry == nil {
		return "---", "---", "---", defColor, defColor, 0
	}

	if entry.Error != "" {
		errTxt := GetTranslation(state.UI.Language, "no_rate_label")
		return errTxt, errTxt, "---", AppColors.Error, AppColors.Error, 0
	}

	buyVal := entry.Rate.BuyRate + " zł"
	sellVal := entry.Rate.SellRate + " zł"
	buyColor := AppColors.Text
	sellColor := AppColors.Text

	currentBuy := parseRate(entry.Rate.BuyRate)
	currentSell := parseRate(entry.Rate.SellRate)
	spreadVal := "---"

	if currentBuy > 0 && currentSell > 0 {
		spread := currentSell - currentBuy
		spreadVal = fmt.Sprintf("%.4f", spread)
	}

	if currentBuy >= bestBuy && bestBuy > 0 {
		buyColor = AppColors.Accent1
	}
	if currentSell <= bestSell && bestSell > 0 {
		sellColor = AppColors.Accent1
	}

	return buyVal, sellVal, spreadVal, buyColor, sellColor, entry.Rate.Change24h
}

// layoutHeader renders the application's header, including the market title, subtitle, and a language selection button.
func layoutHeader(gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {
	isMobile := state.UI.IsMobile

	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Mobile Menu Button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !isMobile {
							return layout.Dimensions{}
						}
						if state.UI.MobileMenuBtn.Clicked(gtx) {
							state.UI.MobileMenuOpen = !state.UI.MobileMenuOpen
							window.Invalidate()
						}
						
						return layout.Inset{Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return state.UI.MobileMenuBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return DrawIconMenu(gtx, AppColors.Text)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										h2 := material.H5(theme, GetTranslation(state.UI.Language, "market_title"))
										h2.Color = AppColors.Text
										h2.Font.Weight = font.Bold
										return h2.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										// Connection Status Dot
										isConnected := state.IsConnected.Load()
										t := float64(time.Now().UnixMilli()) / 500.0
										alpha := (math.Sin(t) + 1.0) / 2.0

										col := color.NRGBA{R: 100, G: 100, B: 110, A: 255} // Default Gray
										statusTitle := "Offline"
										statusDesc := GetTranslation(state.UI.Language, "status_offline_desc")

										if isConnected {
											col = AppColors.Success
											col.A = uint8(150 + (105 * alpha))
											statusTitle = "Live"
											statusDesc = GetTranslation(state.UI.Language, "status_live_desc")
											window.Invalidate()
										}

										// Status Tooltip logic
										if state.UI.StatusClickable.Hovered() {
											state.UI.HoverInfo = HoverInfo{
												Active:   true,
												Title:    statusTitle,
												Subtitle: statusDesc,
												Extra:    "dRPC",
											}
											window.Invalidate()
										}

										sz := gtx.Dp(8)
										return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return state.UI.StatusClickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												circle := clip.Ellipse{Max: image.Point{X: sz, Y: sz}}.Op(gtx.Ops)
												paint.FillShape(gtx.Ops, col, circle)
												return layout.Dimensions{Size: image.Point{X: sz, Y: sz}}
											})
										})
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								caption := material.Caption(theme, GetTranslation(state.UI.Language, "market_subtitle"))
								caption.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
								return caption.Layout(gtx)
							}),
						)
					}),
				)
			}),
		)
	})
}

// layoutLanguageButton renders the language selection button (mini version for sidebar).
func layoutLanguageButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.LangModalButton.Clicked(gtx) {
		state.UI.ModalOpen = "language"
		window.Invalidate()
	}

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
	btn.Color = AppColors.Text
	btn.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 5}
	btn.CornerRadius = unit.Dp(8)
	btn.TextSize = unit.Sp(12)
	btn.Inset = layout.UniformInset(unit.Dp(10))
	return btn.Layout(gtx)
}

// LayoutSearchBar renders a search bar component with a text editor and a Locate button, styled and responsive to the current state.
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

// layoutLocateButton renders the Locate button, handles its click events, and dynamically updates the UI state.
func layoutLocateButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	handleLocateClick(gtx, window, state)
	handleLocateHover(window, state)

	btnText := GetTranslation(state.UI.Language, "locate_button")
	btn := material.Button(theme, &state.UI.LocateButton, btnText)

	btn.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 10}
	btn.Color = AppColors.Accent1
	btn.CornerRadius = unit.Dp(8)
	btn.Inset = layout.UniformInset(unit.Dp(10))
	btn.TextSize = unit.Sp(14)

	if state.UI.UserLocation.Active {
		return layoutLocateActiveState(gtx, window, theme, state, btn)
	}

	return btn.Layout(gtx)
}

func handleLocateClick(gtx layout.Context, window *app.Window, state *AppState) {
	if state.UI.LocateButton.Clicked(gtx) {
		if !state.UI.UserLocation.Active {
			state.UI.UserLocation.Active = true

			go func() {
				lat, lon, err := FetchUserLocation()
				if err == nil && lat != 0 && lon != 0 {
					fmt.Printf("Location success: %f, %f\n", lat, lon)
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
}

func handleLocateHover(window *app.Window, state *AppState) {
	if state.UI.LocateButton.Hovered() {
		state.UI.HoverInfo = HoverInfo{
			Active:   true,
			Title:    GetTranslation(state.UI.Language, "notch_loc_title"),
			Subtitle: GetTranslation(state.UI.Language, "notch_loc_desc"),
			Extra:    "GPS",
		}
		window.Invalidate()
	}
}

func layoutLocateActiveState(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, btn material.ButtonStyle) layout.Dimensions {
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

// layoutModal renders a modal overlay if a modal is currently open in the application state.
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

// ModalOverlay displays a centrally aligned modal overlay with optional content and handles dismissal on background click.
func ModalOverlay(
	gtx layout.Context, window *app.Window,
	state *AppState, content layout.Widget) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{A: 210})
	if state.UI.ModalClick.Clicked(gtx) {
		state.UI.ModalOpen = ""
		window.Invalidate()
		return layout.Dimensions{}
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
						rr := gtx.Dp(10)
						// Create clip area
						stack := clip.UniformRRect(rect, rr).Op(gtx.Ops).Push(gtx.Ops)

						// Gradient Background
						paint.LinearGradientOp{
							Stop1:  f32.Point{X: 0, Y: 0},
							Stop2:  f32.Point{X: 0, Y: float32(rect.Max.Y)},
							Color1: color.NRGBA{R: 25, G: 25, B: 35, A: 255}, // Dark Blueish
							Color2: color.NRGBA{R: 10, G: 10, B: 15, A: 255}, // Almost Black
						}.Add(gtx.Ops)
						paint.PaintOp{}.Add(gtx.Ops)

						stack.Pop()
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

// LanguageModal renders a modal for selecting a language using a vertical wheel-picker style list.
func LanguageModal(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.UI.Language, "select_lang_title")
	
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layoutModalHeader(window, theme, title, state),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			state.UI.ModalList.Axis = layout.Vertical
			options := state.UI.LanguageOptions
			buttons := state.UI.LanguageOptionButtons

			// Draw fading gradients at top and bottom to simulate 3D wheel depth
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return material.List(theme, &state.UI.ModalList).Layout(gtx, len(options), func(gtx layout.Context, index int) layout.Dimensions {
						lang := options[index]
						if buttons[index].Clicked(gtx) {
							state.UI.Language = lang
							state.UI.ModalOpen = "" // Close on select
							window.Invalidate()
						}

						isSelected := state.UI.Language == lang
						
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(theme, &buttons[index], lang)
							btn.Background = color.NRGBA{A: 0}
							btn.Inset = layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}
							
							if isSelected {
								btn.Color = AppColors.Accent1
								btn.TextSize = unit.Sp(24)
								btn.Font.Weight = font.Bold
							} else {
								btn.Color = color.NRGBA{R: 120, G: 120, B: 130, A: 255}
								btn.TextSize = unit.Sp(16)
								btn.Font.Weight = font.Normal
							}
							
							return btn.Layout(gtx)
						})
					})
				}),
				// Top Gradient Overlay
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					h := gtx.Dp(40)
					paint.LinearGradientOp{
						Stop1:  f32.Point{X: 0, Y: 0},
						Stop2:  f32.Point{X: 0, Y: float32(h)},
						Color1: color.NRGBA{R: 20, G: 20, B: 25, A: 255}, // Background color
						Color2: color.NRGBA{R: 20, G: 20, B: 25, A: 0},
					}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: h}}
				}),
				// Bottom Gradient Overlay
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					h := gtx.Dp(40)
					// Position at bottom
					off := gtx.Constraints.Max.Y - h
					defer op.Offset(image.Point{Y: off}).Push(gtx.Ops).Pop()
					
					paint.LinearGradientOp{
						Stop1:  f32.Point{X: 0, Y: 0},
						Stop2:  f32.Point{X: 0, Y: float32(h)},
						Color1: color.NRGBA{R: 20, G: 20, B: 25, A: 0},
						Color2: color.NRGBA{R: 20, G: 20, B: 25, A: 255},
					}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: h}}
				}),
			)
		}),
	)
}

// ModalDialog renders a modal dialog with a title, grid of selectable options, and buttons using the provided configuration.
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

// layoutModalHeader creates a modal header with a title and a close button, styled according to the provided theme and state.
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

// layoutModalGridRow arranges a row of buttons in a modal grid layout, handling click events and callbacks for each option.
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

// layoutLoadingOverlay renders a dimming overlay with a growing stock chart animation.
func layoutLoadingOverlay(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if !state.IsLoading.Load() {
		return layout.Dimensions{}
	}

	// Dimming background
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 120})

	// Animation Progress
	elapsed := time.Since(state.IsLoadingStart).Seconds()
	duration := 1.2
	progress := float32(math.Mod(elapsed, duration) / duration)
	
	// Easing for smoother effect
	progress = progress * (2 - progress) 

	// Keep animating
	window.Invalidate()

	// Position at the top center
	return layout.Inset{Top: unit.Dp(80)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(100))
			height := gtx.Dp(unit.Dp(40))

			// Draw animating chart
			// We clip the drawing area based on progress
			currentWidth := int(float32(width) * progress)
			defer clip.Rect{Max: image.Point{X: currentWidth, Y: height}}.Push(gtx.Ops).Pop()

			var path clip.Path
			path.Begin(gtx.Ops)
			
			// Points: X is 0..1, Y is 0..1 (0 is top)
			// Let's make a nice "uptrend" shape
			startY := float32(height)
			path.MoveTo(f32.Point{X: 0, Y: startY})
			
			points := []f32.Point{
				{0.1, 0.7}, {0.3, 0.8}, {0.4, 0.4}, {0.6, 0.5}, {0.7, 0.2}, {0.9, 0.3}, {1.0, 0.0},
			}
			
			prevX := float32(0)
			prevY := startY
			
			for _, p := range points {
				px := p.X * float32(width)
				py := p.Y * float32(height)
				path.CubeTo(
					f32.Point{X: (prevX + px)/2, Y: prevY}, 
					f32.Point{X: (prevX + px)/2, Y: py}, 
					f32.Point{X: px, Y: py},
				)
				prevX = px
				prevY = py
			}

			paint.FillShape(gtx.Ops, AppColors.Accent1, clip.Stroke{Path: path.End(), Width: 3.0}.Op())
			
			return layout.Dimensions{Size: image.Point{X: width, Y: height}}
		})
	})
}

// LoadFontCollection loads a collection of font faces from predefined font files and returns them.
func LoadFontCollection() ([]font.FontFace, error) {
	var collection []font.FontFace

	f1, err := reading_data.LoadAndParseFont("fonts/Montserrat-Bold.ttf")
	if err == nil {
		f1.Font.Typeface = "Montserrat"
		collection = append(collection, f1)
	}

	f2, err := reading_data.LoadAndParseFont("fonts/Kanit-Regular.ttf")
	if err == nil {
		f2.Font.Typeface = "Kanit"
		collection = append(collection, f2)
	}

	f3, err := reading_data.LoadAndParseFont("fonts/NS.ttf")
	if err == nil {
		f3.Font.Typeface = "Monospace"
		collection = append(collection, f3)
	}

	return collection, nil
}

// layoutNotch renders a UI "notch" element with dynamic alpha, displaying contextually relevant information to the user.
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
