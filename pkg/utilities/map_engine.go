package utilities

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Needed for OSM tiles
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

var (
	tileCache   = make(map[string]paint.ImageOp)
	tileCacheMu sync.RWMutex
	fetching    = make(map[string]bool)
	fetchMu     sync.Mutex
)

// LayoutMap renders the OSM map as a background or standalone component.
func LayoutMap(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	if state.UI.MapState.Zoom == 0 {
		state.UI.MapState.Zoom = 13 // Default Zoom level
	}

	// Handle Dragging and Panning
	handleMapEvents(gtx, window, state)

	// Determine center
	var lat, lon float64
	if state.UI.MapFocus.Latitude != 0 {
		lat, lon = state.UI.MapFocus.Latitude, state.UI.MapFocus.Longitude
		state.UI.MapState.CenterLat = lat
		state.UI.MapState.CenterLon = lon
	} else {
		lat = state.UI.MapState.CenterLat
		lon = state.UI.MapState.CenterLon
	}

	// Clamp Latitude to avoid Mercator infinities
	if lat > 85 {
		lat = 85
	}
	if lat < -85 {
		lat = -85
	}

	if lat == 0 && lon == 0 {
		lat, lon = 52.2297, 21.0122 // Warsaw
		state.UI.MapState.CenterLat, state.UI.MapState.CenterLon = lat, lon
	}

	zoom := int(state.UI.MapState.Zoom)
	const tileSize = 256.0
	halfW := float64(gtx.Constraints.Max.X) / 2
	halfH := float64(gtx.Constraints.Max.Y) / 2

	tx, ty := latLonToTile(lat, lon, zoom)

	bgCol := color.NRGBA{R: 18, G: 18, B: 22, A: 255}
	if state.UI.LightMode {
		bgCol = color.NRGBA{R: 240, G: 240, B: 245, A: 255}
	}
	paint.FillShape(gtx.Ops, bgCol, clip.Rect{Max: gtx.Constraints.Max}.Op())

	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &state.UI.MapState)

	cols := int(float64(gtx.Constraints.Max.X)/tileSize) + 4
	rows := int(float64(gtx.Constraints.Max.Y)/tileSize) + 4

	for i := -cols/2 - 1; i <= cols/2+1; i++ {
		for j := -rows/2 - 1; j <= rows/2+1; j++ {
			ix, iy := int(math.Floor(tx))+i, int(math.Floor(ty))+j
			n := int(math.Pow(2, float64(zoom)))
			if ix < 0 || ix >= n || iy < 0 || iy >= n {
				continue
			}

			posX := halfW + (float64(ix)-tx)*tileSize
			posY := halfH + (float64(iy)-ty)*tileSize

			renderTile(gtx, ix, iy, zoom, float32(posX), float32(posY), int(tileSize), state)
		}
	}

	renderMarkers(gtx, state, tx, ty, tileSize, zoom)

	// Zoom Controls Overlay
	layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.UI.MapState.ZoomInBtn.Clicked(gtx) {
						state.UI.MapState.Zoom += 1.0
						if state.UI.MapState.Zoom > 19 {
							state.UI.MapState.Zoom = 19
						}
						if window != nil {
							window.Invalidate()
						}
					}
					return state.UI.MapState.ZoomInBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return renderMapZoomBtn(gtx, "+", state)
					})
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if state.UI.MapState.ZoomOutBtn.Clicked(gtx) {
						state.UI.MapState.Zoom -= 1.0
						if state.UI.MapState.Zoom < 2 {
							state.UI.MapState.Zoom = 2
						}
						if window != nil {
							window.Invalidate()
						}
					}
					return state.UI.MapState.ZoomOutBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return renderMapZoomBtn(gtx, "-", state)
					})
				}),
			)
		})
	})

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// renderMapZoomBtn renders buttons for the map
func renderMapZoomBtn(gtx layout.Context, symbol string, state *AppState) layout.Dimensions {
	size := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min = image.Point{X: size, Y: size}
	gtx.Constraints.Max = gtx.Constraints.Min

	bgCol := color.NRGBA{R: 255, G: 255, B: 255, A: 220}
	iconCol := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	if !state.UI.LightMode {
		bgCol = color.NRGBA{R: 40, G: 40, B: 50, A: 220}
		iconCol = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(8))
			paint.FillShape(gtx.Ops, bgCol, rect.Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Inside Stacked, Min constraints are 0. Use Max or hardcode the expected inner size (16dp).
				s := float32(gtx.Constraints.Max.X)
				if s > float32(gtx.Dp(unit.Dp(16))) {
					s = float32(gtx.Dp(unit.Dp(16)))
				}

				var path clip.Path
				path.Begin(gtx.Ops)

				// Horizontal line (for both + and -)
				path.MoveTo(f32.Point{X: 0, Y: s / 2})
				path.LineTo(f32.Point{X: s, Y: s / 2})

				// Vertical line (only for +)
				if symbol == "+" {
					path.MoveTo(f32.Point{X: s / 2, Y: 0})
					path.LineTo(f32.Point{X: s / 2, Y: s})
				}

				paint.FillShape(gtx.Ops, iconCol, clip.Stroke{Path: path.End(), Width: float32(gtx.Dp(unit.Dp(2)))}.Op())
				return layout.Dimensions{Size: image.Point{X: int(s), Y: int(s)}}
			})
		}),
	)
}

// handleMapEvents handles pointer events on the map
func handleMapEvents(gtx layout.Context, window *app.Window, state *AppState) {
	for {
		e, ok := gtx.Event(pointer.Filter{
			Target: &state.UI.MapState,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
		})
		if !ok {
			break
		}

		ev, ok := e.(pointer.Event)
		if !ok {
			continue
		}

		switch ev.Kind {
		case pointer.Scroll:
			state.UI.MapState.Zoom -= ev.Scroll.Y / 150.0
			if state.UI.MapState.Zoom < 2 {
				state.UI.MapState.Zoom = 2
			}
			if state.UI.MapState.Zoom > 19 {
				state.UI.MapState.Zoom = 19
			}
			if window != nil {
				window.Invalidate()
			}
		case pointer.Press:
			state.UI.MapState.DragStart = ev.Position
			state.UI.MapState.Dragging = true
			state.UI.MapFocus.Latitude = 0
			state.UI.MapFocus.Longitude = 0
		case pointer.Drag:
			if state.UI.MapState.Dragging {
				diff := ev.Position.Sub(state.UI.MapState.DragStart)
				state.UI.MapState.DragStart = ev.Position

				zoom := int(state.UI.MapState.Zoom)
				tx, ty := latLonToTile(state.UI.MapState.CenterLat, state.UI.MapState.CenterLon, zoom)

				const tileSize = 256.0
				tx -= float64(diff.X) / tileSize
				ty -= float64(diff.Y) / tileSize

				state.UI.MapState.CenterLat, state.UI.MapState.CenterLon = tileToLatLon(tx, ty, zoom)
				if window != nil {
					window.Invalidate()
				}
			}
		case pointer.Release:
			state.UI.MapState.Dragging = false
			if state.Search != nil {
				go func(lat, lon float64) {
					cantors, err := state.Search.SearchCantorsNearby(lat, lon, 30.0)
					if err == nil && len(cantors) > 0 {
						state.CantorsMu.Lock()
						var currentIDs []string
						for _, c := range cantors {
							idStr := fmt.Sprintf("%d", c.ID)
							if _, exists := state.Cantors[idStr]; !exists {
								state.Cantors[idStr] = &CantorInfo{
									ID:          c.ID,
									DisplayName: c.DisplayName,
									Latitude:    c.Location.Lat,
									Longitude:   c.Location.Lon,
								}
							}
							currentIDs = append(currentIDs, idStr)
						}
						state.UI.FilteredIDs = currentIDs
						state.CantorsMu.Unlock()
						if window != nil {
							window.Invalidate()
						}
					}
				}(state.UI.MapState.CenterLat, state.UI.MapState.CenterLon)
			}
		}
	}
}

// renderMarkers renders the markers on the map
func renderMarkers(gtx layout.Context, state *AppState, ctx, cty float64, tileSize float64, zoom int) {
	halfW := float64(gtx.Constraints.Max.X) / 2
	halfH := float64(gtx.Constraints.Max.Y) / 2

	state.CantorsMu.RLock()
	ids := state.UI.FilteredIDs
	// We iterate over a slice while holding the lock to be safe
	for _, id := range ids {
		cantor, ok := state.Cantors[id]
		if !ok {
			continue
		}

		cx, cy := latLonToTile(cantor.Latitude, cantor.Longitude, zoom)
		posX := halfW + (cx-ctx)*tileSize
		posY := halfH + (cy-cty)*tileSize

		if posX < -50 || posX > float64(gtx.Constraints.Max.X)+50 || posY < -50 || posY > float64(gtx.Constraints.Max.Y)+50 {
			continue
		}

		shadowStack := op.Offset(image.Point{X: int(posX), Y: int(posY)}).Push(gtx.Ops)
		shadowCircle := clip.Ellipse{Min: image.Point{X: -6, Y: -3}, Max: image.Point{X: 6, Y: 3}}.Op(gtx.Ops)
		paint.FillShape(gtx.Ops, color.NRGBA{A: 100}, shadowCircle)
		shadowStack.Pop()

		stack := op.Offset(image.Point{X: int(posX) - 12, Y: int(posY) - 24}).Push(gtx.Ops)
		circle := clip.Ellipse{Min: image.Point{X: -1, Y: -1}, Max: image.Point{X: 25, Y: 25}}.Op(gtx.Ops)
		paint.FillShape(gtx.Ops, color.NRGBA{A: 50}, circle)
		DrawIconMap(gtx, AppColors.Accent1)
		stack.Pop()
	}
	state.CantorsMu.RUnlock()
}

// renderTile renders a single tile on the map
func renderTile(gtx layout.Context, x, y, z int, px, py float32, tileSize int, state *AppState) {
	theme := "dark"
	if state.UI.LightMode {
		theme = "light"
	}
	key := fmt.Sprintf("%s/%d/%d/%d", theme, z, x, y)

	tileCacheMu.RLock()
	img, ok := tileCache[key]
	tileCacheMu.RUnlock()

	stack := op.Offset(image.Point{X: int(px), Y: int(py)}).Push(gtx.Ops)
	if ok {
		img.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	} else {
		fetchTileAsync(key, state)
		bgColor := color.NRGBA{R: 25, G: 25, B: 30, A: 255}
		if state.UI.LightMode {
			bgColor = color.NRGBA{R: 235, G: 235, B: 240, A: 255}
		}
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: image.Point{X: 256, Y: 256}}.Op())
	}
	stack.Pop()
}

// fetchTileAsync fetches tiles asynchronously
func fetchTileAsync(key string, state *AppState) {
	fetchMu.Lock()
	if fetching[key] {
		fetchMu.Unlock()
		return
	}
	fetching[key] = true
	fetchMu.Unlock()

	go func() {
		parts := strings.Split(key, "/")
		if len(parts) != 4 {
			return
		}
		theme, z, x, y := parts[0], parts[1], parts[2], parts[3]
		tileSource := "rastertiles/voyager_labels_under"
		if theme == "dark" {
			tileSource = "dark_all"
		}
		url := fmt.Sprintf("https://basemaps.cartocdn.com/%s/%s/%s/%s.png", tileSource, z, x, y)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", UserAgentApp)

		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			data, _ := io.ReadAll(resp.Body)
			img, _, err := image.Decode(bytes.NewReader(data))
			if err == nil {
				tileCacheMu.Lock()
				tileCache[key] = paint.NewImageOp(img)
				tileCacheMu.Unlock()
				if state.Window != nil {
					state.Window.Invalidate()
				}
			}
		}
	}()
}

// latLonToTile is a function that re-tiles latitude and longitude into tile values
func latLonToTile(lat, lon float64, zoom int) (float64, float64) {
	latRad := lat * math.Pi / 180
	n := math.Pow(2, float64(zoom))
	x := (lon + 180) / 360 * n
	y := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n
	return x, y
}

// tileToLatLon is a function that tiles coords into latitude and longitude values
func tileToLatLon(x, y float64, zoom int) (float64, float64) {
	n := math.Pow(2, float64(zoom))
	lon := x/n*360.0 - 180.0
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*y/n)))
	lat := latRad * 180.0 / math.Pi
	return lat, lon
}
