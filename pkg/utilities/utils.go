package utilities

import (
	// Standard libraries
	"encoding/json"
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
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
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
	LightMode     bool
	PulseBest     bool // best rate in list — soft Accent1 pulse
}

// CantorItemArgs holds arguments for rendering a cantor item row.
type CantorItemArgs struct {
	Cantor        *CantorInfo
	CantorKey     string
	DisplayName   string
	BuyVal        string
	SellVal       string
	SpreadVal     string
	BuyColor      color.NRGBA
	SellColor     color.NRGBA
	Change24h     float64
	IsSelected    bool
	IsMobile      bool
	LightMode     bool
	Scale         float32
	BestBuyPulse  bool
	BestSellPulse bool
}

// updateLoadingAlpha animates the global loading overlay alpha (fade in/out).
func updateLoadingAlpha(window *app.Window, state *AppState) {
	targetAlpha := float32(0.0)
	if state.IsLoading.Load() {
		targetAlpha = 1.0
	}
	// Smooth fade — slower out so the loop doesn't "snap" away when harvest ends.
	if state.LoadingAlpha == targetAlpha {
		return
	}
	dt := float32(0.016)
	speed := float32(1.35)
	if targetAlpha > state.LoadingAlpha {
		speed = float32(2.4) // appear a bit faster than fade-out
	}
	diff := targetAlpha - state.LoadingAlpha
	if math.Abs(float64(diff)) < 0.008 {
		state.LoadingAlpha = targetAlpha
	} else {
		state.LoadingAlpha += diff * speed * dt
	}
	window.Invalidate()
}

// applyPalette switches the global palette based on LightMode.
func applyPalette(state *AppState) {
	if state.UI.LightMode {
		AppColors = LightPalette
	} else {
		AppColors = DarkPalette
	}
}

// LayoutUI - Main application layout
func LayoutUI(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) {
	updateNotchAnimation(window, state)

	// Update Loading Alpha (Fade in/out)
	updateLoadingAlpha(window, state)

	// Switch palette based on LightMode
	applyPalette(state)

	state.UI.SearchEditor.SingleLine = true
	state.UI.SearchEditor.Submit = true

	state.Vault.Mu.Lock()
	if state.Vault.Rates == nil {
		state.Vault.Rates = make(map[string]map[string]*CantorEntry)
	}
	state.Vault.Mu.Unlock()

	drawPatternBackground(gtx)

	minW, minH := gtx.Dp(unit.Dp(360)), gtx.Dp(unit.Dp(420))
	tooSmall := gtx.Constraints.Max.X < minW || gtx.Constraints.Max.Y < minH

	layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if tooSmall {
				return layoutWindowTooSmall(gtx, theme, state)
			}
			return layoutContent(gtx, window, theme, state, config)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if tooSmall {
				return layout.Dimensions{}
			}
			return layoutModal(gtx, window, theme, state)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if tooSmall || state.LoadingAlpha <= 0 {
				return layout.Dimensions{}
			}
			return layoutGlobalLoading(gtx, window, state)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if tooSmall {
				return layout.Dimensions{}
			}
			return layoutIntroAnimation(gtx, window, state)
		}),
	)
}

// layoutWindowTooSmall blocks unreadable layouts when the window is extremely small.
func layoutWindowTooSmall(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	bg := color.NRGBA{R: 18, G: 18, B: 22, A: 255}
	if state.UI.LightMode {
		bg = color.NRGBA{R: 245, G: 245, B: 248, A: 255}
	}
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					t := material.H6(theme, GetTranslation(state.UI.Language, "window_too_small_title"))
					t.Color = AppColors.Accent1
					t.Font.Weight = font.Bold
					t.Alignment = text.Middle
					return t.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Body1(theme, GetTranslation(state.UI.Language, "window_too_small_body"))
					b.Color = AppColors.Text
					b.Alignment = text.Middle
					return b.Layout(gtx)
				}),
			)
		})
	})
}

// StartFinOpsMonitoring periodically fetches metrics from the server for the economic dashboard.
func StartFinOpsMonitoring(window *app.Window, state *AppState, config AppConfig) {
	ticker := time.NewTicker(15 * time.Second)
	client := &http.Client{Timeout: 5 * time.Second}

	var failLogged bool
	fetch := func() {
		resp, err := client.Get(config.APIFinOpsURL)
		if err != nil {
			if !failLogged {
				log.Printf("[FinOps] waiting for backend: %v", err)
				failLogged = true
			}
			return
		}
		defer func() { _ = resp.Body.Close() }()

		var status FinOpsStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			log.Printf("[FinOps] Error decoding metrics: %v", err)
			return
		}

		failLogged = false
		state.SetFinOpsStatus(status)
		window.Invalidate()
	}

	go func() {
		// Brief grace period so GUI can start before/with backend.
		for i := 0; i < 6; i++ {
			fetch()
			if !failLogged {
				break
			}
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
		}
		for range ticker.C {
			fetch()
		}
	}()
}

// layoutControlPanelOverlay — full-screen Control Panel for mobile / mid (no right rail).
func layoutControlPanelOverlay(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if !state.UI.IsFinOpsDashboardOpen {
		return layout.Dimensions{}
	}
	bg := AppColors.Background
	bg.A = 255
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layoutFinOpsDashboard(gtx, window, theme, state)
}

// layoutFinOpsDashboard renders the Control Panel (desktop rail or mobile overlay).
func layoutFinOpsDashboard(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	finOps := state.FinOpsSnapshot()
	lang := state.UI.Language
	compact := state.UI.CurrentViewMode != ViewDesktop
	pad := unit.Dp(20)
	if compact {
		pad = unit.Dp(14)
	}
	tabs := []string{
		GetTranslation(lang, "cp_tab_finops"),
		GetTranslation(lang, "cp_tab_scraper"),
		GetTranslation(lang, "cp_tab_providers"),
		GetTranslation(lang, "cp_tab_geo"),
		GetTranslation(lang, "cp_tab_debug"),
	}
	return layout.Inset{Top: pad, Left: pad, Right: pad, Bottom: pad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.H6(theme, GetTranslation(lang, "control_panel"))
						lbl.Color = AppColors.Accent1
						lbl.Font.Weight = font.Bold
						if compact {
							lbl.TextSize = unit.Sp(18)
						}
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if state.UI.FinOpsBtn.Clicked(gtx) {
							state.UI.IsFinOpsDashboardOpen = false
							window.Invalidate()
						}
						return state.UI.FinOpsBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								bgCol := color.NRGBA{R: 255, G: 50, B: 50, A: 40}
								if state.UI.FinOpsBtn.Hovered() {
									bgCol = color.NRGBA{R: 255, G: 50, B: 50, A: 80}
								}
								rect := image.Rect(0, 0, gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
								paint.FillShape(gtx.Ops, bgCol, clip.UniformRRect(rect, 6).Op(gtx.Ops))
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return DrawIconClose(gtx, color.NRGBA{R: 255, G: 150, B: 150, A: 255})
								})
							})
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutControlPanelTabs(gtx, window, theme, state, tabs, compact)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				body := func(gtx layout.Context) layout.Dimensions {
					switch state.UI.ControlPanelTab {
					case 1:
						return layoutScraperTab(gtx, theme, state, window, lang)
					case 2:
						return layoutProvidersTab(gtx, theme, state, lang)
					case 3:
						return layoutGeoTab(gtx, theme, state, window, lang)
					case 4:
						return layoutDebugTab(gtx, theme, state, lang)
					default:
						return layoutFinOpsTab(gtx, theme, finOps, lang)
					}
				}
				if !compact {
					return body(gtx)
				}
				state.UI.ControlPanelList.Axis = layout.Vertical
				return material.List(theme, &state.UI.ControlPanelList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					return body(gtx)
				})
			}),
		)
	})
}

// controlPanelTabColors returns background and text colors for a control panel tab chip.
func controlPanelTabColors(state *AppState, idx int) (bgCol, txtCol color.NRGBA) {
	bgCol = color.NRGBA{R: 40, G: 40, B: 50, A: 255}
	txtCol = color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	if state.UI.LightMode {
		bgCol = color.NRGBA{R: 0, G: 0, B: 0, A: 18}
		txtCol = color.NRGBA{R: 80, G: 80, B: 90, A: 255}
	}
	if state.UI.ControlPanelTab == idx {
		bgCol = AppColors.Accent1
		txtCol = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return bgCol, txtCol
}

// controlPanelTabLabel shortens long tab labels in compact mode.
func controlPanelTabLabel(tabName string, compact bool) string {
	if compact && len([]rune(tabName)) > 8 {
		return string([]rune(tabName)[:7]) + "…"
	}
	return tabName
}

// layoutControlPanelTab renders a single control panel tab chip.
func layoutControlPanelTab(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, idx int, tabName string, compact bool) layout.Dimensions {
	bgCol, txtCol := controlPanelTabColors(state, idx)
	if idx >= len(state.UI.ControlPanelTabs) {
		return layout.Dimensions{}
	}
	if state.UI.ControlPanelTabs[idx].Clicked(gtx) {
		state.UI.ControlPanelTab = idx
		window.Invalidate()
	}
	label := controlPanelTabLabel(tabName, compact)
	return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return state.UI.ControlPanelTabs[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Constraints.Min
					paint.FillShape(gtx.Ops, bgCol, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(6)).Op(gtx.Ops))
					return layout.Dimensions{Size: sz}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, label)
						lbl.Color = txtCol
						lbl.Font.Weight = font.Bold
						lbl.TextSize = unit.Sp(11)
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					})
				}),
			)
		})
	})
}

func layoutControlPanelTabs(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, tabs []string, compact bool) layout.Dimensions {
	// Rigid chips (not Flexed) so long labels like „Dostawcy” never wrap into a tall tower.
	buildTab := func(idx int, tabName string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutControlPanelTab(gtx, window, theme, state, idx, tabName, compact)
		})
	}

	wrapRow := func(from, to int) layout.Dimensions {
		items := make([]layout.FlexChild, 0, to-from)
		for i := from; i < to && i < len(tabs); i++ {
			items = append(items, buildTab(i, tabs[i]))
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, items...)
	}

	if !compact {
		return wrapRow(0, len(tabs))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return wrapRow(0, 3) }),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return wrapRow(3, len(tabs)) }),
	)
}

func formatFinOpsUSD(raw string) string {
	if raw == "" {
		return "$0.00"
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return "$" + raw
	}
	return fmt.Sprintf("$%.4f", f)
}

func finOpsCategoryLabel(lang, key string) string {
	switch key {
	case "AI":
		return GetTranslation(lang, "cp_cat_ai")
	case "Compute":
		return GetTranslation(lang, "cp_cat_compute")
	case "Network":
		return GetTranslation(lang, "cp_cat_network")
	case "Other":
		return GetTranslation(lang, "cp_cat_other")
	default:
		return key
	}
}

func layoutFinOpsTab(gtx layout.Context, theme *material.Theme, finOps FinOpsStatus, lang string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.H4(theme, formatFinOpsUSD(finOps.DailySpendUSD))
			lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, GetTranslation(lang, "cp_burn_rate"))
			lbl.Color = color.NRGBA{R: 150, G: 150, B: 160, A: 255}
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(30)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			t := finOps.SystemTime
			if t == "" {
				t = time.Now().Format("15:04:05")
			}
			statusText := GetTranslation(lang, "cp_status_healthy")
			if len(finOps.BlockedProviders) > 0 {
				statusText = fmt.Sprintf("%s (%d)", GetTranslation(lang, "cp_status_throttled"), len(finOps.BlockedProviders))
			}
			details := fmt.Sprintf("%s: %s\n%s: %v\n%s: %s",
				GetTranslation(lang, "cp_server_time"), t,
				GetTranslation(lang, "cp_governance"), finOps.IsActive,
				GetTranslation(lang, "cp_breaker"), statusText)
			lbl := material.Body2(theme, details)
			lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(finOps.ServiceBreakdown) == 0 {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(theme, GetTranslation(lang, "cp_focus_categories"))
					lbl.Color = color.NRGBA{R: 0, G: 220, B: 150, A: 255}
					lbl.Font.Weight = font.Bold
					return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, lbl.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var widgets []layout.FlexChild
					keys := []string{"AI", "Compute", "Network", "Other"}
					for _, k := range keys {
						if val, ok := finOps.ServiceBreakdown[k]; ok {
							cat := finOpsCategoryLabel(lang, k)
							cost := formatFinOpsUSD(val)
							widgets = append(widgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(theme, cat)
										lbl.Color = color.NRGBA{R: 180, G: 180, B: 190, A: 255}
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body2(theme, cost)
										lbl.Color = color.NRGBA{R: 250, G: 250, B: 250, A: 255}
										return lbl.Layout(gtx)
									}),
								)
							}))
							widgets = append(widgets, layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout))
						}
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, widgets...)
				}),
			)
		}),
	)
}

func layoutScraperTab(gtx layout.Context, theme *material.Theme, state *AppState, window *app.Window, lang string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, GetTranslation(lang, "cp_scraper_config"))
			lbl.Color = color.NRGBA{R: 0, G: 220, B: 150, A: 255}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, GetTranslation(lang, "cp_update_interval"))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
					e := material.Editor(theme, &state.UI.ScraperIntervalEditor, "5000")
					e.Color = color.NRGBA{R: 250, G: 250, B: 250, A: 255}
					e.HintColor = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
					return e.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, GetTranslation(lang, "cp_timeout"))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
					e := material.Editor(theme, &state.UI.ScraperTimeoutEditor, "15000")
					e.Color = color.NRGBA{R: 250, G: 250, B: 250, A: 255}
					e.HintColor = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
					return e.Layout(gtx)
				}),
			)
		}),
	)
}

func layoutProvidersTab(gtx layout.Context, theme *material.Theme, state *AppState, lang string) layout.Dimensions {
	finOps := state.FinOpsSnapshot()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, GetTranslation(lang, "cp_provider_mgmt"))
			lbl.Color = color.NRGBA{R: 0, G: 220, B: 150, A: 255}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(finOps.BlockedProviders) == 0 {
				lbl := material.Body2(theme, GetTranslation(lang, "cp_providers_healthy"))
				lbl.Color = color.NRGBA{R: 100, G: 200, B: 100, A: 255}
				return lbl.Layout(gtx)
			}
			lbl := material.Body2(theme, GetTranslation(lang, "cp_providers_blocked"))
			lbl.Color = color.NRGBA{R: 200, G: 100, B: 100, A: 255}
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var items []layout.FlexChild
			for _, p := range finOps.BlockedProviders {
				prov := p
				items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(theme, "• "+prov)
						lbl.Color = color.NRGBA{R: 255, G: 150, B: 150, A: 255}
						return lbl.Layout(gtx)
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		}),
	)
}

func layoutGeoTab(gtx layout.Context, theme *material.Theme, state *AppState, window *app.Window, lang string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, GetTranslation(lang, "cp_geo_settings"))
			lbl.Color = color.NRGBA{R: 0, G: 220, B: 150, A: 255}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, GetTranslation(lang, "cp_search_radius"))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(120))
					e := material.Editor(theme, &state.UI.GeoRadiusEditor, "30")
					e.Color = color.NRGBA{R: 250, G: 250, B: 250, A: 255}
					e.HintColor = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
					return e.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, GetTranslation(lang, "cp_map_tiles"))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(theme, GetTranslation(lang, "cp_map_tiles_hint"))
					lbl.Color = color.NRGBA{R: 140, G: 140, B: 150, A: 255}
					return lbl.Layout(gtx)
				}),
			)
		}),
	)
}

func layoutDebugTab(gtx layout.Context, theme *material.Theme, state *AppState, lang string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, GetTranslation(lang, "cp_debug_console"))
			lbl.Color = color.NRGBA{R: 0, G: 220, B: 150, A: 255}
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, fmt.Sprintf("%s: %s", GetTranslation(lang, "cp_dbg_language"), state.UI.Language))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, fmt.Sprintf("%s: %s", GetTranslation(lang, "cp_dbg_currency"), state.UI.Currency))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, fmt.Sprintf("%s: %v", GetTranslation(lang, "cp_dbg_light"), state.UI.LightMode))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, fmt.Sprintf("%s: %v", GetTranslation(lang, "cp_dbg_finops"), state.UI.IsFinOpsDashboardOpen))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					status := GetTranslation(lang, "cp_dbg_offline")
					if state.IsConnected.Load() {
						status = GetTranslation(lang, "cp_dbg_connected")
					}
					lbl := material.Body2(theme, fmt.Sprintf("%s: %s", GetTranslation(lang, "cp_dbg_backend"), status))
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					return lbl.Layout(gtx)
				}),
			)
		}),
	)
}

// layoutContent lays out the main content panel of the application.
func layoutContent(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	var currentView ViewMode
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(1000)) {
		currentView = ViewMobile
	} else if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(1350)) {
		currentView = ViewMid
	} else {
		currentView = ViewDesktop
	}

	// Smooth Transition Logic
	if state.UI.CurrentViewMode != currentView {
		state.UI.CurrentViewMode = currentView
		state.UI.IsMobile = (currentView == ViewMobile)
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
	switch currentView {
	case ViewMobile:
		dims = layoutContentMobile(gtx, window, theme, state, config)
	case ViewMid:
		dims = layout.Stack{Alignment: layout.NW}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return LayoutVerticalCurrencyBar(gtx, window, theme, state, config)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layoutCenterPanel(gtx, window, theme, state, config)
					}),
				)
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layoutControlPanelOverlay(gtx, window, theme, state)
			}),
		)
	default:
		// Desktop
		dims = layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return LayoutVerticalCurrencyBar(gtx, window, theme, state, config)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutMapSection(gtx, window, theme, state)
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
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layoutControlPanelOverlay(gtx, window, theme, state)
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
				return LayoutSearchBar(gtx, window, theme, state, config)
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

// layoutMobileControls renders the chart and location controls for mobile and mid devices.
func layoutMobileControls(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	if state.UI.CurrentViewMode == ViewDesktop {
		return layout.Dimensions{}
	}

	if state.UI.MapBtnMobile.Clicked(gtx) {
		state.UI.MapVisibleMobile = !state.UI.MapVisibleMobile
		window.Invalidate()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.NE}.Layout(gtx,
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if state.UI.MapVisibleMobile {
						gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(250))
						gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(250))

						rect := image.Rect(0, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
						defer clip.UniformRRect(rect, gtx.Dp(16)).Push(gtx.Ops).Pop()

						return LayoutMap(gtx, window, state)
					} else {
						return layoutChartSection(gtx, window, theme, state, config)
					}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return state.UI.MapBtnMobile.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return renderMapToggleBtn(gtx, state)
						})
					})
				}),
			)
		}),
	)
}

func renderMapToggleBtn(gtx layout.Context, state *AppState) layout.Dimensions {
	bgCol := color.NRGBA{R: 30, G: 30, B: 40, A: 200}
	iconCol := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if state.UI.LightMode {
		bgCol = color.NRGBA{R: 255, G: 255, B: 255, A: 220}
		iconCol = color.NRGBA{R: 50, G: 50, B: 60, A: 255}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(20)) // Fully rounded
			paint.FillShape(gtx.Ops, bgCol, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if state.UI.MapVisibleMobile {
					// Draw Chart Icon
					return DrawIconChart(gtx, iconCol)
				}
				// Draw Map Icon
				return DrawIconMap(gtx, iconCol)
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
						return LayoutSearchBar(gtx, window, theme, state, config)
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

// layoutMapSection renders the interactive map panel.
func layoutMapSection(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	panelWidth := gtx.Dp(unit.Dp(350))
	gtx.Constraints.Min.X = panelWidth
	gtx.Constraints.Max.X = panelWidth

	// Draw Background
	paint.FillShape(gtx.Ops, AppColors.Dark, clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			// Container for Map and Geocoder UI
			return layout.Stack{Alignment: layout.NW}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return LayoutMap(gtx, window, state)
				}),
			)
		}),
	)
}

// LayoutRightPanel lays out the right-side panel of the application.
func LayoutRightPanel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	panelWidth := gtx.Dp(unit.Dp(400))
	gtx.Constraints.Min.X = panelWidth
	gtx.Constraints.Max.X = panelWidth
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, AppColors.Dark, clip.Rect{Max: gtx.Constraints.Max}.Op())

	if state.UI.IsFinOpsDashboardOpen {
		return layoutFinOpsDashboard(gtx, window, theme, state)
	}

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
				layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layoutAnalysisInsights(gtx, theme, state)
				}),
			)
		})
}

// layoutInsightRow renders one label/value row of the analysis insights panel.
func layoutInsightRow(gtx layout.Context, theme *material.Theme, state *AppState, muted color.NRGBA, label, value string, accent bool) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				l := material.Caption(theme, label)
				l.Color = muted
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				v := material.Body2(theme, value)
				v.Color = insightValueColor(state, accent)
				v.Font.Weight = font.Bold
				return v.Layout(gtx)
			}),
		)
	})
}

// insightValueColor picks the value color of an insight row (accent rows follow the theme).
func insightValueColor(state *AppState, accent bool) color.NRGBA {
	if !accent {
		return AppColors.Text
	}
	if state.UI.LightMode {
		return AppColors.Accent1Dark
	}
	return AppColors.Accent1
}

// formatInsightPrice formats a positive PLN price, or an em dash when unavailable.
func formatInsightPrice(value float64, format string) string {
	if value > 0 {
		return fmt.Sprintf(format, value)
	}
	return "—"
}

// layoutAnalysisInsights fills the empty space under the chart with live market stats.
func layoutAnalysisInsights(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	lang := state.UI.Language
	bestBuy, bestSell, avgSpread, nRates, change24 := computeMarketSnapshot(state)
	histN := 0
	if h := state.HistorySnapshot(); h != nil {
		histN = len(h.Points)
	}
	days := state.UI.HistoryDays
	if days <= 0 {
		days = timeframeToDays(state.UI.Timeframe)
	}

	muted := color.NRGBA{R: 120, G: 120, B: 130, A: 255}
	if state.UI.LightMode {
		muted = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
	}

	row := func(label, value string, accent bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutInsightRow(gtx, theme, state, muted, label, value, accent)
		})
	}

	bestBuyTxt := formatInsightPrice(bestBuy, "%.3f zł")
	bestSellTxt := formatInsightPrice(bestSell, "%.3f zł")
	spreadTxt := formatInsightPrice(avgSpread, "%.3f")
	changeTxt := "—"
	if change24 != 0 {
		changeTxt = fmt.Sprintf("%+.2f%%", change24)
	}
	coverage := GetTranslation(lang, "insight_hist_ok")
	if histN < 3 {
		coverage = GetTranslation(lang, "insight_hist_sparse")
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := material.Caption(theme, GetTranslation(lang, "insight_title"))
			h.Color = AppColors.Accent1
			h.Font.Weight = font.Bold
			return h.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		row(GetTranslation(lang, "insight_best_buy"), bestBuyTxt, true),
		row(GetTranslation(lang, "insight_best_sell"), bestSellTxt, true),
		row(GetTranslation(lang, "insight_avg_spread"), spreadTxt, false),
		row(GetTranslation(lang, "insight_change24"), changeTxt, false),
		row(fmt.Sprintf(GetTranslation(lang, "insight_quotes"), state.UI.Currency), fmt.Sprintf("%d", nRates), false),
		row(fmt.Sprintf(GetTranslation(lang, "insight_hist_points"), days), fmt.Sprintf("%d", histN), false),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			coverageLbl := material.Caption(theme, coverage)
			coverageLbl.Color = muted
			coverageLbl.MaxLines = 4
			return coverageLbl.Layout(gtx)
		}),
	)
}

// validMarketRate returns the parsed buy/sell pair of a rate entry when both sides are usable.
func validMarketRate(e *CantorEntry) (buy, sell float64, ok bool) {
	if e == nil || e.Error != "" {
		return 0, 0, false
	}
	buy = parseRate(e.Rate.BuyRate)
	sell = parseRate(e.Rate.SellRate)
	if buy <= 0 || sell <= 0 {
		return 0, 0, false
	}
	return buy, sell, true
}

// selectedCantorChange24 prefers the selected cantor's 24h change when one is selected and has data.
func selectedCantorChange24(selected string, rates map[string]*CantorEntry, current float64) float64 {
	if selected == "" {
		return current
	}
	if e, ok := rates[selected]; ok && e != nil {
		return e.Rate.Change24h
	}
	return current
}

func computeMarketSnapshot(state *AppState) (bestBuy, bestSell, avgSpread float64, n int, change24 float64) {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()
	rates := state.Vault.Rates[state.UI.Currency]
	if rates == nil {
		return
	}
	var sumSpread float64
	var spreadN int
	bestSell = 0
	for _, e := range rates {
		b, s, ok := validMarketRate(e)
		if !ok {
			continue
		}
		n++
		if b > bestBuy {
			bestBuy = b
		}
		if bestSell == 0 || s < bestSell {
			bestSell = s
		}
		sumSpread += s - b
		spreadN++
		if e.Rate.Change24h != 0 && change24 == 0 {
			change24 = e.Rate.Change24h
		}
	}
	change24 = selectedCantorChange24(state.UI.SelectedCantor, rates, change24)
	if spreadN > 0 {
		avgSpread = sumSpread / float64(spreadN)
	}
	return
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
					days := state.UI.HistoryDays
					if days <= 0 {
						days = timeframeToDays(state.UI.Timeframe)
					}
					winEnd := time.Now().Unix()
					winStart := winEnd - int64(days)*86400
					chart := LineChart{
						Data:        data,
						Timestamps:  timestamps,
						WindowStart: winStart,
						WindowEnd:   winEnd,
						StartLabel:  startLabel,
						Tag:         &state.UI.ChartHoverTag,
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

// layoutGlobalLoading — simple dim overlay (no decorative animation).
func layoutGlobalLoading(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	overlay := color.NRGBA{R: 12, G: 12, B: 14, A: uint8(180 * state.LoadingAlpha)}
	if state.UI.LightMode {
		overlay = color.NRGBA{R: 245, G: 245, B: 248, A: uint8(180 * state.LoadingAlpha)}
	}
	paint.FillShape(gtx.Ops, overlay, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// prepareChartData processes historical or generated data points for chart rendering, returning the data and a start label.
func prepareChartData(state *AppState) ([]float64, []int64, string) {
	if history := state.HistorySnapshot(); history != nil && len(history.Points) > 1 {
		return extractHistoryData(history, state.UI.ChartMode)
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
	for _, p := range history.Points {
		val := float64(p.BuyRate) / 1000.0
		if mode == "SELL" {
			val = float64(p.SellRate) / 1000.0
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

	currency := state.UI.Currency
	go FetchAllRatesRPC(window, state, config.DRPCServerURL, currency)
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
	state.UI.HistoryDays = days

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
		state.CantorsMu.RLock()
		defer state.CantorsMu.RUnlock()
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

	state.SetHistory(&history)
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
	currentSearch := state.UI.SearchEditor.Text()
	currentSort := state.UI.SortMode

	// Re-filter when search/sort change or list was invalidated (FilteredIDs = nil).
	// Also refresh when empty so newly pinned OSM/discovered rows appear without a manual nudge.
	if state.UI.FilteredIDs == nil || state.UI.SearchText != currentSearch || state.UI.LastSortMode != currentSort || len(state.UI.FilteredIDs) == 0 {
		state.UI.SearchText = currentSearch
		state.UI.LastSortMode = currentSort
		state.UI.FilteredIDs = filterCantorList(state, state.UI.SearchText)
	}
	filteredIDs := state.UI.FilteredIDs

	// Best rates among the *visible* list (not distant vault entries).
	state.Vault.Mu.Lock()
	currencyRates := state.Vault.Rates[state.UI.Currency]
	bestBuy, bestSell := calculateBestRatesAmong(currencyRates, filteredIDs)
	state.Vault.Mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// On mobile search, hide sort bar to save space
			if state.UI.IsMobile && state.UI.SearchActive {
				return layout.Dimensions{}
			}
			return layoutSortBar(gtx, window, theme, state, config)
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
func layoutSortBar(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// Keep sort chips in a wrapping-safe horizontal strip; never starve the delete button.
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, GetTranslation(state.UI.Language, "sort_label"))
						lbl.Color = applyAlpha(AppColors.Text, 150)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutSortButton(gtx, theme, state, 0, "NAME", GetTranslation(state.UI.Language, "sort_name"))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutSortButton(gtx, theme, state, 1, "BUY", GetTranslation(state.UI.Language, "sort_buy"))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutSortButton(gtx, theme, state, 2, "SELL", GetTranslation(state.UI.Language, "sort_sell"))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !state.UI.UserLocation.Active {
							return layout.Dimensions{}
						}
						return layoutSortButton(gtx, theme, state, 3, "DIST", GetTranslation(state.UI.Language, "sort_dist"))
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				state.CantorsMu.RLock()
				n := len(state.Cantors)
				state.CantorsMu.RUnlock()
				if n == 0 {
					return layout.Dimensions{}
				}
				if state.UI.DeleteAllButton.Clicked(gtx) {
					go TriggerDeleteAllCantors(state, config, window)
				}
				label := GetTranslation(state.UI.Language, "delete_all_button")
				if state.UI.IsMobile || gtx.Constraints.Max.X < gtx.Dp(unit.Dp(520)) {
					// Short label avoids vertical letter-stack when width is tight.
					label = GetTranslation(state.UI.Language, "delete_all_short")
				}
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return state.UI.DeleteAllButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{Alignment: layout.Center}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Constraints.Min
								bg := color.NRGBA{R: 160, G: 40, B: 40, A: 220}
								paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(6)).Op(gtx.Ops))
								return layout.Dimensions{Size: sz}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Caption(theme, label)
									lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									lbl.TextSize = unit.Sp(11)
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								})
							}),
						)
					})
				})
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
		state.CantorsMu.RLock()
		defer state.CantorsMu.RUnlock()
		if c, ok := state.Cantors[id]; ok {
			return c.DisplayName
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
	ids := make([]string, 0, len(rates))
	for id := range rates {
		ids = append(ids, id)
	}
	return calculateBestRatesAmong(rates, ids)
}

// calculateBestRatesAmong picks best kupno (max) / sprzedaż (min) among the given cantor IDs.
func calculateBestRatesAmong(rates map[string]*CantorEntry, ids []string) (float64, float64) {
	bestBuy := -1.0
	bestSell := 999999.0
	if rates == nil {
		return bestBuy, bestSell
	}
	for _, id := range ids {
		entry := rates[id]
		if entry == nil || entry.Error != "" {
			continue
		}
		buy := parseRate(entry.Rate.BuyRate)
		sell := parseRate(entry.Rate.SellRate)
		if buy > bestBuy {
			bestBuy = buy
		}
		if sell > 0 && sell < bestSell {
			bestSell = sell
		}
	}
	return bestBuy, bestSell
}

// hasUsableCantorRates — true only when buy or sell is a real number (hide empty OSM pins).
func hasUsableCantorRates(state *AppState, id string) bool {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()
	rates, ok := state.Vault.Rates[state.UI.Currency]
	if !ok {
		return false
	}
	entry := rates[id]
	if entry == nil || entry.Error != "" {
		return false
	}
	return parseRate(entry.Rate.BuyRate) > 0 || parseRate(entry.Rate.SellRate) > 0
}

func isOSMDiscoveryPin(id string, cantor *CantorInfo) bool {
	if cantor == nil {
		return false
	}
	if strings.EqualFold(cantor.Strategy, "OSM") {
		return true
	}
	return strings.HasPrefix(id, "osm_")
}

// localizeCantorDisplayName rewrites legacy "Kantor · …" prefixes for the active UI language.
func localizeCantorDisplayName(lang, name string) string {
	name = strings.TrimSpace(name)
	for _, p := range []string{"Kantor · ", "Kantor - ", "Kantor – "} {
		if strings.HasPrefix(name, p) {
			place := strings.TrimSpace(strings.TrimPrefix(name, p))
			if place == "" {
				return name
			}
			return fmt.Sprintf(GetTranslation(lang, "cantor_unnamed_fmt"), place)
		}
	}
	return name
}

// filterCantorList filters and sorts a list of cantors based on search text, user location, and selected sort mode.
func filterCantorList(state *AppState, searchText string) []string {
	searchText = strings.ToLower(searchText)
	var ids []string

	snapshots := make(map[string]struct {
		DisplayName string
		Latitude    float64
		Longitude   float64
	})

	state.CantorsMu.RLock()
	for id, cantor := range state.Cantors {
		if cantor == nil || !isWithinDistance(state, cantor) {
			continue
		}
		// OSM city-discovery pins must show before FX scrape fills rates.
		if !hasUsableCantorRates(state, id) && !isOSMDiscoveryPin(id, cantor) {
			continue
		}
		if matchesSearch(state, id, cantor, searchText) {
			ids = append(ids, id)
			snapshots[id] = struct {
				DisplayName string
				Latitude    float64
				Longitude   float64
			}{
				DisplayName: localizeCantorDisplayName(state.UI.Language, cantor.DisplayName),
				Latitude:    cantor.Latitude,
				Longitude:   cantor.Longitude,
			}
		}
	}
	state.CantorsMu.RUnlock()

	sortCantorList(state, ids, snapshots)
	return ids
}

// sortCantorList sorts a list of cantors based on the selected sort mode.
func sortCantorList(state *AppState, ids []string, snapshots map[string]struct {
	DisplayName string
	Latitude    float64
	Longitude   float64
}) {
	sort.SliceStable(ids, func(i, j int) bool {
		idA, idB := ids[i], ids[j]
		cantorA, cantorB := snapshots[idA], snapshots[idB]

		mode := state.UI.SortMode
		if mode == "BUY" || mode == "SELL" {
			if done, result := compareRates(state, idA, idB, mode); done {
				return result
			}
		}

		if mode == "DIST" {
			if done, result := compareDistanceSnapshot(state, cantorA.Latitude, cantorA.Longitude, cantorB.Latitude, cantorB.Longitude); done {
				return result
			}
		}

		// Absolute fallback to name for perfect stability
		nameA := cantorA.DisplayName
		nameB := cantorB.DisplayName
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

// compareDistance compares distances from the best available origin (user GPS, else city focus).
func compareDistanceSnapshot(state *AppState, latA, lonA, latB, lonB float64) (bool, bool) {
	olat, olon, ok := distanceOrigin(state)
	if !ok {
		return false, false
	}
	latA, lonA = NormalizeLatLon(latA, lonA)
	latB, lonB = NormalizeLatLon(latB, lonB)
	distA := CalculateDistance(olat, olon, latA, lonA)
	distB := CalculateDistance(olat, olon, latB, lonB)
	if distA != distB {
		return true, distA < distB
	}
	return false, false
}

func distanceOrigin(state *AppState) (lat, lon float64, ok bool) {
	// City browse: distance from the searched city (not a distant IP/GPS fix).
	flat, flon := NormalizeLatLon(state.UI.MapFocus.Latitude, state.UI.MapFocus.Longitude)
	if state.UI.MapFocus.CityName != "" && (flat != 0 || flon != 0) {
		return flat, flon, true
	}
	ulat, ulon := NormalizeLatLon(state.UI.UserLocation.Latitude, state.UI.UserLocation.Longitude)
	if ulat != 0 || ulon != 0 {
		return ulat, ulon, true
	}
	return 0, 0, false
}

// parseRateFromVault extracts and parses the buy or sell rate from the Vault for a given cantor ID and mode.
func parseRateFromVault(state *AppState, id string, mode string) float64 {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		if entry, ok := currencyRates[id]; ok {
			return getRateFromEntry(entry, mode)
		}
	}
	return 0
}

// isWithinDistance checks if a cantor is within the user's specified maximum distance.
func isWithinDistance(state *AppState, cantor *CantorInfo) bool {
	// Optional GPS radius (slider removed from UI — MaxDistance stays 0 unless set programmatically).
	if state.UI.UserLocation.Active && state.UI.MaxDistance > 0 {
		olat, olon := NormalizeLatLon(state.UI.UserLocation.Latitude, state.UI.UserLocation.Longitude)
		clat, clon := NormalizeLatLon(cantor.Latitude, cantor.Longitude)
		dist := CalculateDistance(olat, olon, clat, clon)
		return dist <= state.UI.MaxDistance
	}

	// City search: radius around the geocoded city (MapFocus), NEVER map pan / GPS center.
	// Using MapState.Center made Kraków results vanish when Locate/startup recentered the map.
	if state.UI.MapFocus.CityName != "" && state.UI.MapFocus.Latitude != 0 && state.UI.MapFocus.Longitude != 0 {
		olat, olon := NormalizeLatLon(state.UI.MapFocus.Latitude, state.UI.MapFocus.Longitude)
		clat, clon := NormalizeLatLon(cantor.Latitude, cantor.Longitude)
		dist := CalculateDistance(olat, olon, clat, clon)
		return dist <= cityDiscoverRadiusKm
	}

	return true
}

// matchesSearch — id, display name, or address/city (so locality names hit OSM pins).
func matchesSearch(state *AppState, id string, cantor *CantorInfo, searchText string) bool {
	if searchText == "" {
		return true
	}
	if strings.Contains(strings.ToLower(id), searchText) {
		return true
	}
	if strings.Contains(strings.ToLower(cantor.DisplayName), searchText) {
		return true
	}
	addr := strings.ToLower(strings.TrimSpace(cantor.Address))
	if addr == "" || addr == "discovered automatically" || addr == "location_unknown" {
		return false
	}
	return strings.Contains(addr, searchText)
}

// stepCantorEntryAnimation advances the appearance spring and update pulse of a rate entry.
func stepCantorEntryAnimation(window *app.Window, entry *CantorEntry) {
	if entry == nil {
		return
	}
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

// cantorEntryVisuals derives scale, alpha and pulse of a cantor row from its animation state.
func cantorEntryVisuals(entry *CantorEntry) (scale float32, alpha uint8, pulse float32) {
	if entry == nil {
		return 1.0, 255, 0.0
	}
	alphaVal := entry.AppearanceSpring.Current * 255
	if alphaVal > 255 {
		alphaVal = 255
	}
	if alphaVal < 0 {
		alphaVal = 0
	}
	return entry.AppearanceSpring.Current, uint8(alphaVal), entry.UpdatePulse
}

// layoutCantorItem renders a single row representing a cantor item within the layout, including animations and interactions.
func layoutCantorItem(
	gtx layout.Context, window *app.Window, theme *material.Theme,
	state *AppState, config AppConfig, cfg CantorRowConfig) layout.Dimensions {
	cantorKey := cfg.CantorID
	state.CantorsMu.RLock()
	cantor, ok := state.Cantors[cantorKey]
	state.CantorsMu.RUnlock()
	if !ok || cantor == nil {
		return layout.Dimensions{}
	}
	displayName := localizeCantorDisplayName(state.UI.Language, cantor.DisplayName)

	state.Vault.Mu.Lock()
	var entry *CantorEntry
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		entry = currencyRates[cantorKey]
	}

	stepCantorEntryAnimation(window, entry)
	state.Vault.Mu.Unlock()

	scale, alpha, pulse := cantorEntryVisuals(entry)

	buyVal, sellVal, spreadVal, buyColor, sellColor, change, bestBuy, bestSell := getCantorDisplayData(state, entry, cfg.BestBuy, cfg.BestSell)
	buyColor.A = alpha
	sellColor.A = alpha
	if bestBuy || bestSell {
		// Keep the best-rate pulse animating while visible.
		window.Invalidate()
	}

	handleCantorHover(window, state, cantor, cantorKey, displayName)
	handleCantorClick(gtx, window, state, config, cantorKey, cantor)
	handleCantorLongPress(window, state, cantor, cantorKey, displayName)

	isSelected := state.UI.SelectedCantor == cantorKey
	return renderCantorItem(gtx, window, theme, state, config, CantorItemArgs{
		Cantor:        cantor,
		CantorKey:     cantorKey,
		DisplayName:   displayName,
		BuyVal:        buyVal,
		SellVal:       sellVal,
		SpreadVal:     spreadVal,
		BuyColor:      buyColor,
		SellColor:     sellColor,
		Change24h:     change,
		IsSelected:    isSelected,
		IsMobile:      state.UI.CurrentViewMode != ViewDesktop,
		LightMode:     state.UI.LightMode,
		Scale:         scale,
		BestBuyPulse:  bestBuy,
		BestSellPulse: bestSell,
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
	if d, ok := cantorDistanceKm(state, cantor); ok {
		distance = fmt.Sprintf("%.1f km", d)
	}

	state.UI.HoverInfo = HoverInfo{
		Active:   true,
		Title:    displayName,
		Subtitle: address,
		Extra:    distance,
	}
	window.Invalidate()
}

// cantorDistanceKm — city focus while browsing a search; otherwise user GPS/IP.
func cantorDistanceKm(state *AppState, cantor *CantorInfo) (float64, bool) {
	if cantor == nil {
		return 0, false
	}
	clat, clon := NormalizeLatLon(cantor.Latitude, cantor.Longitude)
	if clat == 0 && clon == 0 {
		return 0, false
	}
	olat, olon, ok := distanceOrigin(state)
	if !ok {
		return 0, false
	}
	return CalculateDistance(olat, olon, clat, clon), true
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
	if cantor.Address != "" && cantor.Address != "Discovered automatically" {
		return cantor.Address
	}

	return GetTranslation(state.UI.Language, "location_unknown")
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
			// Center the map on the selected cantor
			if cantor.Latitude != 0 && cantor.Longitude != 0 {
				state.UI.MapFocus.Latitude = cantor.Latitude
				state.UI.MapFocus.Longitude = cantor.Longitude
				window.Invalidate() // Force UI redraw for map update
			}
		}
		state.ChartAnimStart = time.Now()
		fetchHistory(window, state, config)
	}
}

// renderCantorItem renders a single interactive UI element to represent a cantor, including labels, values, and highlight state.
func renderCantorItem(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig, args CantorItemArgs, pulse float32) layout.Dimensions {
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

		return drawCantorItemBackground(gtx, window, theme, state, config, args, pulse)
	})
}

// drawCantorItemBackground renders the background for a cantor item row, including a rounded rectangle shape.
func drawCantorItemBackground(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig, args CantorItemArgs, pulse float32) layout.Dimensions {
	cornerRadius := unit.Dp(16) // Slightly smaller radius like in the image
	pxRadius := gtx.Dp(cornerRadius)

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return args.Cantor.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			inset := unit.Dp(12)
			if args.IsMobile && state.UI.SearchActive {
				inset = unit.Dp(8)
			}
			return layout.UniformInset(inset).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return renderCantorRowContent(gtx, window, theme, state, config, args)
			})
		}),
	)
}

// renderCantorRowContent lays out the content for a cantor item row, including labels, values, and highlight state.
func renderCantorRowContent(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig, args CantorItemArgs) layout.Dimensions {
	displayChange := ""
	state.Vault.Mu.Lock()
	if currencyRates, ok := state.Vault.Rates[state.UI.Currency]; ok {
		if entry, ok := currencyRates[args.Cantor.DisplayName]; ok {
			displayChange = entry.DisplayChange
		}
	}
	state.Vault.Mu.Unlock()

	// Determine if we should hide labels to save horizontal space (mobile)
	hideLabels := args.IsMobile

	// Default Horizontal Layout
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
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := GetTranslation(state.UI.Language, "buy_col")
					if hideLabels {
						label = ""
					}
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:         label,
						Value:         args.BuyVal,
						Color:         args.BuyColor,
						Change:        args.Change24h,
						DisplayChange: displayChange,
						IsMobile:      args.IsMobile,
						LightMode:     args.LightMode,
						PulseBest:     args.BestBuyPulse,
					})
				}),
				layout.Rigid(layout.Spacer{Width: spacing}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := GetTranslation(state.UI.Language, "sell_col")
					if hideLabels {
						label = ""
					}
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:     label,
						Value:     args.SellVal,
						Color:     args.SellColor,
						IsMobile:  args.IsMobile,
						LightMode: args.LightMode,
						PulseBest: args.BestSellPulse,
					})
				}),
				layout.Rigid(layout.Spacer{Width: spacing}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := GetTranslation(state.UI.Language, "spread_col")
					if hideLabels {
						label = ""
					}
					return layoutMarketValue(gtx, theme, MarketValueArgs{
						Label:    label,
						Value:    args.SpreadVal,
						Color:    AppColors.Spread,
						IsMobile: args.IsMobile,
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if args.IsSelected {
						return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if args.Cantor.DeleteBtn.Clicked(gtx) {
								log.Printf("DeleteBtn clicked for %s", args.CantorKey)
								go TriggerDeleteCantor(state, config, args.CantorKey, window)
							}
							btn := material.Button(theme, &args.Cantor.DeleteBtn, "×")
							btn.Background = color.NRGBA{R: 200, G: 50, B: 50, A: 255}
							btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
							btn.CornerRadius = unit.Dp(6)
							btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}
							btn.TextSize = unit.Sp(12)
							return btn.Layout(gtx)
						})
					}
					return layout.Dimensions{}
				}),
			)
		}),
	)
}

// pulseBreath returns a 0..1 breathing value over a ~3.6s cycle.
func pulseBreath() float64 {
	t := time.Now().UnixNano()
	phase := float64(t%3_600_000_000) / 3_600_000_000 // ~3.6s cycle
	return 0.5 + 0.5*math.Sin(phase*2*math.Pi)
}

// marketValueColor resolves the value text color, including the TOP-rate breathing pulse.
func marketValueColor(args MarketValueArgs, pulseHue color.NRGBA) color.NRGBA {
	valColor := args.Color
	if args.LightMode &&
		valColor.R == AppColors.Accent1.R && valColor.G == AppColors.Accent1.G && valColor.B == AppColors.Accent1.B {
		// Light theme: darker amber so TOP rates read clearly on white/cream.
		valColor = AppColors.Accent1Dark
	}
	if !args.PulseBest {
		return valColor
	}
	// Breath on TOP hue — light gets a firmer alpha floor for contrast.
	breath := pulseBreath() // 0..1
	base := valColor
	if base.A == 0 {
		base = pulseHue
	}
	aMin, aSpan := 210.0, 45.0
	if args.LightMode {
		aMin, aSpan = 235.0, 20.0 // nearly opaque on light bg
	}
	return color.NRGBA{
		R: base.R,
		G: base.G,
		B: base.B,
		A: uint8(aMin + breath*aSpan),
	}
}

// layoutMarketValueLabel renders the small caption above a market value.
func layoutMarketValueLabel(gtx layout.Context, theme *material.Theme, args MarketValueArgs, pulseHue color.NRGBA, lblSize unit.Sp) layout.Dimensions {
	if args.Label == "" {
		return layout.Dimensions{}
	}
	l := material.Caption(theme, args.Label)
	l.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
	if args.PulseBest {
		l.Color = applyAlpha(pulseHue, 180)
		if args.LightMode {
			l.Color = applyAlpha(pulseHue, 220)
		}
	}
	l.TextSize = lblSize
	return l.Layout(gtx)
}

// layoutMarketValueHalo paints the soft pulsing wash behind a TOP-rate value.
func layoutMarketValueHalo(gtx layout.Context, args MarketValueArgs, pulseHue color.NRGBA) layout.Dimensions {
	if !args.PulseBest {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	breath := pulseBreath()
	halo := pulseHue
	halo.A = uint8(8 + breath*14)
	if args.LightMode {
		halo.A = uint8(28 + breath*32) // readable wash on white
	}
	r := gtx.Dp(unit.Dp(4))
	paint.FillShape(gtx.Ops, halo, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, r).Op(gtx.Ops))
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// layoutMarketValueChange renders the 24h change caption next to a market value.
func layoutMarketValueChange(gtx layout.Context, theme *material.Theme, args MarketValueArgs) layout.Dimensions {
	if args.Change == 0 {
		return layout.Dimensions{}
	}
	col := AppColors.Success
	if args.Change < 0 {
		col = AppColors.Error
	}

	c := material.Caption(theme, args.DisplayChange)
	c.Color = col
	c.TextSize = unit.Sp(9)
	if args.IsMobile {
		c.TextSize = unit.Sp(8)
	}
	return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, c.Layout)
}

// layoutMarketValue lays out a market value displaying a label and its corresponding value with customizable text color.
func layoutMarketValue(gtx layout.Context, theme *material.Theme, args MarketValueArgs) layout.Dimensions {
	valSize := unit.Sp(18)
	lblSize := unit.Sp(12)

	if args.IsMobile {
		valSize = unit.Sp(16)
		lblSize = unit.Sp(11)
	}

	pulseHue := AppColors.Accent1
	if args.LightMode {
		pulseHue = AppColors.Accent1Dark
	}
	valColor := marketValueColor(args, pulseHue)

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutMarketValueLabel(gtx, theme, args, pulseHue, lblSize)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{Alignment: layout.Center}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							return layoutMarketValueHalo(gtx, args, pulseHue)
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{
								Top: unit.Dp(1), Bottom: unit.Dp(1),
								Left: unit.Dp(3), Right: unit.Dp(3),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								v := material.Body2(theme, args.Value)
								v.Color = valColor
								v.TextSize = valSize
								v.Font.Typeface = "Kanit"
								v.Font.Weight = font.Bold
								return v.Layout(gtx)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutMarketValueChange(gtx, theme, args)
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

// getCantorDisplayData generates display values for buy/sell plus best-rate flags for pulsing.
func getCantorDisplayData(
	state *AppState, entry *CantorEntry, bestBuy, bestSell float64,
) (buy, sell, spread string, buyColor, sellColor color.NRGBA, change float64, isBestBuy, isBestSell bool) {
	defColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	if entry == nil {
		return "---", "---", "---", defColor, defColor, 0, false, false
	}

	if entry.Error != "" {
		errTxt := GetTranslation(state.UI.Language, "no_rate_label")
		return errTxt, errTxt, "---", AppColors.Error, AppColors.Error, 0, false, false
	}

	if entry.DisplayBuy == "" {
		RefreshDisplayStrings(entry)
	}

	buyColor = AppColors.Text
	sellColor = AppColors.Text

	currentBuy := parseRate(entry.Rate.BuyRate)
	currentSell := parseRate(entry.Rate.SellRate)

	// Tiny epsilon so float formatting noise does not drop the pulse.
	const eps = 0.0005
	topHue := AppColors.Accent1
	if state.UI.LightMode {
		topHue = AppColors.Accent1Dark // readable amber on white/cream
	}
	if bestBuy > 0 && currentBuy+eps >= bestBuy {
		isBestBuy = true
		buyColor = topHue
	}
	if bestSell > 0 && currentSell > 0 && currentSell-eps <= bestSell {
		isBestSell = true
		sellColor = topHue
	}

	return entry.DisplayBuy, entry.DisplaySell, entry.DisplaySpread, buyColor, sellColor, entry.Rate.Change24h, isBestBuy, isBestSell
}

// layoutHeader renders the application's header, including the market title, subtitle, and a language selection button.
func layoutHeader(gtx layout.Context, window *app.Window,
	theme *material.Theme, state *AppState) layout.Dimensions {

	// On mobile, hide header if search is active to provide space and stability
	if state.UI.CurrentViewMode == ViewMobile && state.UI.SearchActive {
		return layout.Dimensions{}
	}

	return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutMobileMenuButton(gtx, window, state)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return renderHeaderMainSection(gtx, window, theme, state)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if state.UI.CurrentViewMode == ViewMobile {
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
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutFinOpsButton(gtx, window, state)
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

// finOpsButtonInset returns the button padding for the current view mode.
func finOpsButtonInset(compact bool) layout.Inset {
	if compact {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(4)}
	}
	return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}
}

// finOpsButtonBackground returns the button background (red while providers are blocked).
func finOpsButtonBackground(state *AppState, blocked bool) color.NRGBA {
	bgCol := color.NRGBA{R: 50, G: 50, B: 60, A: 255}
	if state.UI.LightMode {
		bgCol = color.NRGBA{R: 0, G: 0, B: 0, A: 20}
	}
	if blocked {
		return color.NRGBA{R: 200, G: 50, B: 50, A: 255}
	}
	if state.UI.FinOpsBtn.Hovered() {
		return color.NRGBA{R: 70, G: 70, B: 80, A: 255}
	}
	return bgCol
}

// layoutFinOpsButtonContent renders the lock icon (compact) or the bold label inside the button.
func layoutFinOpsButtonContent(gtx layout.Context, compact bool, iconCol color.NRGBA, label string) layout.Dimensions {
	pad := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(10), Right: unit.Dp(10)}
	if compact {
		pad = layout.UniformInset(unit.Dp(8))
	}
	return pad.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if compact {
			return DrawIconLock(gtx, iconCol)
		}
		shaper := text.NewShaper(text.WithCollection(nil))
		macro := op.Record(gtx.Ops)
		dims := widget.Label{Alignment: text.Middle}.Layout(gtx, shaper, font.Font{Weight: font.Bold}, unit.Sp(12), label, op.CallOp{})
		call := macro.Stop()
		paint.ColorOp{Color: iconCol}.Add(gtx.Ops)
		call.Add(gtx.Ops)
		return dims
	})
}

// layoutFinOpsButton opens Control Panel (desktop rail / mobile overlay).
func layoutFinOpsButton(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	// When panel is open, the in-panel close control owns FinOpsBtn to avoid double-toggle.
	if state.UI.IsFinOpsDashboardOpen {
		return layout.Dimensions{}
	}
	finOps := state.FinOpsSnapshot()
	if state.UI.FinOpsBtn.Clicked(gtx) {
		state.UI.IsFinOpsDashboardOpen = true
		state.UI.MobileMenuOpen = false
		window.Invalidate()
	}

	compact := state.UI.CurrentViewMode == ViewMobile
	label := GetTranslation(state.UI.Language, "control_panel")
	blocked := len(finOps.BlockedProviders) > 0
	iconCol := AppColors.Accent1
	if blocked {
		iconCol = color.NRGBA{R: 255, G: 120, B: 120, A: 255}
	}

	return state.UI.FinOpsBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return finOpsButtonInset(compact).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bgCol := finOpsButtonBackground(state, blocked)

			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Constraints.Min
					paint.FillShape(gtx.Ops, bgCol, clip.UniformRRect(image.Rectangle{Max: sz}, gtx.Dp(8)).Op(gtx.Ops))
					return layout.Dimensions{Size: sz}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layoutFinOpsButtonContent(gtx, compact, iconCol, label)
				}),
			)
		})
	})
}

// renderHeaderSearchToggle renders a header for search toogle
func renderHeaderSearchToggle(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	return layout.Dimensions{}
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

// startSearchDiscover runs city discovery from the search field (Discover button or Enter).
func startSearchDiscover(window *app.Window, state *AppState, config AppConfig) {
	q := strings.TrimSpace(state.UI.SearchEditor.Text())
	if q == "" {
		return
	}
	log.Printf("Search submit: %q (city discover / URL)", q)
	// Keep the query visible until discovery finishes — clearing it first hid all error messages
	// (handlers only wrote back when Text() still equalled the city).
	state.UI.SearchActive = false
	state.UI.MapFocus.CityName = "" // drop previous city radius filter while searching
	state.UI.FilteredIDs = nil
	// Form is city/area search. Pasted http(s) links still work as a quiet power-user path.
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") {
		state.UI.SearchEditor.SetText("")
		state.UI.SearchText = ""
		go triggerHeuristicDiscovery(q, "", 0, 0, "", state, config, window)
	} else {
		go DiscoverCityCantors(q, state, config, window)
	}
	window.Invalidate()
}

// searchEditorSubmitted drains editor events and reports whether a submit event was seen.
func searchEditorSubmitted(gtx layout.Context, state *AppState) bool {
	submitted := false
	for {
		ev, ok := state.UI.SearchEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	return submitted
}

// searchEnterPressed is a fallback: macOS/IME often skip Editor SubmitEvent — catch Return/Enter while focused.
func searchEnterPressed(gtx layout.Context, state *AppState) bool {
	pressed := false
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &state.UI.SearchEditor, Name: key.NameReturn},
			key.Filter{Focus: &state.UI.SearchEditor, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			pressed = true
		}
	}
	return pressed
}

// LayoutSearchBar: search field + one action button (Szukaj when typing, else Lokalizuj).
func LayoutSearchBar(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	bottomInset := unit.Dp(20)
	if state.UI.CurrentViewMode == ViewMobile {
		bottomInset = unit.Dp(10)
	}

	return layout.Inset{Bottom: bottomInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		state.UI.SearchText = state.UI.SearchEditor.Text()
		hasQuery := len(strings.TrimSpace(state.UI.SearchText)) > 0

		editorSubmitted := searchEditorSubmitted(gtx, state)
		enterPressed := searchEnterPressed(gtx, state)
		if (editorSubmitted || enterPressed) && hasQuery {
			startSearchDiscover(window, state, config)
		}

		if state.UI.IsMobile {
			state.UI.SearchActive = hasQuery
		}

		children := []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if state.UI.CurrentViewMode == ViewDesktop {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(400))
				}
				return drawSearchInput(gtx, theme, state, window)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSearchActionButton(gtx, window, theme, state, config)
			}),
		}
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

// layoutSearchActionButton — single CTA: city search when the field has text, else GPS locate.
func layoutSearchActionButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	q := strings.TrimSpace(state.UI.SearchEditor.Text())
	searchMode := q != ""

	if searchMode {
		if state.UI.DiscoverButton.Clicked(gtx) {
			startSearchDiscover(window, state, config)
		}
	} else {
		handleLocateClick(gtx, window, state, config)
		handleLocateHover(window, state)
	}

	btnText := GetTranslation(state.UI.Language, "discover_button")
	click := &state.UI.DiscoverButton
	if !searchMode {
		btnText = GetTranslation(state.UI.Language, "locate_button")
		click = &state.UI.LocateButton
	}

	btn := material.Button(theme, click, btnText)
	btn.Background = AppColors.Accent1
	btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	if state.UI.LightMode {
		btn.Background = AppColors.Accent1
		btn.Color = color.NRGBA{R: 20, G: 20, B: 25, A: 255}
	}
	btn.CornerRadius = unit.Dp(8)
	btn.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}
	btn.TextSize = unit.Sp(14)

	if !searchMode && state.UI.UserLocation.Active {
		btn.Background = AppColors.Accent1
		btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return btn.Layout(gtx)
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

// handleLocateClick toggles GPS/IP locate only.
func handleLocateClick(gtx layout.Context, window *app.Window, state *AppState, config AppConfig) {
	if !state.UI.LocateButton.Clicked(gtx) {
		return
	}
	if !state.UI.UserLocation.Active {
		go func() {
			lat, lon, err := FetchUserLocation()
			if err != nil || (lat == 0 && lon == 0) {
				fmt.Printf("Location failed: %v\n", err)
				return
			}
			fmt.Printf("Location success: %f, %f\n", lat, lon)
			// No radius slider — locate only for sort/distance, never hide the list.
			ApplyUserLocation(state, lat, lon, 0)
			state.UI.SortMode = "DIST"
			state.UI.FilteredIDs = nil
			window.Invalidate()
		}()
	} else {
		state.UI.UserLocation.Active = false
		state.UI.MaxDistance = 0
		state.UI.FilteredIDs = nil
	}
	window.Invalidate()
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

// fitModalDimension scales a modal side to a percentage of the available space, clamped to [minSize, maxSize].
func fitModalDimension(avail, size, percent, minSize, maxSize int) int {
	if avail <= 0 {
		return size
	}
	if cand := avail * percent / 100; cand < size {
		size = cand
	}
	if size < minSize {
		size = minSize
	}
	if size > maxSize {
		size = maxSize
	}
	return size
}

// modalMaxSize returns the maximum modal width and height for the open modal.
func modalMaxSize(gtx layout.Context, state *AppState) (int, int) {
	if state.UI.ModalOpen != "language" {
		return gtx.Dp(unit.Dp(350)), gtx.Dp(unit.Dp(500))
	}
	// Responsive tumbler: scale with window, keep readable height.
	ww, wh := gtx.Constraints.Max.X, gtx.Constraints.Max.Y
	width := fitModalDimension(ww, gtx.Dp(unit.Dp(340)), 85, gtx.Dp(unit.Dp(280)), gtx.Dp(unit.Dp(420)))
	height := fitModalDimension(wh, gtx.Dp(unit.Dp(380)), 70, gtx.Dp(unit.Dp(320)), gtx.Dp(unit.Dp(520)))
	return width, height
}

// layoutModalFrame draws the modal gradient background, border and content.
func layoutModalFrame(gtx layout.Context, content layout.Widget) layout.Dimensions {
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
				gtx.Constraints.Max.X, gtx.Constraints.Max.Y = modalMaxSize(gtx, state)
				return layoutModalFrame(gtx, content)
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

// layoutIntroAnimation — short splash fade (no decorative wave).
func layoutIntroAnimation(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if !state.UI.IntroAnim.Active {
		return layout.Dimensions{}
	}

	elapsed := time.Since(state.UI.IntroAnim.StartTime).Seconds()
	const duration = 0.9
	const fadeStart = 0.45

	if elapsed > duration {
		state.UI.IntroAnim.Active = false
		window.Invalidate()
		return layout.Dimensions{}
	}

	window.Invalidate()

	opacity := float32(1.0)
	if elapsed > fadeStart {
		opacity = 1.0 - float32((elapsed-fadeStart)/(duration-fadeStart))
	}

	bg := AppColors.Background
	bg.A = uint8(255 * opacity)
	paint.Fill(gtx.Ops, bg)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
