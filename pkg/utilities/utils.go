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
	"runtime"
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

// MarketValueArgs holds arguments for rendering a market value display.
type MarketValueArgs struct {
	Label         string
	Value         string
	Color         color.NRGBA
	Change        float64
	DisplayChange string
	IsMobile      bool
}

// CantorItemArgs holds arguments for rendering a cantor item row.
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
	IsMobile    bool
	Scale       float32
}

// LayoutUI - Main application layout
func LayoutUI(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) {
	updateNotchAnimation(window, state)

	// Switch palette based on LightMode
	if state.UI.LightMode {
		AppColors = LightPalette
	} else {
		AppColors = DarkPalette
	}

	state.UI.SearchEditor.SingleLine = true
	state.UI.SearchEditor.Submit = true

	if state.Vault.Rates == nil {
		state.Vault.Rates = make(map[string]map[string]*CantorEntry)
	}

	drawPatternBackground(gtx)

	layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layoutContent(gtx, window, theme, state, config)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutModal(gtx, window, theme, state)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutIntroAnimation(gtx, window, state)
		}),
	)
}

// layoutContent lays out the main content panel of the application.
func layoutContent(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	isMobile := gtx.Constraints.Max.X < gtx.Dp(unit.Dp(1050))

	// Smooth Transition Logic
	if state.UI.IsMobile != isMobile {
		state.UI.IsMobile = isMobile
		state.UI.LayoutTransitionTime = time.Now()
		window.Invalidate()
	}

	// Calculate Opacity based on time since transition
	opacity := float32(1.0)
	if !state.UI.LayoutTransitionTime.IsZero() {
		elapsed := time.Since(state.UI.LayoutTransitionTime).Seconds()
		duration := 0.4
		if elapsed < duration {
			opacity = float32(elapsed / duration)
			// Cubic ease-out
			opacity = 1.0 - (1.0-opacity)*(1.0-opacity)*(1.0-opacity)
			window.Invalidate()
		} else {
			state.UI.LayoutTransitionTime = time.Time{} // Reset
		}
	}

	// Apply Opacity Wrapper
	macro := op.Record(gtx.Ops)
	var dims layout.Dimensions
	if isMobile {
		dims = layoutContentMobile(gtx, window, theme, state, config)
	} else {
		dims = layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return LayoutVerticalCurrencyBar(gtx, window, theme, state, config)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layoutCenterPanel(gtx, window, theme, state, config)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return LayoutRightPanel(gtx, window, theme, state, config)
			}),
		)
	}
	call := macro.Stop()

	// Draw with opacity
	if opacity < 1.0 {
		call.Add(gtx.Ops)

		fadeOverlay := float32(1.0) - opacity
		paint.Fill(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 35, A: uint8(255 * fadeOverlay)})
	} else {
		call.Add(gtx.Ops)
	}

	return dims
}

// layoutContentMobile lays out the main content panel for mobile devices.
func layoutContentMobile(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(50)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutCenterPanel(gtx, window, theme, state, config)
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutMobileMenuOverlay(gtx, window, theme, state, config)
		}),
	)
}

// layoutCenterPanel lays out the main content panel, including the header, search bar, and cantor selection area.
func layoutCenterPanel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	// Mobile Landscape Logic
	if state.UI.IsMobile && gtx.Constraints.Max.X > gtx.Constraints.Max.Y {
		return layoutCenterPanelMobileLandscape(gtx, window, theme, state, config)
	}

	topInset := unit.Dp(16)
	if state.UI.IsMobile && state.UI.SearchActive {
		topInset = unit.Dp(8)
	}

	return layout.Inset{
		Left:  unit.Dp(24),
		Right: unit.Dp(24),
		Top:   topInset,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// If height is very low (keyboard open), hide the search bar as well if search is active
		// This ensures the list has maximum space and prevents layout "jitter/squashing"
		hideSearch := state.UI.IsMobile && state.UI.SearchActive && gtx.Constraints.Max.Y < gtx.Dp(unit.Dp(450))

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutHeader(gtx, window, theme, state)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if hideSearch {
					return layout.Dimensions{}
				}
				return LayoutSearchBar(gtx, window, theme, state)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Hide Movers when searching on mobile to save CPU and space
				if state.UI.IsMobile && state.UI.SearchActive {
					return layout.Dimensions{}
				}
				return layoutInfoBar(gtx, window, theme, state)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// Hide Chart/Controls when searching on mobile to prevent squashing and lag
				if state.UI.IsMobile && state.UI.SearchActive {
					return layout.Dimensions{}
				}
				return layoutMobileControls(gtx, window, theme, state, config)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layoutCantorSelection(gtx, window, theme, state, config)
			}),
		)
	})
}

// layoutInfoBar lays out the information bar at the top of the main content panel.
func layoutInfoBar(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutTopMovers(gtx, window, theme, state)
		})
	})
}

// layoutMobileControls renders the chart and location controls for mobile devices.
func layoutMobileControls(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	if !state.UI.IsMobile {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutChartSection(gtx, window, theme, state, config)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutLocateButton(gtx, window, theme, state)
			})
		}),
	)
}

// layoutCenterPanelMobileLandscape lays out the main content panel for mobile devices in landscape orientation.
func layoutCenterPanelMobileLandscape(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	topInset := unit.Dp(16)
	if gtx.Constraints.Max.Y < gtx.Dp(unit.Dp(400)) {
		topInset = unit.Dp(8)
	}

	return layout.Inset{
		Left:  unit.Dp(24),
		Right: unit.Dp(24),
		Top:   topInset,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			// Left Column: Header, Chart, Controls - adjusted ratio
			layout.Flexed(0.55, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutHeader(gtx, window, theme, state)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return LayoutSearchBar(gtx, window, theme, state)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if state.UI.SearchActive {
							return layout.Dimensions{}
						}
						return layoutInfoBar(gtx, window, theme, state)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if state.UI.SearchActive {
							return layout.Dimensions{}
						}
						// Adjust spacing before controls in landscape
						spacerHeight := unit.Dp(20)
						if gtx.Constraints.Max.Y < gtx.Dp(unit.Dp(400)) {
							spacerHeight = unit.Dp(8)
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(layout.Spacer{Height: spacerHeight}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutLocateButton(gtx, window, theme, state)
							}),
						)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
			// Right Column: List
			layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
				return layoutCantorSelection(gtx, window, theme, state, config)
			}),
		)
	})
}

// LayoutRightPanel lays out the right-side panel of the application.
func LayoutRightPanel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	panelWidth := gtx.Dp(unit.Dp(400))
	gtx.Constraints.Min.X = panelWidth
	gtx.Constraints.Max.X = panelWidth
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, AppColors.Dark, clip.Rect{Max: gtx.Constraints.Max}.Op())

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
					return layoutChartSection(gtx, window, theme, state, config)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
			)
		})
}

// the layoutChartSection lays out the chart section of the main content panel.
func layoutChartSection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	chartHeight := unit.Dp(180)
	if state.UI.IsMobile && gtx.Constraints.Max.X > gtx.Constraints.Max.Y {
		chartHeight = unit.Dp(110) // Much shorter in landscape to save vertical space
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bottomInset := unit.Dp(12)
			if state.UI.IsMobile && gtx.Constraints.Max.X > gtx.Constraints.Max.Y {
				bottomInset = unit.Dp(4)
			}
			return layout.Inset{Bottom: bottomInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Currency Pair Label
						pair := fmt.Sprintf("%s/PLN   ", state.UI.Currency)
						lbl := material.Caption(theme, pair)
						lbl.Color = AppColors.Text
						lbl.Font.Weight = font.Bold
						lbl.TextSize = unit.Sp(11)
						return lbl.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Group toggles together
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutChartToggle(gtx, theme, state)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutTimeframeBar(gtx, window, theme, state, config)
							}),
						)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(chartHeight)
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					bgColor := color.NRGBA{R: 20, G: 20, B: 25, A: 255}
					if state.UI.LightMode {
						bgColor = color.NRGBA{R: 240, G: 240, B: 245, A: 255}
					}
					shape := clip.UniformRRect(image.Rectangle{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}, 12)
					paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
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
						return chart.Layout(gtx, window, theme, chartAlpha, &state.UI)
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if state.IsLoading.Load() {
						return layoutChartLoading(gtx, window, state, height)
					}
					return layout.Dimensions{}
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Always reserve space for the notch below the chart to prevent jumping
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(40))
			gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(40))

			alpha := state.UI.NotchState.CurrentAlpha
			if alpha <= 0.001 {
				return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y}}
			}
			info := state.UI.NotchState.LastContent
			if info.Extra == "UI" {
				return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y}}
			}

			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutNotch(gtx, theme, state)
			})
		}),
	)
}

// layoutTimeframeBar lays out the timeframe selection buttons for the chart section.
func layoutTimeframeBar(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutTimeframeButton(gtx, theme, state, TimeframeButtonArgs{window, config, 0, "1D", GetTranslation(state.UI.Language, "tf_1d")})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutTimeframeButton(gtx, theme, state, TimeframeButtonArgs{window, config, 1, "7D", GetTranslation(state.UI.Language, "tf_7d")})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutTimeframeButton(gtx, theme, state, TimeframeButtonArgs{window, config, 2, "30D", GetTranslation(state.UI.Language, "tf_30d")})
		}),
	)
}

// TimeframeButtonArgs holds arguments for rendering a timeframe selection button.
type TimeframeButtonArgs struct {
	Window *app.Window
	Config AppConfig
	Index  int
	TF     string
	Text   string
}

// layoutTimeframeButton lays out a timeframe selection button.
func layoutTimeframeButton(gtx layout.Context, theme *material.Theme, state *AppState, args TimeframeButtonArgs) layout.Dimensions {
	btn := &state.UI.TimeframeButtons[args.Index]
	if btn.Clicked(gtx) {
		state.UI.Timeframe = args.TF
		fetchHistory(args.Window, state, args.Config)
	}

	active := state.UI.Timeframe == args.TF
	return layoutToggleButton(gtx, theme, btn, args.Text, active, func() {
		state.UI.Timeframe = args.TF
		fetchHistory(args.Window, state, args.Config)
	})
}

// layoutChartLoading renders a loading animation overlay specifically for the chart section.
func layoutChartLoading(gtx layout.Context, window *app.Window, state *AppState, height int) layout.Dimensions {
	overlayColor := color.NRGBA{R: 20, G: 20, B: 25, A: 200}
	trendColor := AppColors.Accent1
	if state.UI.LightMode {
		overlayColor = color.NRGBA{R: 245, G: 245, B: 250, A: 150}
		trendColor = color.NRGBA{R: 100, G: 100, B: 120, A: 200}
	}
	paint.FillShape(gtx.Ops, overlayColor, clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: height}}.Op())

	elapsed := time.Since(state.IsLoadingStart).Seconds()
	duration := 1.5
	progress := float32(math.Mod(elapsed, duration) / duration)

	window.Invalidate()

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Dp(80)
		h := gtx.Dp(40)
		return drawSmoothTrend(gtx, w, h, trendColor, progress, false)
	})
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

// extractHistoryData processes historical data points for chart rendering, returning the data, timestamps, and a start label.
func extractHistoryData(history *pb.HistoryResponse, mode string) ([]float64, []int64, string) {
	if history == nil || len(history.Points) == 0 {
		return nil, nil, ""
	}
	var data []float64
	var timestamps []int64
	for i, p := range history.Points {
		val := float64(p.BuyRate) / 1000.0
		if mode == "SELL" {
			val = float64(p.SellRate) / 1000.0
		}

		if i < 5 { // Log first 5 points for debugging
			log.Printf("Chart Debug [%s]: Point %d = %f (Raw Buy: %d, Raw Sell: %d)", mode, i, val, p.BuyRate, p.SellRate)
		}

		data = append(data, val)
		timestamps = append(timestamps, p.Time)
	}
	startTime := time.Unix(history.Points[0].Time, 0)
	return data, timestamps, startTime.Format("02 Jan")
}

// getBasePrice extracts and returns the base price for the selected currency and chart mode.
func getBasePrice(state *AppState) float64 {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()

	currencyRates := state.Vault.Rates[state.UI.Currency]
	if currencyRates == nil {
		return 0
	}

	if state.UI.SelectedCantor != "" {
		if entry, ok := currencyRates[state.UI.SelectedCantor]; ok {
			return getRateFromEntry(entry, state.UI.ChartMode)
		}
	} else {
		for _, entry := range currencyRates {
			val := getRateFromEntry(entry, state.UI.ChartMode)
			if val > 0 {
				return val
			}
		}
	}
	return 0
}

// generateMockChartData generates mock chart data for the selected currency and chart mode.
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

	for i := range 100 {
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

// getChartAlpha calculates the alpha value for the chart based on the animation progress.
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
	b.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}
	b.CornerRadius = unit.Dp(4)

	if active {
		b.Background = AppColors.Accent1
		b.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	} else {
		if AppColors.Background.R > 200 { // Light Mode
			b.Background = color.NRGBA{R: 230, G: 230, B: 235, A: 255}
			b.Color = color.NRGBA{R: 60, G: 60, B: 70, A: 255}
		} else { // Dark Mode
			b.Background = color.NRGBA{R: 35, G: 35, B: 40, A: 255}
			b.Color = color.NRGBA{R: 180, G: 180, B: 190, A: 255}
		}
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

	shouldShow := shouldShowNotch(state, now)

	targetAlpha := float32(0.0)
	if shouldShow {
		targetAlpha = 1.0
		state.UI.NotchState.LastContent = state.UI.HoverInfo
	}

	speed := float32(8.0)
	if targetAlpha == 0 {
		speed = 4.0
	}

	// Exponential smoothing for fluid animation (Lerp)
	// 1 - exp(-dt * speed) makes it frame-rate independent
	factor := float32(1.0 - math.Exp(-float64(dt)*float64(speed)))
	diff := targetAlpha - state.UI.NotchState.CurrentAlpha
	if math.Abs(float64(diff)) < 0.001 {
		state.UI.NotchState.CurrentAlpha = targetAlpha
	} else {
		state.UI.NotchState.CurrentAlpha += diff * factor
	}

	if state.UI.NotchState.CurrentAlpha > 0.01 || state.UI.HoverInfo.Active {
		window.Invalidate()
	}
	state.UI.HoverInfo = HoverInfo{Active: false}
}

// shouldShowNotch determines whether the notch should be shown based on the current state and time.
func shouldShowNotch(state *AppState, now time.Time) bool {
	if state.UI.HoverInfo.Active {
		if state.UI.NotchState.HoverStartTime.IsZero() {
			state.UI.NotchState.HoverStartTime = now
		}
		state.UI.NotchState.LastHoverTime = now
	} else {
		state.UI.NotchState.HoverStartTime = time.Time{}
	}

	settleDelay := 1500 * time.Millisecond
	if state.UI.IsMobile {
		settleDelay = 0
	}

	isSettled := !state.UI.NotchState.HoverStartTime.IsZero() && now.Sub(state.UI.NotchState.HoverStartTime) > settleDelay
	return isSettled || (state.UI.NotchState.CurrentAlpha > 0.01 && now.Sub(state.UI.NotchState.LastHoverTime) < 500*time.Millisecond)
}

// drawPatternBackground fills the background with the current theme's background color.
func drawPatternBackground(gtx layout.Context) {
	bg := AppColors.Background
	bg.A = 255
	paint.Fill(gtx.Ops, bg)
}

// FetchAllRates initiates the concurrent fetching of exchange rates.
func FetchAllRates(window *app.Window, state *AppState, config AppConfig) {
	if config.APIRatesURL == "" {
		return
	}

	state.IsLoading.Store(true)
	state.IsLoadingStart = time.Now()

	// Reset animations for the selected currency so they "fly in" again
	state.Vault.Mu.Lock()
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		for _, entry := range currencyRates {
			entry.AppearanceSpring.Current = 0
			entry.AppearanceSpring.Velocity = 0
		}
	}
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
	days := timeframeToDays(state.UI.Timeframe)

	go performHistoryFetch(window, state, config, currency, cantorID, days)
}

// timeframeToDays converts a timeframe string to the corresponding number of days.
func timeframeToDays(tf string) int {
	switch tf {
	case "1D":
		return 1
	case "30D":
		return 30
	default:
		return 7
	}
}

// getSelectedCantorID retrieves the Cantor ID for the currently selected cantor or returns 0 if none is selected.
func getSelectedCantorID(state *AppState) int {
	if state.UI.SelectedCantor != "" {
		if c, ok := state.Cantors[state.UI.SelectedCantor]; ok {
			return c.ID
		}
	}
	return 0
}

// performHistoryFetch retrieves historical currency data from the API and updates the application's state with the response.
func performHistoryFetch(window *app.Window, state *AppState, config AppConfig, curr string, cID int, days int) {
	url := buildHistoryURL(config.APIHistoryURL, curr, cID, days)

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

// buildHistoryURL constructs the URL for fetching historical currency data with optional Cantor ID and timeframe.
func buildHistoryURL(baseURL, curr string, cID int, days int) string {
	url := fmt.Sprintf("%s?currency=%s&days=%d", baseURL, curr, days)
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

	paint.FillShape(gtx.Ops, AppColors.Dark, clip.Rect{Max: gtx.Constraints.Max}.Op())

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

				l := material.List(theme, list)
				l.Track.Color = color.NRGBA{A: 0}
				l.Indicator.Color = applyAlpha(AppColors.Secondary, 100)

				return l.Layout(gtx, len(state.UI.CurrencyOptions),
					func(gtx layout.Context, i int) layout.Dimensions {
						return layoutCurrencyItem(gtx, theme, state, window, config, i)
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

// layoutThemeButton renders the theme toggle button for switching between light and dark modes.
func layoutThemeButton(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if state.UI.ThemeButton.Clicked(gtx) {
		state.UI.LightMode = !state.UI.LightMode
		window.Invalidate()
	}

	return state.UI.ThemeButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(24))
		gtx.Constraints.Min = image.Point{X: size, Y: size}

		// Hover background
		if state.UI.ThemeButton.Hovered() {
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			paint.FillShape(gtx.Ops, applyAlpha(AppColors.Text, 20), clip.UniformRRect(rect, size/2).Op(gtx.Ops))
		}

		// Draw the icon
		center := float32(size) / 2
		radius := float32(size) / 2.5

		if state.UI.LightMode {
			// Light Mode: Full bright circle (Sun-ish)
			circle := clip.Ellipse{
				Min: image.Point{X: int(center - radius), Y: int(center - radius)},
				Max: image.Point{X: int(center + radius), Y: int(center + radius)},
			}.Op(gtx.Ops)
			paint.FillShape(gtx.Ops, AppColors.Accent1, circle)
		} else {
			// Dark Mode: Half-filled circle (Moon/Contrast-ish)
			// Full outline
			outline := clip.Stroke{
				Path: clip.Ellipse{
					Min: image.Point{X: int(center - radius), Y: int(center - radius)},
					Max: image.Point{X: int(center + radius), Y: int(center + radius)},
				}.Path(gtx.Ops),
				Width: 2.0,
			}.Op()
			paint.FillShape(gtx.Ops, AppColors.Text, outline)

			halfRect := image.Rect(int(center-radius), int(center-radius), int(center), int(center+radius))
			stack := clip.Rect(halfRect).Push(gtx.Ops)
			circle := clip.Ellipse{
				Min: image.Point{X: int(center - radius), Y: int(center - radius)},
				Max: image.Point{X: int(center + radius), Y: int(center + radius)},
			}.Op(gtx.Ops)
			paint.FillShape(gtx.Ops, AppColors.Text, circle)
			stack.Pop()
		}

		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
}

// layoutCurrencyItem renders a single currency selection button in the sidebar.
func layoutCurrencyItem(gtx layout.Context, theme *material.Theme, state *AppState, window *app.Window, config AppConfig, index int) layout.Dimensions {
	currency := state.UI.CurrencyOptions[index]
	btn := &state.UI.CurrencyOptionButtons[index]
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
				// Hover/Select Background
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
					// Accent Bar
					if isSelected {
						barRect := image.Rect(0, 12, 3, 38)
						paint.FillShape(gtx.Ops, AppColors.Accent1, clip.UniformRRect(barRect, 1).Op(gtx.Ops))
					}
					// Content
					return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(theme, currency)
							lbl.Color = txtColor
							lbl.Font.Weight = font.Bold
							lbl.TextSize = unit.Sp(13)
							return lbl.Layout(gtx)
						})
					})
				}),
			)
		})
	})
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
	currencyRates := state.Vault.Rates[state.UI.Currency]
	bestBuy, bestSell := calculateBestRates(currencyRates)

	currentSearch := state.UI.SearchEditor.Text()
	currentSort := state.UI.SortMode

	// Robust re-filter logic: filter if search text changed, sort mode changed, location active, or list empty
	if state.UI.SearchText != currentSearch || state.UI.LastSortMode != currentSort || state.UI.UserLocation.Active || len(state.UI.FilteredIDs) == 0 {
		state.UI.SearchText = currentSearch
		state.UI.LastSortMode = currentSort
		state.UI.FilteredIDs = filterCantorList(state, state.UI.SearchText)
	}
	filteredIDs := state.UI.FilteredIDs
	state.Vault.Mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// On mobile search, hide sort bar to save space
			if state.UI.IsMobile && state.UI.SearchActive {
				return layout.Dimensions{}
			}
			return layoutSortBar(gtx, theme, state)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
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

			l := material.List(theme, list)
			l.Track.Color = color.NRGBA{A: 0} // Transparent track
			l.Indicator.Color = applyAlpha(AppColors.Secondary, 100)

			cantorListWidget := l.Layout(gtx, len(filteredIDs),
				func(gtx layout.Context, i int) layout.Dimensions {
					rowCfg := CantorRowConfig{
						CantorID: filteredIDs[i],
						BestBuy:  bestBuy,
						BestSell: bestSell,
					}
					return layoutCantorItem(gtx, window, theme, state, config, rowCfg)
				})

			if !state.UI.IsMobile {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return cantorListWidget
				})
			}
			return cantorListWidget
		}),
	)
}

// layoutSortBar renders the sort bar for the cantor selection list.
func layoutSortBar(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(theme, GetTranslation(state.UI.Language, "sort_label"))
				lbl.Color = applyAlpha(AppColors.Text, 150)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSortButton(gtx, theme, state, 0, "NAME", GetTranslation(state.UI.Language, "sort_name"))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSortButton(gtx, theme, state, 1, "BUY", GetTranslation(state.UI.Language, "sort_buy"))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSortButton(gtx, theme, state, 2, "SELL", GetTranslation(state.UI.Language, "sort_sell"))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !state.UI.UserLocation.Active {
					return layout.Dimensions{}
				}
				return layoutSortButton(gtx, theme, state, 3, "DIST", GetTranslation(state.UI.Language, "sort_dist"))
			}),
		)
	})
}

// layoutSortButton renders a single sort button for the cantor selection list.
func layoutSortButton(gtx layout.Context, theme *material.Theme, state *AppState, idx int, mode, text string) layout.Dimensions {
	btn := &state.UI.SortButtons[idx]
	if btn.Clicked(gtx) {
		state.UI.SortMode = mode
	}

	active := state.UI.SortMode == mode

	return layoutToggleButton(gtx, theme, btn, text, active, func() {
		state.UI.SortMode = mode
	})
}

// layoutTopMovers renders the information bar showing top gainers and losers.
func layoutTopMovers(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	state.Vault.Mu.Lock()
	gainerID, loserID := calculateMoverCandidates(state)
	updateTopMoversState(state, gainerID, loserID)

	gainerName := state.UI.TopMovers.GainerID
	loserName := state.UI.TopMovers.LoserID

	currencyRates := state.Vault.Rates[state.UI.Currency]
	var topGainer, topLoser *CantorEntry
	if currencyRates != nil {
		topGainer = currencyRates[gainerName]
		topLoser = currencyRates[loserName]
	}

	if topGainer != nil {
		if UpdateSpring(&topGainer.AppearanceSpring, 0.016) {
			window.Invalidate()
		}
	}
	if topLoser != nil {
		if UpdateSpring(&topLoser.AppearanceSpring, 0.016) {
			window.Invalidate()
		}
	}
	state.Vault.Mu.Unlock()

	if topGainer == nil && topLoser == nil {
		return layout.Dimensions{}
	}

	gScale := float32(1.0)
	lScale := float32(1.0)
	if topGainer != nil {
		gScale = topGainer.AppearanceSpring.Current
	}
	if topLoser != nil {
		lScale = topLoser.AppearanceSpring.Current
	}

	return renderMovers(gtx, theme, state, gainerName, loserName, topGainer, topLoser, gScale, lScale)
}

// calculateMoverCandidates identifies the cantors with the largest 24h gain and loss.
func calculateMoverCandidates(state *AppState) (string, string) {
	var gainerID, loserID string
	maxGain := -999999.0
	maxLoss := 999999.0

	currencyRates := state.Vault.Rates[state.UI.Currency]
	for id, entry := range currencyRates {
		if entry == nil || entry.Rate.Change24h == 0 {
			continue
		}
		chg := entry.Rate.Change24h
		if chg > 0 && chg > maxGain {
			maxGain = chg
			gainerID = id
		}
		if chg < 0 && chg < maxLoss {
			maxLoss = chg
			loserID = id
		}
	}
	return gainerID, loserID
}

// updateTopMoversState updates the TopMovers state with stabilization/debounce logic.
func updateTopMoversState(state *AppState, gainerID, loserID string) {
	now := time.Now()
	if state.UI.TopMovers.LastUpdate.IsZero() || now.Sub(state.UI.TopMovers.LastUpdate) > 3*time.Second {
		changed := state.UI.TopMovers.GainerID != gainerID || state.UI.TopMovers.LoserID != loserID

		state.UI.TopMovers.GainerID = gainerID
		state.UI.TopMovers.LoserID = loserID
		state.UI.TopMovers.LastUpdate = now

		// Trigger animation if gainer/loser changed
		if changed {
			currencyRates := state.Vault.Rates[state.UI.Currency]
			if currencyRates != nil {
				if g, ok := currencyRates[gainerID]; ok {
					g.AppearanceSpring.Current = 0
					g.AppearanceSpring.Velocity = 0
				}
				if l, ok := currencyRates[loserID]; ok {
					l.AppearanceSpring.Current = 0
					l.AppearanceSpring.Velocity = 0
				}
			}
		}
	}
}

// renderMovers handles the layout for the top gainer and loser notches.
func renderMovers(gtx layout.Context, theme *material.Theme, state *AppState, gainerName, loserName string, topGainer, topLoser *CantorEntry, gScale, lScale float32) layout.Dimensions {
	resolveName := func(id string) string {
		if c, ok := state.Cantors[id]; ok {
			return GetTranslation(state.UI.Language, c.DisplayName)
		}
		return id
	}

	axis := layout.Horizontal
	spacerWidth := unit.Dp(6)
	spacerHeight := unit.Dp(0)
	if state.UI.IsMobile {
		spacerWidth = unit.Dp(4)
	}

	// Dynamic axis switching if space is tight
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(350)) {
		axis = layout.Vertical
		spacerWidth = unit.Dp(0)
		spacerHeight = unit.Dp(4)
	}

	return layout.Flex{Axis: axis, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if topGainer != nil {
				if gScale != 1.0 {
					offY := (1.0 - gScale) * 15
					op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(gScale, gScale)).Offset(f32.Pt(0, offY))).Add(gtx.Ops)
				}
				return layoutMoverNotch(gtx, theme, resolveName(gainerName), topGainer.Rate.Change24h, AppColors.Success)
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(layout.Spacer{Width: spacerWidth, Height: spacerHeight}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if topLoser != nil {
				if lScale != 1.0 {
					offY := (1.0 - lScale) * 15
					op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(lScale, lScale)).Offset(f32.Pt(0, offY))).Add(gtx.Ops)
				}
				return layoutMoverNotch(gtx, theme, resolveName(loserName), topLoser.Rate.Change24h, AppColors.Error)
			}
			return layout.Dimensions{}
		}),
	)
}

// layoutMoverNotch renders a single "Top Mover" notch.
func layoutMoverNotch(gtx layout.Context, theme *material.Theme, name string, change float64, col color.NRGBA) layout.Dimensions {
	fixedWidth := gtx.Dp(unit.Dp(170))
	fixedWidth = min(fixedWidth, gtx.Constraints.Max.X)
	gtx.Constraints.Min.X = fixedWidth
	gtx.Constraints.Max.X = fixedWidth

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			// Background
			bgColor := color.NRGBA{R: 35, G: 35, B: 40, A: 255}
			if AppColors.Background.R > 200 {
				bgColor = color.NRGBA{R: 0, G: 0, B: 0, A: 20}
			}
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(10))
			paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Content with tight padding
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, name)
						lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
						if AppColors.Background.R > 200 {
							lbl.Color = AppColors.Text
						}
						lbl.MaxLines = 1
						lbl.TextSize = unit.Sp(11)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(50))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(50))

						return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							txt := fmt.Sprintf("%+.2f%%", change)
							val := material.Caption(theme, txt)
							val.Color = col
							val.Font.Weight = font.Bold
							val.TextSize = unit.Sp(11)
							return val.Layout(gtx)
						})
					}),
				)
			})
		}),
	)
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

// filterCantorList filters and sorts a list of cantors based on search text, user location, and selected sort mode.
func filterCantorList(state *AppState, searchText string) []string {
	searchText = strings.ToLower(searchText)
	var ids []string
	for id, cantor := range state.Cantors {
		if !isWithinDistance(state, cantor) {
			continue
		}

		if matchesSearch(state, id, cantor, searchText) {
			ids = append(ids, id)
		}
	}

	sortCantorList(state, ids)
	return ids
}

// sortCantorList sorts a list of cantors based on the selected sort mode.
func sortCantorList(state *AppState, ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		idA, idB := ids[i], ids[j]
		cantorA, cantorB := state.Cantors[idA], state.Cantors[idB]

		mode := state.UI.SortMode
		if mode == "BUY" || mode == "SELL" {
			if done, result := compareRates(state, idA, idB, mode); done {
				return result
			}
		}

		if mode == "DIST" && state.UI.UserLocation.Active {
			if done, result := compareDistance(state, cantorA, cantorB); done {
				return result
			}
		}

		// Absolute fallback to name for perfect stability
		nameA := GetTranslation(state.UI.Language, cantorA.DisplayName)
		nameB := GetTranslation(state.UI.Language, cantorB.DisplayName)
		return strings.ToLower(nameA) < strings.ToLower(nameB)
	})
}

func compareRates(state *AppState, idA, idB, mode string) (bool, bool) {
	rateA := parseRateFromVault(state, idA, mode)
	rateB := parseRateFromVault(state, idB, mode)

	// Always push 0 rates (---) to the bottom
	if rateA == 0 && rateB > 0 {
		return true, false
	}
	if rateB == 0 && rateA > 0 {
		return true, true
	}

	if rateA != rateB {
		if mode == "BUY" {
			return true, rateA > rateB
		}
		return true, rateA < rateB
	}
	return false, false
}

// compareDistance compares the distances of two cantors from the user's location.
func compareDistance(state *AppState, cantorA, cantorB *CantorInfo) (bool, bool) {
	distA := CalculateDistance(state.UI.UserLocation.Latitude, state.UI.UserLocation.Longitude, cantorA.Latitude, cantorA.Longitude)
	distB := CalculateDistance(state.UI.UserLocation.Latitude, state.UI.UserLocation.Longitude, cantorB.Latitude, cantorB.Longitude)
	if distA != distB {
		return true, distA < distB
	}
	return false, false
}

// parseRateFromVault extracts and parses the buy or sell rate from the Vault for a given cantor ID and mode.
func parseRateFromVault(state *AppState, id string, mode string) float64 {
	// state.Vault.Mu.Lock() - assume called within lock if needed, or use a copy
	// For sorting we don't necessarily need a lock if we are okay with slightly stale data during sort
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		if entry, ok := currencyRates[id]; ok {
			return getRateFromEntry(entry, mode)
		}
	}
	return 0
}

// isWithinDistance checks if a cantor is within the user's specified maximum distance.
func isWithinDistance(state *AppState, cantor *CantorInfo) bool {
	if !state.UI.UserLocation.Active || state.UI.MaxDistance <= 0 {
		return true
	}
	dist := CalculateDistance(
		state.UI.UserLocation.Latitude,
		state.UI.UserLocation.Longitude,
		cantor.Latitude,
		cantor.Longitude,
	)
	return dist <= state.UI.MaxDistance
}

// matchesSearch determines if a cantor's ID or display name matches the search text.
func matchesSearch(state *AppState, id string, cantor *CantorInfo, searchText string) bool {
	if searchText == "" {
		return true
	}
	displayName := strings.ToLower(GetTranslation(state.UI.Language, cantor.DisplayName))
	return strings.Contains(strings.ToLower(id), searchText) ||
		strings.Contains(displayName, searchText)
}

// layoutCantorItem renders a single row representing a cantor item within the layout, including animations and interactions.
func layoutCantorItem(
	gtx layout.Context, window *app.Window, theme *material.Theme,
	state *AppState, config AppConfig, cfg CantorRowConfig) layout.Dimensions {
	cantorKey := cfg.CantorID
	cantor := state.Cantors[cantorKey]
	displayName := GetTranslation(state.UI.Language, cantor.DisplayName)

	state.Vault.Mu.Lock()
	var entry *CantorEntry
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		entry = currencyRates[cantorKey]
	}

	if entry != nil {
		if UpdateSpring(&entry.AppearanceSpring, 0.016) {
			window.Invalidate()
		}
		if entry.UpdatePulse > 0 {
			entry.UpdatePulse -= 0.03 // Adjust speed of fade-out
			if entry.UpdatePulse < 0 {
				entry.UpdatePulse = 0
			}
			window.Invalidate()
		}
	}
	state.Vault.Mu.Unlock()

	scale := float32(1.0)
	alpha := uint8(255)
	pulse := float32(0.0)
	if entry != nil {
		scale = entry.AppearanceSpring.Current
		alphaVal := entry.AppearanceSpring.Current * 255
		if alphaVal > 255 {
			alphaVal = 255
		}
		if alphaVal < 0 {
			alphaVal = 0
		}
		alpha = uint8(alphaVal)
		pulse = entry.UpdatePulse
	}

	buyVal, sellVal, spreadVal, buyColor, sellColor, change := getCantorDisplayData(state, entry, cfg.BestBuy, cfg.BestSell)
	buyColor.A = alpha
	sellColor.A = alpha

	handleCantorHover(window, state, cantor, cantorKey, displayName)
	handleCantorClick(gtx, window, state, config, cantorKey, cantor)
	handleCantorLongPress(window, state, cantor, cantorKey, displayName)

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
		IsMobile:    state.UI.IsMobile,
		Scale:       scale,
	}, pulse)
}

// handleCantorLongPress handles the long press event for mobile devices to show hover info.
func handleCantorLongPress(window *app.Window, state *AppState, cantor *CantorInfo, cantorKey, displayName string) {
	if !state.UI.IsMobile {
		return
	}

	if cantor.Button.Pressed() {
		if !cantor.IsPressing {
			cantor.IsPressing = true
			cantor.PressStart = time.Now()
			cantor.LongPressTriggered = false
			window.Invalidate()
		} else {
			if !cantor.LongPressTriggered && time.Since(cantor.PressStart).Seconds() >= 1.5 {
				cantor.LongPressTriggered = true
			}

			if cantor.LongPressTriggered {
				updateHoverInfoForCantor(window, state, cantor, cantorKey, displayName)
			} else {
				window.Invalidate()
			}
		}
	} else {
		resetCantorPressState(window, cantor)
	}
}

// updateHoverInfoForCantor updates the UI hover information for a cantor item.
func updateHoverInfoForCantor(window *app.Window, state *AppState, cantor *CantorInfo, cantorKey, displayName string) {
	address := getCantorAddress(state, cantor, cantorKey)
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

// resetCantorPressState resets the long press state for a cantor.
func resetCantorPressState(window *app.Window, cantor *CantorInfo) {
	if cantor.IsPressing {
		cantor.IsPressing = false
		cantor.LongPressTriggered = false
		window.Invalidate()
	}
}

// getCantorAddress retrieves the address for a cantor, falling back to a default translation if not provided.
func getCantorAddress(state *AppState, cantor *CantorInfo, cantorKey string) string {
	address := cantor.Address
	if address != "" {
		return address
	}

	switch strings.ToLower(cantorKey) {
	case "supersam":
		return "Adama Asnyka 12, 35-001 Rzeszów"
	case "tadek":
		return "Gen. Okulickiego 1b, 37-450 Stalowa Wola"
	case "exchange":
		return "Grottgera 20, 35-001 Rzeszów"
	case "grosz":
		return "Sławkowska 4, 31-014 Kraków"
	case "centrum":
		return "Świdnicka 3, 50-064 Wrocław"
	case "lider":
		return "Wolności 1, 41-800 Zabrze"
	case "baks":
		return "Marszałkowska 85, 00-683 Warszawa"
	case "waluciarz":
		return "Szewska 21, 31-009 Kraków"
	case "joker":
		return "Piłsudskiego 34, 35-001 Rzeszów"
	default:
		return GetTranslation(state.UI.Language, "location_unknown")
	}
}

// handleCantorHover updates the UI hover information based on cantor hover state, user location, and application language.
func handleCantorHover(window *app.Window, state *AppState, cantor *CantorInfo, cantorKey, displayName string) {
	// Disable hover on mobile (rely on Long Press)
	if state.UI.IsMobile {
		return
	}

	if cantor.Button.Hovered() {
		updateHoverInfoForCantor(window, state, cantor, cantorKey, displayName)
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

// renderCantorItem renders a single interactive UI element to represent a cantor, including labels, values, and highlight state.
func renderCantorItem(gtx layout.Context, theme *material.Theme, state *AppState, args CantorItemArgs, pulse float32) layout.Dimensions {
	bottom := unit.Dp(8)
	if args.IsMobile && state.UI.SearchActive {
		bottom = unit.Dp(4)
	}

	return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Apply Spring Transform
		if args.Scale != 1.0 {
			// Offset based on scale to give a "lifting" effect
			offsetY := (1.0 - args.Scale) * 20
			trans := f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(args.Scale, args.Scale)).Offset(f32.Pt(0, offsetY))
			op.Affine(trans).Add(gtx.Ops)
		}

		return args.Cantor.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return drawCantorItemBackground(gtx, theme, state, args, pulse)
		})
	})
}

// drawCantorItemBackground renders the background for a cantor item row, including a rounded rectangle shape.
func drawCantorItemBackground(gtx layout.Context, theme *material.Theme, state *AppState, args CantorItemArgs, pulse float32) layout.Dimensions {
	cornerRadius := unit.Dp(16) // Slightly smaller radius like in the image
	pxRadius := gtx.Dp(cornerRadius)

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, pxRadius)

			// 1. Ultra-thin Background (Glass)
			var bgColor color.NRGBA
			if state.UI.LightMode {
				// Light Mode: White background with high alpha (milky glass)
				bgAlpha := uint8(200)
				if args.IsSelected {
					bgAlpha = 240
				}
				bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: bgAlpha}
			} else {
				// Dark Mode: Black background with low alpha (dark glass)
				bgAlpha := uint8(40)
				if args.IsSelected {
					bgAlpha = 80
				}
				bgColor = color.NRGBA{R: 0, G: 0, B: 0, A: bgAlpha}
			}

			paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))

			// 2. Bright Glass Border
			borderColor := color.NRGBA{R: 255, G: 255, B: 255, A: 45}
			if args.IsSelected {
				borderColor = AppColors.Accent1
				borderColor.A = 120
			} else if state.UI.LightMode {
				borderColor = color.NRGBA{R: 0, G: 0, B: 0, A: 30}
			}

			widget.Border{
				Color:        borderColor,
				Width:        unit.Dp(1.2),
				CornerRadius: cornerRadius,
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Min}
			})

			// 3. Pulse Flash Overlay
			if pulse > 0 {
				flashCol := AppColors.Accent1
				flashCol.A = uint8(pulse * 60)
				paint.FillShape(gtx.Ops, flashCol, shape.Op(gtx.Ops))
			}

			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			inset := unit.Dp(12)
			if args.IsMobile && state.UI.SearchActive {
				inset = unit.Dp(8)
			}
			return layout.UniformInset(inset).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return renderCantorRowContent(gtx, theme, state, args)
			})
		}),
	)
}

// getCantorItemBackgroundColor returns the background color for a cantor row based on selection or hover state.
func getCantorItemBackgroundColor(args CantorItemArgs) color.NRGBA {
	if args.IsSelected {
		col := AppColors.Accent1
		col.A = 40
		if !args.IsMobile && AppColors.Background.R < 100 {
			return color.NRGBA{R: 90, G: 65, B: 25, A: 255}
		}
		if AppColors.Background.R < 100 {
			return color.NRGBA{R: 90, G: 65, B: 25, A: 255}
		}
		return col
	} else if args.Cantor.Button.Hovered() {
		return applyAlpha(AppColors.Secondary, 30)
	}
	return applyAlpha(AppColors.Dark, 120)
}

// renderCantorRowContent lays out the content for a cantor item row, including labels, values, and highlight state.
func renderCantorRowContent(gtx layout.Context, theme *material.Theme, state *AppState, args CantorItemArgs) layout.Dimensions {
	displayChange := ""
	state.Vault.Mu.Lock()
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		if entry, ok := currencyRates[args.Cantor.DisplayName]; ok {
			displayChange = entry.DisplayChange
		}
	}
	state.Vault.Mu.Unlock()

	// If search is active on mobile, use a specialized vertical layout that removes labels to save space
	if args.IsMobile && state.UI.SearchActive {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(theme, args.DisplayName)
				lbl.Color = AppColors.Text
				lbl.TextSize = unit.Sp(16)
				lbl.Font.Weight = font.Bold
				lbl.MaxLines = 1
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, lbl.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutMarketValue(gtx, theme, MarketValueArgs{
							Label:         "", // NO LABEL in search mode
							Value:         args.BuyVal,
							Color:         args.BuyColor,
							Change:        args.Change24h,
							DisplayChange: displayChange,
							IsMobile:      true,
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutMarketValue(gtx, theme, MarketValueArgs{
							Label:    "", // NO LABEL
							Value:    args.SellVal,
							Color:    args.SellColor,
							IsMobile: true,
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layoutMarketValue(gtx, theme, MarketValueArgs{
								Label:    "", // NO LABEL
								Value:    args.SpreadVal,
								Color:    AppColors.Spread,
								IsMobile: true,
							})
						})
					}),
				)
			}),
		)
	}

	// Default Horizontal Layout (Non-search or Desktop)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body1(theme, args.DisplayName)
			lbl.Color = AppColors.Text
			if args.IsMobile {
				lbl.TextSize = unit.Sp(14)
			}
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			spacing := unit.Dp(20)
			if args.IsMobile {
				spacing = unit.Dp(8)
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:         GetTranslation(state.UI.Language, "buy_col"),
						Value:         args.BuyVal,
						Color:         args.BuyColor,
						Change:        args.Change24h,
						DisplayChange: displayChange,
						IsMobile:      args.IsMobile,
					})
				}),
				layout.Rigid(layout.Spacer{Width: spacing}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:    GetTranslation(state.UI.Language, "sell_col"),
						Value:    args.SellVal,
						Color:    args.SellColor,
						IsMobile: args.IsMobile,
					})
				}),
				layout.Rigid(layout.Spacer{Width: spacing}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:    GetTranslation(state.UI.Language, "spread_col"),
						Value:    args.SpreadVal,
						Color:    AppColors.Spread,
						IsMobile: args.IsMobile,
					})
				}),
			)
		}),
	)
}

// layoutMarketValue lays out a market value displaying a label and its corresponding value with customizable text color.
func layoutMarketValue(gtx layout.Context, theme *material.Theme, args MarketValueArgs) layout.Dimensions {
	valSize := unit.Sp(18)
	lblSize := unit.Sp(12)

	if args.IsMobile {
		valSize = unit.Sp(16)
		lblSize = unit.Sp(11)
	}

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if args.Label == "" {
				return layout.Dimensions{}
			}
			l := material.Caption(theme, args.Label)
			l.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
			l.TextSize = lblSize
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					v := material.Body2(theme, args.Value)
					v.Color = args.Color
					v.TextSize = valSize
					v.Font.Typeface = "Kanit"
					v.Font.Weight = font.Bold
					return v.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var col color.NRGBA
					txt := args.DisplayChange

					switch {
					case args.Change == 0:
						return layout.Dimensions{}
					case args.Change < 0:
						col = AppColors.Error
					default:
						col = AppColors.Success
					}

					c := material.Caption(theme, txt)
					c.Color = col
					c.TextSize = unit.Sp(9)
					if args.IsMobile {
						c.TextSize = unit.Sp(8)
					}
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, c.Layout)
				}),
			)
		}),
	)
}

// UpdateSpring calculates the next value for a spring animation based on elapsed time.
func UpdateSpring(s *Spring, dt float32) bool {
	if s.Tension <= 0 {
		s.Tension = 150
	}
	if s.Friction <= 0 {
		s.Friction = 22
	}

	// Calculate spring force: F = k * x (where x is distance to target)
	dist := s.Target - s.Current
	force := dist * s.Tension

	// Calculate damping: F = -b * v (where v is velocity)
	damping := s.Velocity * s.Friction

	// Acceleration = Force - Damping (assuming mass = 1)
	acceleration := force - damping

	// Update velocity and position
	s.Velocity += acceleration * dt
	s.Current += s.Velocity * dt

	// Return true if still moving
	isMoving := math.Abs(float64(s.Velocity)) > 0.01 || math.Abs(float64(s.Target-s.Current)) > 0.01
	if !isMoving {
		s.Current = s.Target
		s.Velocity = 0
	}
	return isMoving
}

// RefreshDisplayStrings pre-formats display strings for a cantor entry to save GC cycles during layout.
func RefreshDisplayStrings(entry *CantorEntry) {
	if entry == nil {
		return
	}

	buy := parseRate(entry.Rate.BuyRate)
	sell := parseRate(entry.Rate.SellRate)

	entry.DisplayBuy = "---"
	if buy > 0 {
		entry.DisplayBuy = fmt.Sprintf("%.3f zł", buy)
	}

	entry.DisplaySell = "---"
	if sell > 0 {
		entry.DisplaySell = fmt.Sprintf("%.3f zł", sell)
	}

	entry.DisplaySpread = "---"
	if buy > 0 && sell > 0 {
		entry.DisplaySpread = fmt.Sprintf("%.3f", sell-buy)
	}

	if entry.Rate.Change24h == 0 {
		entry.DisplayChange = ""
	} else {
		entry.DisplayChange = fmt.Sprintf("%+.2f%%", entry.Rate.Change24h)
	}
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

	// Safety: ensure strings are not empty (e.g. after fresh load from cache)
	if entry.DisplayBuy == "" {
		RefreshDisplayStrings(entry)
	}

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

	return entry.DisplayBuy, entry.DisplaySell, entry.DisplaySpread, buyColor, sellColor, entry.Rate.Change24h
}

// layoutHeader renders the application's header, including the market title, subtitle, and a language selection button.
func layoutHeader(gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {

	// On mobile, hide header if search is active to provide space and stability
	if state.UI.IsMobile && state.UI.SearchActive {
		return layout.Dimensions{}
	}

	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutMobileMenuButton(gtx, window, state)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return renderHeaderMainSection(gtx, window, theme, state)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.UI.IsMobile {
					return layout.Dimensions{}
				}
				return layoutThemeButton(gtx, window, state)
			}),
		)
	})
}

func renderHeaderMainSection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
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
					return layoutStatusIndicator(gtx, window, state)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return renderHeaderSearchToggle(gtx, window, state)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			caption := material.Caption(theme, GetTranslation(state.UI.Language, "market_subtitle"))
			caption.Color = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
			return caption.Layout(gtx)
		}),
	)
}

func renderHeaderSearchToggle(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if !state.UI.IsMobile {
		return layout.Dimensions{}
	}
	if state.UI.SearchClickable.Clicked(gtx) {
		state.UI.SearchActive = !state.UI.SearchActive
		window.Invalidate()
	}
	return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return state.UI.SearchClickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			col := AppColors.Text
			if state.UI.SearchActive {
				col = AppColors.Accent1
			}
			return DrawIconSearch(gtx, col)
		})
	})
}

// layoutMobileMenuButton renders the hamburger menu button for mobile layout.
func layoutMobileMenuButton(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if !state.UI.IsMobile {
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
}

// layoutStatusIndicator renders the connection status dot and handles its tooltip.
func layoutStatusIndicator(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
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
}

// layoutLanguageButton renders the language selection button (mini version for sidebar).
func layoutLanguageButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.LangModalButton.Clicked(gtx) {
		state.UI.ModalOpen = "language"
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
	if state.UI.IsMobile && !state.UI.SearchActive {
		return layout.Dimensions{}
	}

	bottomInset := unit.Dp(20)
	if state.UI.IsMobile {
		bottomInset = unit.Dp(10)
	}

	return layout.Inset{Bottom: bottomInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		state.UI.SearchText = state.UI.SearchEditor.Text()
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !state.UI.IsMobile {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(400))
				}
				return drawSearchInput(gtx, theme, state, window)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.UI.IsMobile {
					return layout.Dimensions{}
				}
				return drawSearchLocateButton(gtx, window, theme, state)
			}),
		)
	})
}

// drawSearchInput renders the search input field with a background and border.
func drawSearchInput(gtx layout.Context, theme *material.Theme, state *AppState, window *app.Window) layout.Dimensions {
	return layout.Stack{Alignment: layout.E}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bgColor := color.NRGBA{R: 25, G: 25, B: 30, A: 200}
			if state.UI.LightMode {
				bgColor = color.NRGBA{R: 0, G: 0, B: 0, A: 10}
			}
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(14))
			paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))

			borderColor := color.NRGBA{R: 255, G: 255, B: 255, A: 20}
			if state.UI.LightMode {
				borderColor = color.NRGBA{R: 0, G: 0, B: 0, A: 30}
			}

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
			inset := unit.Dp(16)
			if state.UI.IsMobile {
				inset = unit.Dp(12)
			}
			return layout.Inset{Top: inset, Bottom: inset, Left: inset, Right: unit.Dp(45)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				hint := GetTranslation(state.UI.Language, "search_placeholder")
				ed := material.Editor(theme, &state.UI.SearchEditor, hint)
				ed.Color = AppColors.Text
				ed.HintColor = applyAlpha(AppColors.Text, 120)
				ed.TextSize = unit.Sp(16)
				return ed.Layout(gtx)
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !state.UI.IsMobile {
				return layout.Dimensions{}
			}

			if state.UI.SearchClickable.Clicked(gtx) {
				state.UI.SearchActive = false
				state.UI.SearchEditor.SetText("")
				window.Invalidate()
			}

			return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return state.UI.SearchClickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return DrawIconClose(gtx, AppColors.Text)
				})
			})
		}),
	)
}

// drawSearchLocateButton renders the Locate button for desktop layout.
func drawSearchLocateButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutLocateButton(gtx, window, theme, state)
	})
}

// layoutLocateButton renders the Locate button, handles its click events, and dynamically updates the UI state.
func layoutLocateButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	handleLocateClick(gtx, window, state)
	handleLocateHover(window, state)

	btnText := GetTranslation(state.UI.Language, "locate_button")
	btn := material.Button(theme, &state.UI.LocateButton, btnText)

	btnBg := color.NRGBA{R: 255, G: 255, B: 255, A: 10}
	if state.UI.LightMode {
		btnBg = color.NRGBA{R: 0, G: 0, B: 0, A: 25}
	}
	btn.Background = btnBg
	btn.Color = AppColors.Accent1
	if state.UI.LightMode {
		btn.Color = color.NRGBA{R: 160, G: 110, B: 0, A: 255} // Darker yellow for text on light bg
	}
	btn.CornerRadius = unit.Dp(8)
	btn.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}
	btn.TextSize = unit.Sp(14)

	if state.UI.UserLocation.Active {
		return layoutLocateActiveState(gtx, window, theme, state, btn)
	}

	return btn.Layout(gtx)
}

// handleLocateClick handles the click event for the Locate button, fetching and updating user location.
func handleLocateClick(gtx layout.Context, window *app.Window, state *AppState) {
	if state.UI.LocateButton.Clicked(gtx) {
		if !state.UI.UserLocation.Active {
			state.UI.UserLocation.Active = true
			state.UI.FilteredIDs = nil // Force re-filter

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

// handleLocateHover updates the hover-related UI state when the Locate button is hovered or long-pressed on mobile.
func handleLocateHover(window *app.Window, state *AppState) {
	isHovered := state.UI.LocateButton.Hovered()
	isPressed := state.UI.LocateButton.Pressed()

	if state.UI.IsMobile {
		if isPressed {
			if !state.UI.LocateIsPressing {
				state.UI.LocateIsPressing = true
				state.UI.LocatePressStart = time.Now()
				state.UI.LocateLongPressTriggered = false
			} else if !state.UI.LocateLongPressTriggered && time.Since(state.UI.LocatePressStart).Seconds() >= 1.5 {
				state.UI.LocateLongPressTriggered = true
			}

			if state.UI.LocateLongPressTriggered {
				state.UI.HoverInfo = HoverInfo{
					Active:   true,
					Title:    GetTranslation(state.UI.Language, "notch_loc_title"),
					Subtitle: GetTranslation(state.UI.Language, "notch_loc_desc"),
					Extra:    "GPS",
				}
			}
			window.Invalidate()
		} else {
			state.UI.LocateIsPressing = false
			state.UI.LocateLongPressTriggered = false
		}
	} else if isHovered {
		state.UI.HoverInfo = HoverInfo{
			Active:   true,
			Title:    GetTranslation(state.UI.Language, "notch_loc_title"),
			Subtitle: GetTranslation(state.UI.Language, "notch_loc_desc"),
			Extra:    "GPS",
		}
		window.Invalidate()
	}
}

// layoutLocateActiveState renders the active state of the Locate button, including a distance slider and label.
func layoutLocateActiveState(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, btn material.ButtonStyle) layout.Dimensions {
	btn.Background = AppColors.Accent1
	btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	newVal := float64(state.UI.DistanceSlider.Value) * 100.0
	if newVal != state.UI.MaxDistance {
		state.UI.MaxDistance = newVal
		window.Invalidate()
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return btn.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(60))
			slider := material.Slider(theme, &state.UI.DistanceSlider)
			slider.Color = AppColors.Accent1
			return slider.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(45))
			label := material.Caption(theme, fmt.Sprintf("%.0fkm", state.UI.MaxDistance))
			label.Color = AppColors.Text
			return layout.Center.Layout(gtx, label.Layout)
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
						stack := clip.UniformRRect(rect, rr).Op(gtx.Ops).Push(gtx.Ops)

						// Gradient Background
						c1, c2 := AppColors.Dark, AppColors.Background
						c1.A, c2.A = 255, 255
						paint.LinearGradientOp{
							Stop1:  f32.Point{X: 0, Y: 0},
							Stop2:  f32.Point{X: 0, Y: float32(rect.Max.Y)},
							Color1: c1,
							Color2: c2,
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

// renderLanguageItem renders a language selection button in the modal dialog.
func renderLanguageItem(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, lang string, btnWidget *widget.Clickable) layout.Dimensions {
	if btnWidget.Clicked(gtx) {
		state.UI.Language = lang
		state.UI.ModalOpen = ""
		window.Invalidate()
	}

	isSelected := state.UI.Language == lang

	// Premium Card Look
	bgColor := AppColors.Button
	if isSelected {
		bgColor = AppColors.Accent1
	} else if btnWidget.Hovered() {
		bgColor = applyAlpha(AppColors.Secondary, 50)
	}

	return btnWidget.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(12))
				paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))

				// Subtle Border for all cards, stronger for selected
				borderAlpha := uint8(30)
				if isSelected {
					borderAlpha = 150
				}
				borderColor := applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, borderAlpha)
				if state.UI.LightMode && !isSelected {
					borderColor = applyAlpha(color.NRGBA{R: 0, G: 0, B: 0, A: 255}, 20)
				}

				return widget.Border{Color: borderColor, Width: unit.Dp(1), CornerRadius: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				})
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(theme, lang)
					lbl.Color = AppColors.Text
					if isSelected {
						lbl.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

// LanguageModal renders a modal for selecting a language using a modern grid-based card layout.
func LanguageModal(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	title := GetTranslation(state.UI.Language, "select_lang_title")

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layoutModalHeader(window, theme, title, state),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutLanguageGrid(gtx, window, theme, state)
		}),
	)
}

// layoutLanguageGrid renders a grid of language selection buttons in a modal dialog.
func layoutLanguageGrid(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	state.UI.ModalList.Axis = layout.Vertical
	options := state.UI.LanguageOptions
	buttons := state.UI.LanguageOptionButtons

	cols := 3
	rows := (len(options) + cols - 1) / cols

	return material.List(theme, &state.UI.ModalList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			func() []layout.FlexChild {
				children := make([]layout.FlexChild, cols)
				for c := range cols {
					idx := row*cols + c
					if idx < len(options) {
						children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return renderLanguageItem(gtx, window, theme, state, options[idx], &buttons[idx])
							})
						})
					} else {
						children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
					}
				}
				return children
			}()...,
		)
	})
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
				btn.Color = AppColors.Text
				if state.UI.LightMode {
					btn.Color = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
				}
				btn.Inset = layout.UniformInset(unit.Dp(12))
				btn.TextSize = unit.Sp(18)
				return layout.Inset{Right: unit.Dp(8), Top: unit.Dp(8)}.Layout(gtx, btn.Layout)
			}),
		)
	})
}

// LoadFontCollection loads a collection of font faces from predefined font files and returns them.
func LoadFontCollection() ([]font.FontFace, error) {
	var collection []font.FontFace

	var f1 font.FontFace
	var err error

	if runtime.GOOS == "linux" {
		// Use Kanit-Regular instead of Montserrat-Bold on Linux to avoid artifacts
		f1, err = reading_data.LoadAndParseFont("fonts/Kanit-Regular.ttf")
	} else {
		f1, err = reading_data.LoadAndParseFont("fonts/Montserrat-Bold.ttf")
	}

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
	if alphaVal <= 0.001 {
		return layout.Dimensions{}
	}

	info := state.UI.NotchState.LastContent

	// Allow it to grow up to available space to prevent "..."
	gtx.Constraints.Min.X = 0
	return renderNotchContent(gtx, theme, info, alphaVal, state)
}

// renderNotchContent renders the main content of the notch, including a title and subtitle.
func renderNotchContent(gtx layout.Context, theme *material.Theme, info HoverInfo, alpha float32, state *AppState) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return renderNotchExtra(gtx, theme, info.Extra, alpha)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				titleCol := color.NRGBA{R: 255, G: 255, B: 255, A: uint8(255 * alpha)}
				subCol := color.NRGBA{R: 200, G: 200, B: 210, A: uint8(255 * alpha)}

				if AppColors.Background.R > 200 {
					titleCol = AppColors.Text
					titleCol.A = uint8(255 * alpha)
					subCol = color.NRGBA{R: 80, G: 80, B: 90, A: uint8(255 * alpha)}
				}

				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(theme, info.Title)
						lbl.Color = titleCol
						lbl.Font.Weight = font.Bold
						lbl.MaxLines = 1
						lbl.TextSize = unit.Sp(11) // Match Mover text size
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if info.Subtitle == "" {
							return layout.Dimensions{}
						}
						lbl := material.Caption(theme, info.Subtitle)
						lbl.Color = subCol
						lbl.MaxLines = 1
						lbl.TextSize = unit.Sp(10)
						return lbl.Layout(gtx)
					}),
				)
			}),
		)
	})
}

// renderNotchExtra renders an extra text element within the notch, with a background and rounded corners.
func renderNotchExtra(gtx layout.Context, theme *material.Theme, extra string, alpha float32) layout.Dimensions {
	if extra == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		m := op.Record(gtx.Ops)
		lbl := material.Caption(theme, extra)
		lbl.Color = color.NRGBA{R: 0, G: 0, B: 0, A: uint8(255 * alpha)}
		lbl.Font.Weight = font.Bold
		lbl.TextSize = unit.Sp(10)
		d := layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, lbl.Layout)
		c := m.Stop()

		rr := gtx.Dp(6)
		bg := AppColors.Accent1
		bg.A = uint8(255 * alpha)
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: d.Size}, rr).Op(gtx.Ops))

		c.Add(gtx.Ops)
		return d
	})
}

// drawSmoothTrend draws a mathematically generated smooth trend line.
func drawSmoothTrend(gtx layout.Context, width, height int, col color.NRGBA, progress float32, isBearish bool) layout.Dimensions {
	if progress > 1.0 {
		progress = 1.0
	}

	// Clip based on progress (reveal from left to right)
	revealWidth := int(float32(width) * progress)

	// Anchoring: we keep the path at its full size but reveal it using clip.Rect
	defer clip.Rect{Max: image.Point{X: revealWidth, Y: height}}.Push(gtx.Ops).Pop()

	var path clip.Path
	path.Begin(gtx.Ops)

	steps := 100
	getVal := func(x float64) float64 {
		trend := math.Pow(x, 1.2)
		volatility := 0.08 * math.Sin(x*12.0) * math.Sin(x*3.14)
		y := trend + volatility

		if y < 0 {
			y = 0
		}
		if y > 1 {
			y = 1
		}

		if isBearish {
			return y
		}
		return 1.0 - y
	}

	startY := getVal(0)
	path.MoveTo(f32.Point{X: 0, Y: float32(startY) * float32(height)})

	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		val := getVal(t)

		px := float32(t) * float32(width)
		py := float32(val) * float32(height)
		path.LineTo(f32.Point{X: px, Y: py})
	}

	// Draw ONLY the main line (outline) on top, no fill
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: path.End(), Width: 3.0}.Op())

	return layout.Dimensions{Size: image.Point{X: width, Y: height}}
}

// layoutIntroAnimation renders a full-screen intro with the trend line animation.
func layoutIntroAnimation(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if !state.UI.IntroAnim.Active {
		return layout.Dimensions{}
	}

	elapsed := time.Since(state.UI.IntroAnim.StartTime).Seconds()
	duration := 2.2     // Total duration including fade
	fadeStart := 1.7    // Start fading out at this point
	animDuration := 1.5 // Time for the line reveal

	if elapsed > duration {
		state.UI.IntroAnim.Active = false
		window.Invalidate()
		return layout.Dimensions{}
	}

	window.Invalidate()

	// Calculate overall opacity for the fade out
	opacity := float32(1.0)
	if elapsed > fadeStart {
		opacity = 1.0 - float32((elapsed-fadeStart)/(duration-fadeStart))
	}

	// Background
	bg := AppColors.Background
	bg.A = uint8(255 * opacity)
	paint.Fill(gtx.Ops, bg)

	// Reveal progress
	progress := float32(elapsed / animDuration)
	if progress > 1.0 {
		progress = 1.0
	}

	lineCol := AppColors.Accent1
	lineCol.A = uint8(255 * opacity)

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Use a large portion of the screen
		w := int(float32(gtx.Constraints.Max.X) * 0.8)
		h := int(float32(gtx.Constraints.Max.Y) * 0.6)

		h = min(h, w)

		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return drawSmoothTrend(gtx, w, h, lineCol, progress, false)
			}),
		)
	})
}
