package utilities

import (
	// Standard libraries
	"fmt"
	"image"
	"image/color"
	"math"

	// Gio utilities
	"gioui.org/f32"
	"gioui.org/font"
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
	StartLabel string
}

// Layout renders the line chart within the given constraints.
func (lc *LineChart) Layout(gtx layout.Context, theme *material.Theme, alpha uint8) layout.Dimensions {
	width := float32(gtx.Constraints.Max.X)
	height := float32(gtx.Constraints.Max.Y)

	if len(lc.Data) < 2 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	applyAlpha := func(c color.NRGBA, a uint8) color.NRGBA {
		newA := uint16(c.A) * uint16(a) / 255
		c.A = uint8(newA)
		return c
	}

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
	padding := rangeVal * 0.25
	minPlotVal := minVal - padding
	maxPlotVal := maxVal + padding
	plotRange := maxPlotVal - minPlotVal

	gridColor := applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 15}, alpha)
	for _, p := range []float32{0.0, 0.33, 0.66, 1.0} {
		y := height - (p * height)
		stack := clip.Rect{Min: image.Point{0, int(y)}, Max: image.Point{int(width), int(y) + 1}}.Push(gtx.Ops)
		paint.ColorOp{Color: gridColor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}

	var path clip.Path
	path.Begin(gtx.Ops)

	getPoint := func(i int, val float64) f32.Point {
		x := (float32(i) / float32(len(lc.Data)-1)) * width
		normalizedY := (val - minPlotVal) / plotRange
		y := height - (float32(normalizedY) * height)
		return f32.Point{X: x, Y: y}
	}

	startPt := getPoint(0, lc.Data[0])
	path.MoveTo(startPt)
	for i := 1; i < len(lc.Data); i++ {
		path.LineTo(getPoint(i, lc.Data[i]))
	}
	path.LineTo(f32.Point{X: width, Y: height})
	path.LineTo(f32.Point{X: 0, Y: height})
	path.Close()

	fillColor := applyAlpha(color.NRGBA{R: AppColors.Accent1.R, G: AppColors.Accent1.G, B: AppColors.Accent1.B, A: 30}, alpha)
	paint.FillShape(gtx.Ops, fillColor, clip.Outline{Path: path.End()}.Op())

	var linePath clip.Path
	linePath.Begin(gtx.Ops)
	linePath.MoveTo(startPt)
	for i := 1; i < len(lc.Data); i++ {
		linePath.LineTo(getPoint(i, lc.Data[i]))
	}
	strokeColor := applyAlpha(AppColors.Accent1, alpha)
	paint.FillShape(gtx.Ops, strokeColor, clip.Stroke{Path: linePath.End(), Width: 2.0}.Op())

	resetOffset := func(p image.Point) {
		op.Offset(p.Mul(-1)).Add(gtx.Ops)
	}

	lastIdx := len(lc.Data) - 1
	lastVal := lc.Data[lastIdx]
	pt := getPoint(lastIdx, lastVal)

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

	valTxt := fmt.Sprintf("%.4f", lastVal)
	lblVal := material.Caption(theme, valTxt)
	lblVal.Color = applyAlpha(color.NRGBA{R: 255, G: 255, B: 255, A: 255}, alpha)
	lblVal.Font.Weight = font.Bold
	lblVal.TextSize = unit.Sp(11)

	labelOffset := image.Point{
		X: int(pt.X) - 60,
		Y: int(pt.Y) - 25,
	}
	if pt.Y < 40 {
		labelOffset.Y = int(pt.Y) + 15
	}

	op.Offset(labelOffset).Add(gtx.Ops)
	bgRect := clip.UniformRRect(image.Rectangle{Max: image.Point{X: 55, Y: 18}}, 4).Op(gtx.Ops)
	paint.FillShape(gtx.Ops, applyAlpha(color.NRGBA{R: 20, G: 20, B: 25, A: 200}, alpha), bgRect)
	layout.Inset{Left: unit.Dp(4), Top: unit.Dp(1)}.Layout(gtx, lblVal.Layout)
	resetOffset(labelOffset) // RESET OFFSET!

	layout.Inset{Top: unit.Dp(4), Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(theme, fmt.Sprintf("%.4f", maxVal))
		lbl.Color = applyAlpha(color.NRGBA{R: 200, G: 200, B: 200, A: 180}, alpha)
		lbl.TextSize = unit.Sp(10)
		return lbl.Layout(gtx)
	})

	minOffset := image.Point{X: 0, Y: int(height) - 35}
	op.Offset(minOffset).Add(gtx.Ops)
	layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(theme, fmt.Sprintf("%.4f", minVal))
		lbl.Color = applyAlpha(color.NRGBA{R: 200, G: 200, B: 200, A: 180}, alpha)
		lbl.TextSize = unit.Sp(10)
		return lbl.Layout(gtx)
	})
	resetOffset(minOffset) // RESET OFFSET!

	if lc.StartLabel != "" {
		dateOffset := image.Point{X: 0, Y: int(height) - 15}
		op.Offset(dateOffset).Add(gtx.Ops)
		layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(theme, lc.StartLabel)
			lbl.Color = applyAlpha(color.NRGBA{R: 150, G: 150, B: 160, A: 150}, alpha)
			lbl.TextSize = unit.Sp(9)
			return lbl.Layout(gtx)
		})
		resetOffset(dateOffset) // RESET OFFSET!
	}

	return layout.Dimensions{Size: gtx.Constraints.Max}
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
