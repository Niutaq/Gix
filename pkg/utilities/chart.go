package utilities

import (
	// Standard libraries
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	// Gio utilities
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// LineChart holds the configuration for a simple line chart.
type LineChart struct {
	Data       []float64
	Timestamps []int64 // Unix timestamps corresponding to Data points
	StartLabel string
	Tag        interface{} // Tag for pointer events
}

// chartLayoutContext holds pre-calculated layout values to avoid passing too many arguments.
type chartLayoutContext struct {
	width      float32
	height     float32
	paddingX   float32
	chartWidth float32
	minVal     float64
	maxVal     float64
	minPlotVal float64
	maxPlotVal float64
	plotRange  float64
}

// Layout renders the line chart within the given constraints.
func (lc *LineChart) Layout(gtx layout.Context, theme *material.Theme, alpha uint8, state *UIState) layout.Dimensions {
	if len(lc.Data) < 2 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	processChartEvents(gtx, lc, state)

	// Calculate layout context
	ctx := calculateChartLayout(gtx, lc)

	// Draw components
	drawGrid(gtx, ctx, alpha)
	drawChartLines(gtx, ctx, lc, alpha)
	drawChartLabels(gtx, theme, ctx, lc, alpha, state.Language)

	if state.ChartHoverActive {
		drawTooltip(gtx, theme, ctx, lc, state, alpha)
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func processChartEvents(gtx layout.Context, lc *LineChart, state *UIState) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: lc.Tag,
			Kinds:  pointer.Move | pointer.Enter | pointer.Leave,
		})
		if !ok {
			break
		}
		if xev, ok := ev.(pointer.Event); ok {
			switch xev.Kind {
			case pointer.Move, pointer.Enter:
				state.ChartHoverActive = true
				state.ChartHoverX = xev.Position.X
			case pointer.Leave:
				state.ChartHoverActive = false
			}
		}
	}

	// Area for pointer events
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, lc.Tag)
}

func calculateChartLayout(gtx layout.Context, lc *LineChart) chartLayoutContext {
	width := float32(gtx.Constraints.Max.X)
	height := float32(gtx.Constraints.Max.Y)

	minVal, maxVal := lc.Data[0], lc.Data[0]
	for _, v := range lc.Data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	rangeVal := maxVal - minVal
	if rangeVal == 0 {
		rangeVal = maxVal * 0.2
		if rangeVal == 0 {
			rangeVal = 0.1
		}
	}

	verticalPadding := rangeVal * 0.25
	paddingX := float32(16.0)

	return chartLayoutContext{
		width:      width,
		height:     height,
		paddingX:   paddingX,
		chartWidth: width - (paddingX * 2),
		minVal:     minVal,
		maxVal:     maxVal,
		minPlotVal: minVal - verticalPadding,
		maxPlotVal: maxVal + verticalPadding,
		plotRange:  (maxVal + verticalPadding) - (minVal - verticalPadding),
	}
}

func drawGrid(gtx layout.Context, ctx chartLayoutContext, alpha uint8) {
	gridColor := applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 10}, alpha)
	for _, p := range []float32{0.0, 0.33, 0.66, 1.0} {
		y := ctx.height - (p * ctx.height)
		stack := clip.Rect{Min: image.Point{0, int(y)}, Max: image.Point{int(ctx.width), int(y) + 1}}.Push(gtx.Ops)
		paint.ColorOp{Color: gridColor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}
}

func drawChartLines(gtx layout.Context, ctx chartLayoutContext, lc *LineChart, alpha uint8) {
	var path clip.Path
	path.Begin(gtx.Ops)

	total := len(lc.Data)
	startPt := getPoint(ctx, 0, total, lc.Data[0])
	path.MoveTo(startPt)
	for i := 1; i < total; i++ {
		path.LineTo(getPoint(ctx, i, total, lc.Data[i]))
	}

	lastPt := getPoint(ctx, total-1, total, lc.Data[total-1])
	path.LineTo(f32.Point{X: lastPt.X, Y: ctx.height})
	path.LineTo(f32.Point{X: startPt.X, Y: ctx.height})
	path.Close()

	// Gradient Fill
	fillStartColor := applyAlpha(AppColors.Accent1, alpha/4)
	fillEndColor := applyAlpha(AppColors.Accent1, 0)

	stack := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
	paint.LinearGradientOp{
		Stop1:  f32.Point{X: 0, Y: 0},
		Stop2:  f32.Point{X: 0, Y: ctx.height},
		Color1: fillStartColor,
		Color2: fillEndColor,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()

	// Stroke Line
	var linePath clip.Path
	linePath.Begin(gtx.Ops)
	linePath.MoveTo(startPt)
	for i := 1; i < total; i++ {
		linePath.LineTo(getPoint(ctx, i, total, lc.Data[i]))
	}
	strokeColor := applyAlpha(AppColors.Accent1, alpha)
	paint.FillShape(gtx.Ops, strokeColor, clip.Stroke{Path: linePath.End(), Width: 3.0}.Op())

	// End Point Dots
	drawEndDots(gtx, ctx, lastPt, alpha)
}

func drawEndDots(gtx layout.Context, ctx chartLayoutContext, pt f32.Point, alpha uint8) {
	circle := clip.Ellipse{
		Min: image.Point{X: int(pt.X) - 5, Y: int(pt.Y) - 5},
		Max: image.Point{X: int(pt.X) + 5, Y: int(pt.Y) + 5},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 150}, alpha), circle)

	core := clip.Ellipse{
		Min: image.Point{X: int(pt.X) - 3, Y: int(pt.Y) - 3},
		Max: image.Point{X: int(pt.X) + 3, Y: int(pt.Y) + 3},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alpha), core)
}

func drawChartLabels(gtx layout.Context, theme *material.Theme, ctx chartLayoutContext, lc *LineChart, alpha uint8, lang string) {
	resetOffset := func(p image.Point) {
		op.Offset(p.Mul(-1)).Add(gtx.Ops)
	}

	// Last Value Label
	total := len(lc.Data)
	lastVal := lc.Data[total-1]
	lastPt := getPoint(ctx, total-1, total, lastVal)

	valTxt := fmt.Sprintf("%.4f", lastVal)
	lblVal := material.Caption(theme, valTxt)
	lblVal.Color = applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alpha)
	lblVal.Font.Weight = font.Bold
	lblVal.TextSize = unit.Sp(11)

	labelOffset := image.Point{
		X: int(lastPt.X) - 70,
		Y: int(lastPt.Y) - 30,
	}
	if lastPt.Y < 40 {
		labelOffset.Y = int(lastPt.Y) + 15
	}
	if labelOffset.X+55 > int(ctx.width) {
		labelOffset.X = int(ctx.width) - 55
	}

	op.Offset(labelOffset).Add(gtx.Ops)
	bgRect := clip.UniformRRect(image.Rectangle{Max: image.Point{X: 55, Y: 18}}, 4).Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 20, G: 20, B: 25, A: 200}, alpha), bgRect)
	layout.Inset{Left: unit.Dp(4), Top: unit.Dp(1)}.Layout(gtx, lblVal.Layout)
	resetOffset(labelOffset)

	// Max Value Label
	maxOffset := image.Point{X: int(ctx.paddingX), Y: 15}
	op.Offset(maxOffset).Add(gtx.Ops)
	layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(theme, fmt.Sprintf("%.4f", ctx.maxVal))
		lbl.Color = applyAlpha(color.NRGBA{R: 200, G: 200, B: 200, A: 180}, alpha)
		lbl.TextSize = unit.Sp(10)
		return lbl.Layout(gtx)
	})
	resetOffset(maxOffset)

	// X-Axis Labels
	startTxt := lc.StartLabel
	endTxt := ""

	if len(lc.Timestamps) >= 2 {
		startTxt = GetFormattedDate(lang, time.Unix(lc.Timestamps[0], 0))
		endTxt = GetFormattedDate(lang, time.Unix(lc.Timestamps[len(lc.Timestamps)-1], 0))
	}

	// Draw Start Label
	if startTxt != "" {
		dateOffset := image.Point{X: int(ctx.paddingX), Y: int(ctx.height) - 20}
		op.Offset(dateOffset).Add(gtx.Ops)
		layout.Inset{Left: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, startTxt)
			lbl.Color = applyAlpha(color.NRGBA{R: 150, G: 150, B: 160, A: 150}, alpha)
			lbl.TextSize = unit.Sp(10)
			return lbl.Layout(gtx)
		})
		resetOffset(dateOffset)
	}

	// Draw End Label
	if endTxt != "" {
		endOffset := image.Point{X: int(ctx.width) - 40, Y: int(ctx.height) - 20}
		op.Offset(endOffset).Add(gtx.Ops)
		layout.Inset{Left: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, endTxt)
			lbl.Color = applyAlpha(color.NRGBA{R: 150, G: 150, B: 160, A: 150}, alpha)
			lbl.TextSize = unit.Sp(10)
			return lbl.Layout(gtx)
		})
		resetOffset(endOffset)
	}
}

func drawTooltip(gtx layout.Context, theme *material.Theme, ctx chartLayoutContext, lc *LineChart, state *UIState, alpha uint8) {
	resetOffset := func(p image.Point) {
		op.Offset(p.Mul(-1)).Add(gtx.Ops)
	}

	relativeX := state.ChartHoverX - ctx.paddingX
	if relativeX < 0 {
		relativeX = 0
	}
	if relativeX > ctx.chartWidth {
		relativeX = ctx.chartWidth
	}

	hoverIdx := int(math.Round(float64(relativeX / ctx.chartWidth * float32(len(lc.Data)-1))))
	if hoverIdx < 0 || hoverIdx >= len(lc.Data) {
		return
	}

	hoverVal := lc.Data[hoverIdx]
	hoverPt := getPoint(ctx, hoverIdx, len(lc.Data), hoverVal)

	// Vertical line
	lineRect := image.Rect(int(hoverPt.X), 0, int(hoverPt.X)+1, int(ctx.height))
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 50}, alpha), clip.Rect(lineRect).Op())

	// Hover point circle
	hCircle := clip.Ellipse{
		Min: image.Point{X: int(hoverPt.X) - 4, Y: int(hoverPt.Y) - 4},
		Max: image.Point{X: int(hoverPt.X) + 4, Y: int(hoverPt.Y) + 4},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(AppColors.Accent1, alpha), hCircle)

	// Tooltip Text
	valTxt := fmt.Sprintf("%.4f", hoverVal)
	timeTxt := ""
	if len(lc.Timestamps) > hoverIdx {
		t := time.Unix(lc.Timestamps[hoverIdx], 0)
		timeTxt = GetFormattedDateTime(state.Language, t)
	}

	// Tooltip Layout
	tipOffset := image.Point{X: int(hoverPt.X) - 40, Y: int(hoverPt.Y) - 45}
	// Boundary checks
	if tipOffset.X < 0 {
		tipOffset.X = 0
	}
	if tipOffset.X > int(ctx.width)-80 {
		tipOffset.X = int(ctx.width) - 80
	}
	if tipOffset.Y < 0 {
		tipOffset.Y = int(hoverPt.Y) + 20
	}

	op.Offset(tipOffset).Add(gtx.Ops)

	// Background
	tipWidth := 80
	tipHeight := 34
	if timeTxt == "" {
		tipHeight = 20
	}
	tipBG := clip.UniformRRect(image.Rectangle{Max: image.Point{X: tipWidth, Y: tipHeight}}, 4).Op(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 35, G: 35, B: 40, A: 255}, tipBG)

	// Render Text
	layout.Inset{Left: unit.Dp(6), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(theme, valTxt)
				lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if timeTxt == "" {
					return layout.Dimensions{}
				}
				lbl := material.Caption(theme, timeTxt)
				lbl.Color = color.NRGBA{R: 190, G: 190, B: 210, A: 255}
				lbl.TextSize = unit.Sp(10)
				return lbl.Layout(gtx)
			}),
		)
	})

	resetOffset(tipOffset)
}

func getPoint(ctx chartLayoutContext, i int, totalPoints int, val float64) f32.Point {
	x := ctx.paddingX + (float32(i)/float32(totalPoints-1))*ctx.chartWidth
	normalizedY := (val - ctx.minPlotVal) / ctx.plotRange
	y := ctx.height - (float32(normalizedY) * ctx.height)
	return f32.Point{X: x, Y: y}
}

// Helper to calculate alpha
func applyAlpha(c color.NRGBA, a uint8) color.NRGBA {
	newA := uint16(c.A) * uint16(a) / 255
	c.A = uint8(newA)
	return c
}

// GenerateFakeData creates a sine-wave like dataset for demo purposes.
func GenerateFakeData(points int, basePrice float64, seed int64) []float64 {
	data := make([]float64, points)
	if basePrice <= 0 {
		return nil
	}

	phaseShift := float64(seed % 100)

	for i := 0; i < points; i++ {
		x := float64(i)*0.1 + phaseShift
		volatility := basePrice * 0.01
		sineComponent := math.Sin(x) * volatility * 0.5
		trendComponent := float64(i) * (volatility * 0.01)
		microNoise := math.Cos(x*3.0+float64(seed)) * volatility * 0.2
		val := basePrice + sineComponent + trendComponent + microNoise
		data[i] = val
	}

	lastVal := data[points-1]
	diff := basePrice - lastVal

	for i := 0; i < points; i++ {
		data[i] += diff
	}

	return data
}
