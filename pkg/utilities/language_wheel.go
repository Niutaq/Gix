package utilities

import (
	"image"
	"image/color"
	"math"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// drawLangWheelSoftBand — soft center wash; fades out left/right (no hard vertical walls).
func drawLangWheelSoftBand(ops *op.Ops, width, centerY, rowH int, light bool) {
	bandTop := centerY - rowH/2
	bandBot := centerY + rowH/2
	fadeY := rowH * 55 / 100
	if fadeY < 12 {
		fadeY = 12
	}
	// Wide side falloff so L/R edges dissolve instead of clipping.
	fadeX := width * 32 / 100
	if fadeX < 40 {
		fadeX = 40
	}
	if fadeX > width/2-4 {
		fadeX = width/2 - 4
		if fadeX < 12 {
			fadeX = 12
		}
	}
	cx := width / 2
	topY := bandTop - fadeY
	botY := bandBot + fadeY

	var cMid, cClear color.NRGBA
	if light {
		cMid = color.NRGBA{R: 180, G: 140, B: 40, A: 18}
		cClear = color.NRGBA{R: 180, G: 140, B: 40, A: 0}
	} else {
		cMid = color.NRGBA{R: 255, G: 200, B: 60, A: 22}
		cClear = color.NRGBA{R: 255, G: 200, B: 60, A: 0}
	}

	// Horizontal soft strip first (A=0 at outer L/R → no vertical hard edge).
	func() {
		defer clip.Rect{
			Min: image.Point{X: cx - fadeX, Y: topY},
			Max: image.Point{X: cx, Y: botY},
		}.Push(ops).Pop()
		paint.LinearGradientOp{
			Stop1:  f32.Point{X: float32(cx - fadeX)},
			Stop2:  f32.Point{X: float32(cx)},
			Color1: cClear,
			Color2: cMid,
		}.Add(ops)
		paint.PaintOp{}.Add(ops)
	}()
	func() {
		defer clip.Rect{
			Min: image.Point{X: cx, Y: topY},
			Max: image.Point{X: cx + fadeX, Y: botY},
		}.Push(ops).Pop()
		paint.LinearGradientOp{
			Stop1:  f32.Point{X: float32(cx)},
			Stop2:  f32.Point{X: float32(cx + fadeX)},
			Color1: cMid,
			Color2: cClear,
		}.Add(ops)
		paint.PaintOp{}.Add(ops)
	}()

	// Soften top/bottom of that strip with modal-matching cover fades.
	bg := AppColors.Dark
	bg.A = 255
	if light {
		bg = AppColors.Background
		bg.A = 255
	}
	bgClear := bg
	bgClear.A = 0
	func() {
		defer clip.Rect{
			Min: image.Point{X: cx - fadeX, Y: topY},
			Max: image.Point{X: cx + fadeX, Y: bandTop + rowH/4},
		}.Push(ops).Pop()
		paint.LinearGradientOp{
			Stop1:  f32.Point{Y: float32(topY)},
			Stop2:  f32.Point{Y: float32(bandTop + rowH/4)},
			Color1: bg,
			Color2: bgClear,
		}.Add(ops)
		paint.PaintOp{}.Add(ops)
	}()
	func() {
		defer clip.Rect{
			Min: image.Point{X: cx - fadeX, Y: bandBot - rowH/4},
			Max: image.Point{X: cx + fadeX, Y: botY},
		}.Push(ops).Pop()
		paint.LinearGradientOp{
			Stop1:  f32.Point{Y: float32(bandBot - rowH/4)},
			Stop2:  f32.Point{Y: float32(botY)},
			Color1: bgClear,
			Color2: bg,
		}.Add(ops)
		paint.PaintOp{}.Add(ops)
	}()
}

// languageNativeName — endonym for ISO language code.
func languageNativeName(code string) string {
	switch NormalizeLanguageCode(code) {
	case "EN":
		return "English"
	case "PL":
		return "Polski"
	case "DE":
		return "Deutsch"
	case "DA":
		return "Dansk"
	case "NO":
		return "Norsk"
	case "FR":
		return "Français"
	case "SV":
		return "Svenska"
	case "CZ":
		return "Čeština"
	case "HR":
		return "Hrvatski"
	case "HU":
		return "Magyar"
	case "UA":
		return "Українська"
	case "BG":
		return "Български"
	case "RO":
		return "Română"
	case "AL":
		return "Shqip"
	case "TR":
		return "Türkçe"
	case "IS":
		return "Íslenska"
	default:
		return code
	}
}

func langWheelIndexOf(options []string, code string) int {
	code = NormalizeLanguageCode(code)
	for i, o := range options {
		if NormalizeLanguageCode(o) == code {
			return i
		}
	}
	return 0
}

func ensureLangWheelReady(state *AppState) {
	opts := state.UI.LanguageOptions
	if len(state.UI.LangWheelClicks) < len(opts) {
		state.UI.LangWheelClicks = make([]widget.Clickable, len(opts))
	}
	if !state.UI.LangWheelSynced {
		idx := float32(langWheelIndexOf(opts, state.UI.Language))
		state.UI.LangWheelScroll = idx
		state.UI.LangWheelTarget = idx
		state.UI.LangWheelSettling = false
		state.UI.LangWheelSynced = true
	}
}

func langWheelCenteredIndex(state *AppState) int {
	n := len(state.UI.LanguageOptions)
	if n == 0 {
		return 0
	}
	idx := int(math.Round(float64(state.UI.LangWheelScroll)))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return idx
}

func langWheelSnapTo(state *AppState, idx int, window *app.Window) {
	n := len(state.UI.LanguageOptions)
	if n == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	state.UI.LangWheelTarget = float32(idx)
	state.UI.LangWheelSettling = true
	if window != nil {
		window.Invalidate()
	}
}

func langWheelConfirm(state *AppState, window *app.Window) {
	opts := state.UI.LanguageOptions
	if len(opts) == 0 {
		return
	}
	idx := langWheelCenteredIndex(state)
	state.UI.Language = NormalizeLanguageCode(opts[idx])
	state.UI.ModalOpen = ""
	state.UI.LangWheelSynced = false
	if window != nil {
		window.Invalidate()
	}
}

func langWheelTick(state *AppState, window *app.Window) {
	if !state.UI.LangWheelSettling {
		return
	}
	diff := state.UI.LangWheelTarget - state.UI.LangWheelScroll
	if float32(math.Abs(float64(diff))) < 0.008 {
		state.UI.LangWheelScroll = state.UI.LangWheelTarget
		state.UI.LangWheelSettling = false
		return
	}
	state.UI.LangWheelScroll += diff * 0.35
	if window != nil {
		window.Invalidate()
	}
}

// HandleLanguageWheelKey — window-level keys (Gio delivers arrows outside gtx.Event).
func HandleLanguageWheelKey(state *AppState, window *app.Window, e key.Event) bool {
	if state == nil || state.UI.ModalOpen != "language" || e.State != key.Press {
		return false
	}
	ensureLangWheelReady(state)
	cur := langWheelCenteredIndex(state)
	switch e.Name {
	case key.NameUpArrow:
		langWheelSnapTo(state, cur-1, window)
		return true
	case key.NameDownArrow:
		langWheelSnapTo(state, cur+1, window)
		return true
	case key.NameReturn, key.NameEnter:
		langWheelConfirm(state, window)
		return true
	case key.NameEscape:
		state.UI.ModalOpen = ""
		state.UI.LangWheelSynced = false
		if window != nil {
			window.Invalidate()
		}
		return true
	}
	return false
}

// LanguageModal — compact macOS-style language tumbler (content-sized, no empty Flexed void).
func LanguageModal(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	ensureLangWheelReady(state)
	langWheelTick(state, window)
	gtx.Execute(key.FocusCmd{Tag: &state.UI.LangWheelTag})

	title := GetTranslation(state.UI.Language, "select_lang_title")
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layoutModalHeader(window, theme, title, state),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutLanguageWheel(gtx, window, theme, state)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.UI.IsMobile || state.UI.CurrentViewMode == ViewMobile {
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{}
				})
			}
			hint := material.Caption(theme, "↑↓  ·  Enter")
			hint.Color = color.NRGBA{R: 120, G: 120, B: 130, A: 255}
			hint.Alignment = text.Middle
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, hint.Layout)
		}),
	)
}

func layoutLanguageWheel(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	opts := state.UI.LanguageOptions
	n := len(opts)
	if n == 0 {
		return layout.Dimensions{}
	}

	// Fixed row pitch — band and labels share the same grid (no scale-on-box drift).
	rowH := gtx.Dp(unit.Dp(40))
	visibleRows := 5
	if maxY := gtx.Constraints.Max.Y; maxY > 0 {
		// Use available modal body height (leave room for header/hint already outside).
		fit := maxY / rowH
		if fit >= 7 {
			visibleRows = 7
		} else if fit >= 5 {
			visibleRows = 5
		} else if fit >= 3 {
			visibleRows = 3
		}
		if visibleRows*rowH > maxY && maxY >= rowH*3 {
			visibleRows = maxY / rowH
			if visibleRows%2 == 0 {
				visibleRows-- // keep odd so a row sits on center
			}
			if visibleRows < 3 {
				visibleRows = 3
			}
		}
	}
	viewportH := rowH * visibleRows
	width := gtx.Constraints.Max.X
	if width < gtx.Dp(unit.Dp(200)) {
		width = gtx.Constraints.Min.X
	}

	// Also accept keys via frame queue (when focus works).
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &state.UI.LangWheelTag, Name: key.NameUpArrow},
			key.Filter{Focus: &state.UI.LangWheelTag, Name: key.NameDownArrow},
			key.Filter{Focus: &state.UI.LangWheelTag, Name: key.NameReturn},
			key.Filter{Focus: &state.UI.LangWheelTag, Name: key.NameEnter},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		HandleLanguageWheelKey(state, window, ke)
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &state.UI.LangWheelTag,
			Kinds:  pointer.Scroll,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll {
			continue
		}
		// macOS trackpad: positive Y often means scroll content up → index increases.
		state.UI.LangWheelScroll += pe.Scroll.Y / float32(rowH)
		if state.UI.LangWheelScroll < 0 {
			state.UI.LangWheelScroll = 0
		}
		max := float32(n - 1)
		if state.UI.LangWheelScroll > max {
			state.UI.LangWheelScroll = max
		}
		state.UI.LangWheelTarget = float32(math.Round(float64(state.UI.LangWheelScroll)))
		state.UI.LangWheelSettling = true
		window.Invalidate()
	}

	defer clip.Rect{Max: image.Point{X: width, Y: viewportH}}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &state.UI.LangWheelTag)

	centerY := float32(viewportH) / 2
	scroll := state.UI.LangWheelScroll
	light := state.UI.LightMode

	drawLangWheelSoftBand(gtx.Ops, width, int(centerY), rowH, light)

	activeIdx := langWheelCenteredIndex(state)

	for i := 0; i < n; i++ {
		// Row top in viewport: index i centered when scroll == i.
		top := centerY + (float32(i)-scroll)*float32(rowH) - float32(rowH)/2
		if top+float32(rowH) < 0 || top > float32(viewportH) {
			continue
		}

		distRows := math.Abs(float64(float32(i) - scroll))
		opacity := 1.0 - distRows*0.32
		if opacity < 0.18 {
			continue
		}
		if opacity > 1 {
			opacity = 1
		}

		code := NormalizeLanguageCode(opts[i])
		label := code + "   " + languageNativeName(code)
		isActive := i == activeIdx && distRows < 0.5

		var txtCol color.NRGBA
		weight := font.Normal
		size := unit.Sp(14)
		if isActive {
			weight = font.Bold
			size = unit.Sp(16)
			if light {
				txtCol = AppColors.Accent1Dark // dark gold on light wash
			} else {
				txtCol = color.NRGBA{R: 255, G: 236, B: 170, A: 255}
			}
		} else if light {
			// Dark grey — readable on white; fade with distance.
			txtCol = color.NRGBA{R: 70, G: 70, B: 80, A: uint8(220 * opacity)}
		} else {
			txtCol = color.NRGBA{R: 190, G: 190, B: 200, A: uint8(210 * opacity)}
		}

		idx := i
		btn := &state.UI.LangWheelClicks[idx]
		if btn.Clicked(gtx) {
			if isActive {
				langWheelConfirm(state, window)
			} else {
				langWheelSnapTo(state, idx, window)
			}
		}

		stack := op.Offset(image.Point{X: 0, Y: int(math.Round(float64(top)))}).Push(gtx.Ops)
		rowGtx := gtx
		rowGtx.Constraints.Min = image.Point{X: width, Y: rowH}
		rowGtx.Constraints.Max = rowGtx.Constraints.Min
		btn.Layout(rowGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(theme, label)
				lbl.Color = txtCol
				lbl.Font.Weight = weight
				lbl.TextSize = size
				lbl.MaxLines = 1
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			})
		})
		stack.Pop()
	}

	// Edge fade (mask-image analogue).
	fadeH := rowH
	cEdge := AppColors.Dark
	cEdge.A = 255
	cClear := cEdge
	cClear.A = 0
	func() {
		defer clip.Rect{Max: image.Point{X: width, Y: fadeH}}.Push(gtx.Ops).Pop()
		paint.LinearGradientOp{
			Stop1: f32.Point{}, Stop2: f32.Point{Y: float32(fadeH)},
			Color1: cEdge, Color2: cClear,
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
	func() {
		defer clip.Rect{
			Min: image.Point{Y: viewportH - fadeH},
			Max: image.Point{X: width, Y: viewportH},
		}.Push(gtx.Ops).Pop()
		paint.LinearGradientOp{
			Stop1: f32.Point{Y: float32(viewportH - fadeH)}, Stop2: f32.Point{Y: float32(viewportH)},
			Color1: cClear, Color2: cEdge,
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()

	return layout.Dimensions{Size: image.Point{X: width, Y: viewportH}}
}

// layoutLanguageButton — sidebar / drawer trigger (bg sized to content — no zero-height “horn”).
func layoutLanguageButton(gtx layout.Context, window *app.Window, theme *material.Theme, state *AppState) layout.Dimensions {
	if state.UI.LangModalButton.Clicked(gtx) {
		state.UI.ModalOpen = "language"
		state.UI.LangWheelSynced = false
		window.Invalidate()
	}

	code := NormalizeLanguageCode(state.UI.Language)
	native := languageNativeName(code)
	showNative := state.UI.CurrentViewMode != ViewMobile

	return state.UI.LangModalButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := color.NRGBA{R: 255, G: 215, B: 0, A: 18}
		border := AppColors.Accent1
		border.A = 100
		if state.UI.LightMode {
			bg = color.NRGBA{R: 212, G: 175, B: 55, A: 28}
		}
		if state.UI.LangModalButton.Hovered() {
			bg.A = 40
			border.A = 180
		}
		rr := gtx.Dp(10)

		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Constraints.Min
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, rr).Op(gtx.Ops))
				return widget.Border{Color: border, Width: unit.Dp(1), CornerRadius: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: sz}
				})
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(theme, code)
							lbl.Color = AppColors.Accent1
							lbl.Font.Weight = font.Bold
							lbl.TextSize = unit.Sp(13)
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !showNative {
								return layout.Dimensions{}
							}
							short := native
							runes := []rune(short)
							if len(runes) > 8 {
								short = string(runes[:7]) + "…"
							}
							cap := material.Caption(theme, short)
							cap.Color = applyAlpha(AppColors.Text, 170)
							cap.TextSize = unit.Sp(9)
							cap.Alignment = text.Middle
							return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, cap.Layout)
						}),
					)
				})
			}),
		)
	})
}
