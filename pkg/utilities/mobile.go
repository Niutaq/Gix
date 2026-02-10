package utilities

import (
	// Standard libraries
	"image"
	"image/color"
	"math"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// layoutMobileMenuOverlay renders the side drawer for mobile view.
func layoutMobileMenuOverlay(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState, config AppConfig) layout.Dimensions {
	if !state.UI.MobileMenuOpen {
		return layout.Dimensions{}
	}

	if state.UI.MobileMenuBackdrop.Clicked(gtx) {
		state.UI.MobileMenuOpen = false
		window.Invalidate()
	}

	layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return state.UI.MobileMenuBackdrop.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				paint.Fill(gtx.Ops, color.NRGBA{A: 200})
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
	)

	drawerWidth := gtx.Dp(unit.Dp(280))
	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Background of the drawer
			gtx.Constraints.Min.X = drawerWidth
			gtx.Constraints.Max.X = drawerWidth
			gtx.Constraints.Min.Y = gtx.Constraints.Max.Y

			rect := image.Rectangle{Max: gtx.Constraints.Max}
			bgColor := AppColors.Background
			bgColor.A = 255
			paint.FillShape(gtx.Ops, bgColor, clip.Rect(rect).Op())

			return layout.Inset{Top: unit.Dp(20), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Close Button
						h3 := material.H6(theme, "Menu")
						h3.Color = AppColors.Title
						return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, h3.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutThemeButton(gtx, window, state)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutLanguageButton(gtx, window, theme, state)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(theme, GetTranslation(state.UI.Language, "sidebar_currency_label"))
						lbl.Color = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						// Reuse logic from VerticalCurrencyBar but adapted
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
									// Close menu on selection
									state.UI.MobileMenuOpen = false
									window.Invalidate()
								}

								return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										txtColor := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
										if isSelected {
											txtColor = AppColors.Accent1
										}
										lbl := material.Body1(theme, currency)
										lbl.Color = txtColor
										return layout.UniformInset(unit.Dp(10)).Layout(gtx, lbl.Layout)
									})
								})
							})
					}),
				)
			})
		}),
	)
}

// DrawIconMenu draws a Material-style hamburger menu icon.
func DrawIconMenu(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	size := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min = image.Point{X: size, Y: size}

	thickness := gtx.Dp(unit.Dp(2))
	width := gtx.Dp(unit.Dp(18))
	xOffset := (size - width) / 2

	// Draw 3 bars
	for i := 0; i < 3; i++ {
		y := (size / 4) * (i + 1)
		rect := image.Rect(xOffset, y-thickness/2, xOffset+width, y+thickness/2)
		paint.FillShape(gtx.Ops, col, clip.UniformRRect(rect, thickness/2).Op(gtx.Ops))
	}

	return layout.Dimensions{Size: image.Point{X: size, Y: size}}
}

// DrawIconClose draws a Material-style close (X) icon.
func DrawIconClose(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	size := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min = image.Point{X: size, Y: size}

	thickness := float32(gtx.Dp(unit.Dp(2)))
	padding := float32(size) * 0.25

	// Diagonal 1
	var path1 clip.Path
	path1.Begin(gtx.Ops)
	path1.MoveTo(f32.Point{X: padding, Y: padding})
	path1.LineTo(f32.Point{X: float32(size) - padding, Y: float32(size) - padding})
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: path1.End(), Width: thickness}.Op())

	// Diagonal 2
	var path2 clip.Path
	path2.Begin(gtx.Ops)
	path2.MoveTo(f32.Point{X: float32(size) - padding, Y: padding})
	path2.LineTo(f32.Point{X: padding, Y: float32(size) - padding})
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: path2.End(), Width: thickness}.Op())

	return layout.Dimensions{Size: image.Point{X: size, Y: size}}
}

// DrawIconSearch draws a Material-style search icon (magnifying glass).
func DrawIconSearch(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	size := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min = image.Point{X: size, Y: size}

	thickness := float32(gtx.Dp(unit.Dp(2)))
	radius := float32(size) * 0.25
	center := f32.Point{X: float32(size) * 0.45, Y: float32(size) * 0.45}

	// Circle
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(f32.Point{X: center.X + radius, Y: center.Y})
	path.Arc(f32.Point{X: -radius, Y: 0}, f32.Point{X: -radius, Y: 0}, 2*math.Pi)
	
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  path.End(),
		Width: thickness,
	}.Op())

	// Handle
	handleStart := f32.Point{
		X: center.X + radius*float32(math.Cos(math.Pi/4)),
		Y: center.Y + radius*float32(math.Sin(math.Pi/4)),
	}
	handleEnd := f32.Point{X: float32(size) * 0.85, Y: float32(size) * 0.85}

	var handlePath clip.Path
	handlePath.Begin(gtx.Ops)
	handlePath.MoveTo(handleStart)
	handlePath.LineTo(handleEnd)
	
	paint.FillShape(gtx.Ops, col, clip.Stroke{
		Path:  handlePath.End(),
		Width: thickness,
	}.Op())

	return layout.Dimensions{Size: image.Point{X: size, Y: size}}
}