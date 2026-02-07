package utilities

import (
	// Standard libraries
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"runtime"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
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
func (lc *LineChart) Layout(gtx layout.Context, window *app.Window, theme *material.Theme, alpha uint8, state *UIState) layout.Dimensions {
	if len(lc.Data) < 2 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	processChartEvents(gtx, lc, state)

	// Calculate layout context
	ctx := calculateChartLayout(gtx, lc)

	// Draw components
	drawGrid(gtx, ctx, alpha)
	drawChartLines(gtx, window, ctx, lc, alpha)
	drawChartLabels(gtx, theme, ctx, lc, alpha, state.Language)

	if state.ChartHoverActive {
		drawTooltip(gtx, theme, ctx, lc, state, alpha)
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// processChartEvents handles pointer events for a line chart, updating UI interaction states like hover activity and position.
func processChartEvents(gtx layout.Context, lc *LineChart, state *UIState) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: lc.Tag,
			Kinds:  pointer.Move | pointer.Enter | pointer.Leave | pointer.Drag | pointer.Press,
		})
		if !ok {
			break
		}
		if xev, ok := ev.(pointer.Event); ok {
			switch xev.Kind {
			case pointer.Move, pointer.Enter, pointer.Drag, pointer.Press:
				state.ChartHoverActive = true
				state.ChartHoverX = xev.Position.X
			case pointer.Leave:
				state.ChartHoverActive = false
			default:
			}
		}
	}

	// Area for pointer events
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, lc.Tag)
}

// calculateChartLayout calculates the layout context for a line chart.
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

// drawGrid renders the grid lines for the chart.
func drawGrid(gtx layout.Context, ctx chartLayoutContext, alpha uint8) {
	gridCol := color.NRGBA{R: 255, G: 255, B: 255, A: 10}
	if AppColors.Background.R > 200 { // Check if light theme
		gridCol = color.NRGBA{R: 0, G: 0, B: 0, A: 15}
	}
	gridColor := applyAlpha(gridCol, alpha)
	for _, p := range []float32{0.0, 0.33, 0.66, 1.0} {
		y := ctx.height - (p * ctx.height)
		stack := clip.Rect{Min: image.Point{Y: int(y)}, Max: image.Point{X: int(ctx.width), Y: int(y) + 1}}.Push(gtx.Ops)
		paint.ColorOp{Color: gridColor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}
}

// drawChartLines renders the line chart's lines and fills.
func drawChartLines(gtx layout.Context, window *app.Window, ctx chartLayoutContext, lc *LineChart, alpha uint8) {
	var path clip.Path

	total := len(lc.Data)
	startPt := getPoint(ctx, 0, total, lc.Data[0])
	lastPt := getPoint(ctx, total-1, total, lc.Data[total-1])

	// Gradient Fill
	gradAlpha := alpha / 4
	if runtime.GOOS == "linux" {
		gradAlpha = alpha / 8
	}
	fillStartColor := applyAlpha(AppColors.Accent1, gradAlpha)
	fillEndColor := applyAlpha(AppColors.Accent1, 0)

	path.Begin(gtx.Ops)
	path.MoveTo(startPt)
	for i := 1; i < total; i++ {
		path.LineTo(getPoint(ctx, i, total, lc.Data[i]))
	}

	path.LineTo(f32.Point{X: lastPt.X, Y: ctx.height})
	path.LineTo(f32.Point{X: startPt.X, Y: ctx.height})
	path.Close()

	stack := clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
	paint.LinearGradientOp{
		Stop1:  f32.Point{X: 0, Y: 0},
		Stop2:  f32.Point{X: 0, Y: ctx.height},
		Color1: fillStartColor,
		Color2: fillEndColor,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()

	var linePath clip.Path
	linePath.Begin(gtx.Ops)
	linePath.MoveTo(startPt)
	for i := 1; i < total; i++ {
		linePath.LineTo(getPoint(ctx, i, total, lc.Data[i]))
	}
	pathSpec := linePath.End()

	strokeColor := applyAlpha(AppColors.Accent1, alpha)

	glowAlpha := alpha / 6
	glowWidth := float32(10.0)
	if runtime.GOOS == "linux" {
		glowAlpha = alpha / 12
		glowWidth = 4.0
	}
	glowColor := applyAlpha(AppColors.Accent1, glowAlpha)

	paint.FillShape(gtx.Ops, glowColor, clip.Stroke{
		Path:  pathSpec,
		Width: glowWidth,
	}.Op())

	// Sharp core line
	paint.FillShape(gtx.Ops, strokeColor, clip.Stroke{
		Path:  pathSpec,
		Width: 2.0,
	}.Op())

	// End Point Dots with Pulse
	drawEndDots(gtx, window, lastPt, alpha)
}

// drawEndDots renders the end points of the chart with a circle and a core, including a pulsing animation.
func drawEndDots(gtx layout.Context, window *app.Window, pt f32.Point, alpha uint8) {
	t := float64(time.Now().UnixMilli()) / 1000.0
	pulse := float32(math.Sin(t*4.0)*0.5 + 0.5) // 0..1 pulse

	// Outer Pulse Circle - more subtle in Light Mode
	baseAlpha := 0.3
	if AppColors.Background.R > 200 {
		baseAlpha = 0.40
	}
	pulseAlpha := uint8(float32(alpha) * (1.0 - pulse) * float32(baseAlpha))

	pCircle := clip.Ellipse{
		Min: image.Point{X: int(pt.X) - int(5.0+pulse*10.0), Y: int(pt.Y) - int(5.0+pulse*10.0)},
		Max: image.Point{X: int(pt.X) + int(5.0+pulse*10.0), Y: int(pt.Y) + int(5.0+pulse*10.0)},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(AppColors.Accent1, pulseAlpha), pCircle)

	circle := clip.Ellipse{
		Min: image.Point{X: int(pt.X) - 5, Y: int(pt.Y) - 5},
		Max: image.Point{X: int(pt.X) + 5, Y: int(pt.Y) + 5},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(AppColors.Accent1, alpha), circle)

	core := clip.Ellipse{
		Min: image.Point{X: int(pt.X) - 2, Y: int(pt.Y) - 2},
		Max: image.Point{X: int(pt.X) + 2, Y: int(pt.Y) + 2},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alpha), core)

	// Redraw for animation
	window.Invalidate()
}

// drawChartLabels renders labels for the chart's start, end, and last value.
func drawChartLabels(gtx layout.Context, theme *material.Theme, ctx chartLayoutContext, lc *LineChart, alpha uint8, lang string) {
	// Label color based on theme
	labelCol := color.NRGBA{R: 150, G: 150, B: 160, A: 255}
	if AppColors.Background.R > 200 {
		labelCol = color.NRGBA{R: 60, G: 60, B: 70, A: 255}
	}

	// 1. Last Value Label
	total := len(lc.Data)
	lastVal := lc.Data[total-1]
	lastPt := getPoint(ctx, total-1, total, lastVal)

	valTxt := fmt.Sprintf("%.3f", lastVal)
	lblVal := material.Caption(theme, valTxt)
	lblVal.Color = applyAlpha(AppColors.Text, alpha)
	lblVal.Font.Weight = font.Bold
	lblVal.TextSize = unit.Sp(11)

	{
		labelOffset := image.Point{
			X: int(lastPt.X) - 45,
			Y: int(lastPt.Y) - 20,
		}
		stack := op.Offset(labelOffset).Push(gtx.Ops)
		lblVal.Layout(gtx)
		stack.Pop()
	}

	// 2. Max Value Label
	{
		maxOffset := image.Point{X: int(ctx.paddingX), Y: 15}
		stack := op.Offset(maxOffset).Push(gtx.Ops)

		lbl := material.Caption(theme, fmt.Sprintf("%.3f", ctx.maxVal))
		lbl.Color = applyAlpha(labelCol, 180)
		lbl.TextSize = unit.Sp(10)
		lbl.Layout(gtx)

		stack.Pop()
	}

	startTxt := lc.StartLabel
	endTxt := ""

	if len(lc.Timestamps) >= 2 {
		startTxt = GetFormattedDate(lang, time.Unix(lc.Timestamps[0], 0))
		endTxt = GetFormattedDate(lang, time.Unix(lc.Timestamps[len(lc.Timestamps)-1], 0))
	}

	// Draw Start Label
	if startTxt != "" {
		dateOffset := image.Point{X: int(ctx.paddingX), Y: int(ctx.height) - 20}
		stack := op.Offset(dateOffset).Push(gtx.Ops)

		lbl := material.Caption(theme, startTxt)
		lbl.Color = applyAlpha(labelCol, 200)
		lbl.TextSize = unit.Sp(10)
		lbl.Layout(gtx)

		stack.Pop()
	}

	// Draw End Label
	if endTxt != "" {
		endOffset := image.Point{X: int(ctx.width) - 50, Y: int(ctx.height) - 20}
		stack := op.Offset(endOffset).Push(gtx.Ops)

		lbl := material.Caption(theme, endTxt)
		lbl.Color = applyAlpha(labelCol, 200)
		lbl.TextSize = unit.Sp(10)
		lbl.Layout(gtx)

		stack.Pop()
	}
}

// drawTooltip renders a tooltip for the hovered point on the chart.
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
	lineCol := color.NRGBA{R: 255, G: 255, B: 255, A: 50}
	if AppColors.Background.R > 200 {
		lineCol = color.NRGBA{R: 0, G: 0, B: 0, A: 40}
	}
	paint.FillShape(gtx.Ops, applyAlpha(lineCol, alpha), clip.Rect(lineRect).Op())

	// Hover point circle
	hCircle := clip.Ellipse{
		Min: image.Point{X: int(hoverPt.X) - 4, Y: int(hoverPt.Y) - 4},
		Max: image.Point{X: int(hoverPt.X) + 4, Y: int(hoverPt.Y) + 4},
	}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(AppColors.Accent1, alpha), hCircle)

	// Tooltip Text
	valTxt := fmt.Sprintf("%.3f", hoverVal)
	timeTxt := ""
	if len(lc.Timestamps) > hoverIdx {
		t := time.Unix(lc.Timestamps[hoverIdx], 0)
		timeTxt = GetFormattedDateTime(state.Language, t)
	}

	// Tooltip Layout
	tipWidth := unit.Dp(100)
	tipHeight := unit.Dp(40)
	if timeTxt == "" {
		tipHeight = unit.Dp(26)
	}

	// Calculate pixel dimensions for background
	pxWidth := gtx.Dp(tipWidth)
	pxHeight := gtx.Dp(tipHeight)

	tipOffset := image.Point{
		X: int(hoverPt.X) - (pxWidth / 2),
		Y: int(hoverPt.Y) - pxHeight - gtx.Dp(unit.Dp(10)),
	}

	// Boundary checks
	if tipOffset.X < 0 {
		tipOffset.X = 0
	}
	if tipOffset.X > int(ctx.width)-pxWidth {
		tipOffset.X = int(ctx.width) - pxWidth
	}
	if tipOffset.Y < 0 {
		tipOffset.Y = int(hoverPt.Y) + gtx.Dp(unit.Dp(15))
	}

	op.Offset(tipOffset).Add(gtx.Ops)

	// Tooltip Container
	tipRect := image.Rectangle{Max: image.Point{X: pxWidth, Y: pxHeight}}

	// Draw Background & Border
	{
		radius := unit.Dp(8)
		pxRadius := gtx.Dp(radius)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 10, G: 10, B: 15, A: 252}, clip.UniformRRect(tipRect, pxRadius).Op(gtx.Ops))

		widget.Border{
			Color:        applyAlpha(AppColors.Accent1, 100),
			Width:        unit.Dp(1),
			CornerRadius: radius,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: tipRect.Max}
		})
	}

	// Render Text perfectly centered inside the FIXED box
	{
		gtx.Constraints.Min = tipRect.Max
		gtx.Constraints.Max = tipRect.Max
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(theme, valTxt)
					lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					lbl.Font.Weight = font.Bold
					lbl.TextSize = unit.Sp(13)
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if timeTxt == "" {
						return layout.Dimensions{}
					}
					lbl := material.Caption(theme, timeTxt)
					lbl.Color = color.NRGBA{R: 200, G: 200, B: 210, A: 255}
					lbl.TextSize = unit.Sp(10)
					return lbl.Layout(gtx)
				}),
			)
		})
	}

	resetOffset(tipOffset)
}

// getPoint calculates the x and y coordinates for a given data point on the chart.
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

	for i := range points {
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

	for i := range points {
		data[i] += diff
	}

	return data
}
