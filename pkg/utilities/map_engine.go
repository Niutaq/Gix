package utilities

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Needed for OSM/Carto tiles
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

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
	tileCache     = make(map[string]paint.ImageOp)
	tileCacheMu   sync.RWMutex
	fetching      = make(map[string]bool)
	tileFailUntil = make(map[string]time.Time)
	fetchMu       sync.Mutex

	tileQueue     = make(chan tileJob, 256)
	tileWorkers   sync.Once
	invalidateMu  sync.Mutex
	invalidateDue bool
)

type tileJob struct {
	key   string
	state *AppState
}

const maxTileCacheEntries = 768
const tileFetchWorkers = 8

func ensureTileWorkers() {
	tileWorkers.Do(func() {
		for i := 0; i < tileFetchWorkers; i++ {
			go tileWorker()
		}
	})
}

func scheduleMapInvalidate(state *AppState) {
	invalidateMu.Lock()
	if invalidateDue {
		invalidateMu.Unlock()
		return
	}
	invalidateDue = true
	invalidateMu.Unlock()
	go func() {
		time.Sleep(40 * time.Millisecond)
		invalidateMu.Lock()
		invalidateDue = false
		invalidateMu.Unlock()
		if state != nil && state.Window != nil {
			state.Window.Invalidate()
		}
	}()
}

// LayoutMap renders the OSM map as a background or standalone component.
func LayoutMap(gtx layout.Context, window *app.Window, state *AppState) layout.Dimensions {
	ensureTileWorkers()
	if state.UI.MapState.Zoom == 0 {
		state.UI.MapState.Zoom = 13 // Default Zoom level
	}

	// Handle Dragging and Panning
	handleMapEvents(gtx, window, state)

	// Apply MapFocus once (city search / pin), then release so panning doesn't snap back.
	var lat, lon float64
	if state.UI.MapFocus.Latitude != 0 || state.UI.MapFocus.Longitude != 0 {
		lat, lon = state.UI.MapFocus.Latitude, state.UI.MapFocus.Longitude
		state.UI.MapState.CenterLat = lat
		state.UI.MapState.CenterLon = lon
		state.UI.MapFocus.Latitude = 0
		state.UI.MapFocus.Longitude = 0
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

// markerFrame holds the per-frame values shared by all map pins.
type markerFrame struct {
	halfW, halfH float64
	ctx, cty     float64
	tileSize     float64
	zoom         int
	breath       float64
	hasTop       bool
}

// visibleMarkerIDs returns the cantor IDs to pin: the filtered list, or on-map cantors with rates.
func visibleMarkerIDs(state *AppState) []string {
	state.CantorsMu.RLock()
	defer state.CantorsMu.RUnlock()
	ids := append([]string(nil), state.UI.FilteredIDs...)
	if len(ids) == 0 {
		// List not built yet — show on-map cantors that already have rates.
		for id, c := range state.Cantors {
			if c != nil && isWithinDistance(state, c) && hasUsableCantorRates(state, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// isTopMarkerRate reports whether a buy/sell pair matches the best kupno / sprzedaż within eps.
func isTopMarkerRate(buy, sell, bestBuy, bestSell, eps float64) bool {
	return (bestBuy > 0 && buy+eps >= bestBuy) ||
		(bestSell > 0 && sell > 0 && sell-eps <= bestSell)
}

// topMarkerIDs returns the pins holding the best rates — same TOP definition as the list.
func topMarkerIDs(state *AppState, ids []string) map[string]bool {
	// Same TOP definition as the list: best kupno / sprzedaż among visible pins with rates.
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()
	rates := state.Vault.Rates[state.UI.Currency]
	bestBuy, bestSell := calculateBestRatesAmong(rates, ids)
	topIDs := map[string]bool{}
	const eps = 0.0005
	for _, id := range ids {
		entry := rates[id]
		if entry == nil || entry.Error != "" {
			continue
		}
		buy := parseRate(entry.Rate.BuyRate)
		sell := parseRate(entry.Rate.SellRate)
		if isTopMarkerRate(buy, sell, bestBuy, bestSell, eps) {
			topIDs[id] = true
		}
	}
	return topIDs
}

// drawTopPinIcon draws a TOP pin: subtle gold pulse — darker amber in light theme for dock-like contrast.
func drawTopPinIcon(gtx layout.Context, state *AppState, breath float64) {
	topHue := AppColors.Accent1
	if state.UI.LightMode {
		topHue = AppColors.Accent1Dark
	}
	halo := topHue
	halo.A = uint8(10 + breath*16)
	if state.UI.LightMode {
		halo.A = uint8(36 + breath*40)
	}
	haloCircle := clip.Ellipse{Min: image.Point{X: -4, Y: -4}, Max: image.Point{X: 28, Y: 28}}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, halo, haloCircle)
	pin := topHue
	pin.A = uint8(220 + breath*35)
	if state.UI.LightMode {
		pin.A = 255
	}
	DrawIconMap(gtx, pin)
}

// drawMutedPinIcon draws a quieter pin for other cantors in the area.
func drawMutedPinIcon(gtx layout.Context, state *AppState) {
	muted := AppColors.Secondary
	if state.UI.LightMode {
		muted = color.NRGBA{R: 150, G: 150, B: 158, A: 210}
	} else {
		muted.A = 190
	}
	DrawIconMap(gtx, muted)
}

// drawMapPin draws the shadow and icon of a single cantor pin (skipped when off-screen).
func drawMapPin(gtx layout.Context, state *AppState, f markerFrame, id string, isTop bool) {
	state.CantorsMu.RLock()
	cantor, ok := state.Cantors[id]
	state.CantorsMu.RUnlock()
	if !ok || cantor == nil {
		return
	}

	cx, cy := latLonToTile(cantor.Latitude, cantor.Longitude, f.zoom)
	posX := f.halfW + (cx-f.ctx)*f.tileSize
	posY := f.halfH + (cy-f.cty)*f.tileSize

	if posX < -50 || posX > float64(gtx.Constraints.Max.X)+50 || posY < -50 || posY > float64(gtx.Constraints.Max.Y)+50 {
		return
	}

	shadowStack := op.Offset(image.Point{X: int(posX), Y: int(posY)}).Push(gtx.Ops)
	shadowCircle := clip.Ellipse{Min: image.Point{X: -6, Y: -3}, Max: image.Point{X: 6, Y: 3}}.Op(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{A: 100}, shadowCircle)
	shadowStack.Pop()

	stack := op.Offset(image.Point{X: int(posX) - 12, Y: int(posY) - 24}).Push(gtx.Ops)
	defer stack.Pop()

	switch {
	case isTop:
		drawTopPinIcon(gtx, state, f.breath)
	case f.hasTop:
		// Other cantors in the area stay readable but quieter.
		drawMutedPinIcon(gtx, state)
	default:
		// No rates yet — normal pins (don't mute the whole city).
		DrawIconMap(gtx, AppColors.Accent1)
	}
}

// renderMarkers renders the markers on the map
func renderMarkers(gtx layout.Context, state *AppState, ctx, cty float64, tileSize float64, zoom int) {
	ids := visibleMarkerIDs(state)
	topIDs := topMarkerIDs(state, ids)

	hasTop := len(topIDs) > 0
	phase := float64(time.Now().UnixNano()%3_600_000_000) / 3_600_000_000
	breath := 0.5 + 0.5*math.Sin(phase*2*math.Pi)
	if hasTop {
		gtx.Execute(op.InvalidateCmd{})
	}

	frame := markerFrame{
		halfW:    float64(gtx.Constraints.Max.X) / 2,
		halfH:    float64(gtx.Constraints.Max.Y) / 2,
		ctx:      ctx,
		cty:      cty,
		tileSize: tileSize,
		zoom:     zoom,
		breath:   breath,
		hasTop:   hasTop,
	}

	// Non-TOP first, TOP last so winners paint on top when pins overlap.
	for _, id := range ids {
		if !topIDs[id] {
			drawMapPin(gtx, state, frame, id, false)
		}
	}
	for _, id := range ids {
		if topIDs[id] {
			drawMapPin(gtx, state, frame, id, true)
		}
	}
}

// renderTile renders a single tile on the map
func renderTile(gtx layout.Context, x, y, z int, px, py float32, tileSize int, state *AppState) {
	theme := "dark"
	if state.UI.LightMode {
		theme = "light"
	}
	// "v8" — linear filter + parent underlay (smoother load, fewer seams).
	key := fmt.Sprintf("v8/%s/%d/%d/%d", theme, z, x, y)

	tileCacheMu.RLock()
	img, ok := tileCache[key]
	var parentImg paint.ImageOp
	var parentOK bool
	if !ok && z > 0 {
		pkey := fmt.Sprintf("v8/%s/%d/%d/%d", theme, z-1, x/2, y/2)
		parentImg, parentOK = tileCache[pkey]
	}
	tileCacheMu.RUnlock()

	// Overlap 2px + subpixel affine — kills hairline seams while panning.
	drawSize := float32(tileSize + 2)
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(px, py))).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: image.Point{X: int(drawSize), Y: int(drawSize)}}.Push(gtx.Ops).Pop()

	if ok {
		paintTileScaled(gtx, img, drawSize)
		return
	}

	enqueueTileFetch(key, state)

	if parentOK {
		// Parent covers 2×2 children; shift so the matching quadrant sits in this clip.
		ox := -float32(x%2) * drawSize
		oy := -float32(y%2) * drawSize
		stack := op.Affine(f32.Affine2D{}.Offset(f32.Pt(ox, oy))).Push(gtx.Ops)
		paintTileScaled(gtx, parentImg, drawSize*2)
		stack.Pop()
		return
	}

	bgColor := color.NRGBA{R: 25, G: 25, B: 30, A: 255}
	if state.UI.LightMode {
		bgColor = color.NRGBA{R: 235, G: 235, B: 240, A: 255}
	}
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: image.Point{X: int(drawSize), Y: int(drawSize)}}.Op())
}

func paintTileScaled(gtx layout.Context, img paint.ImageOp, drawSize float32) {
	sz := img.Size()
	if sz.X <= 0 || sz.Y <= 0 {
		return
	}
	img.Filter = paint.FilterLinear
	img.Add(gtx.Ops)
	op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(
		drawSize/float32(sz.X),
		drawSize/float32(sz.Y),
	))).Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func looksLikeMapAPIKey(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 20 || len(s) > 128 {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "optional") || strings.Contains(lower, "carto") ||
		strings.Contains(lower, "maptiler") || strings.Contains(lower, " ") ||
		strings.Contains(lower, "your_api") {
		return false
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func enqueueTileFetch(key string, state *AppState) {
	fetchMu.Lock()
	if fetching[key] {
		fetchMu.Unlock()
		return
	}
	if until, ok := tileFailUntil[key]; ok && time.Now().Before(until) {
		fetchMu.Unlock()
		return
	}
	fetching[key] = true
	fetchMu.Unlock()

	job := tileJob{key: key, state: state}
	select {
	case tileQueue <- job:
	default:
		// Queue full — allow retry on a later frame.
		fetchMu.Lock()
		delete(fetching, key)
		fetchMu.Unlock()
	}
}

func tileWorker() {
	client := &http.Client{Timeout: 8 * time.Second}
	for job := range tileQueue {
		ok := fetchOneTile(client, job.key, job.state)
		fetchMu.Lock()
		delete(fetching, job.key)
		if !ok {
			tileFailUntil[job.key] = time.Now().Add(1500 * time.Millisecond)
			// Keep asking the UI to retry failed edge tiles.
			scheduleMapInvalidate(job.state)
		} else {
			delete(tileFailUntil, job.key)
		}
		fetchMu.Unlock()
		if ok {
			scheduleMapInvalidate(job.state)
		}
	}
}

// providerTileURL returns the tile URL of an API-key based provider, or "" when it is not usable.
func providerTileURL(provider, theme, z, x, y string, state *AppState) string {
	switch provider {
	case "carto":
		cartoKey := strings.TrimSpace(os.Getenv("CARTO_API_KEY"))
		if ed := strings.TrimSpace(state.UI.MapTilerKeyEditor.Text()); looksLikeMapAPIKey(ed) {
			cartoKey = ed
		}
		if !looksLikeMapAPIKey(cartoKey) {
			return ""
		}
		style := "rastertiles/voyager_labels_under"
		if theme == "dark" {
			style = "dark_all"
		}
		return fmt.Sprintf("https://basemaps.cartocdn.com/%s/%s/%s/%s.png?key=%s", style, z, x, y, cartoKey)
	case "maptiler":
		apiKey := strings.TrimSpace(state.UI.MapTilerAPIKey)
		if ed := strings.TrimSpace(state.UI.MapTilerKeyEditor.Text()); looksLikeMapAPIKey(ed) {
			apiKey = ed
		}
		if !looksLikeMapAPIKey(apiKey) {
			return ""
		}
		return fmt.Sprintf("https://api.maptiler.com/maps/streets-v2/256/%s/%s/%s.png?key=%s", z, x, y, apiKey)
	}
	return ""
}

// buildTileURLs lists candidate tile sources in priority order.
func buildTileURLs(theme, z, x, y string, state *AppState) []string {
	urls := []string{
		fmt.Sprintf("https://tile.openstreetmap.org/%s/%s/%s.png", z, x, y),
		fmt.Sprintf("https://tile.openstreetmap.de/%s/%s/%s.png", z, x, y),
		fmt.Sprintf("https://a.tile.openstreetmap.fr/hot/%s/%s/%s.png", z, x, y),
		fmt.Sprintf("https://b.tile.openstreetmap.fr/hot/%s/%s/%s.png", z, x, y),
		// Esri World Street (raster FOSS-friendly fallback; note z/y/x order).
		fmt.Sprintf("https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/%s/%s/%s", z, y, x),
		fmt.Sprintf("https://a.tile.openstreetmap.fr/osmfr/%s/%s/%s.png", z, x, y),
	}

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MAP_TILE_PROVIDER")))
	if providerURL := providerTileURL(provider, theme, z, x, y, state); providerURL != "" {
		urls = append([]string{providerURL}, urls...)
	}
	return urls
}

// downloadTileImage fetches and validates a single 256×256 tile image.
func downloadTileImage(client *http.Client, tileURL string) (image.Image, bool) {
	req, err := http.NewRequest("GET", tileURL, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", UserAgentApp+" (local map; github.com/Niutaq/Gix)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	// Reject Carto keyless watermark (~2.5KB) and other tiny error PNGs.
	if readErr != nil || resp.StatusCode != http.StatusOK || len(data) < 4000 {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	if b.Dx() != 256 || b.Dy() != 256 {
		return nil, false
	}
	return img, true
}

// storeTileImage caches a decoded tile, evicting one random entry when the cache is full.
func storeTileImage(key string, img image.Image) {
	tileCacheMu.Lock()
	defer tileCacheMu.Unlock()
	if len(tileCache) >= maxTileCacheEntries {
		for oldKey := range tileCache {
			delete(tileCache, oldKey)
			break
		}
	}
	opImg := paint.NewImageOp(img)
	opImg.Filter = paint.FilterLinear
	tileCache[key] = opImg
}

// fetchOneTile loads one FOSS raster tile. Default: OpenStreetMap (no API key).
func fetchOneTile(client *http.Client, key string, state *AppState) bool {
	parts := strings.Split(key, "/")
	if len(parts) != 5 { // v7/theme/z/x/y
		return false
	}
	theme, z, x, y := parts[1], parts[2], parts[3], parts[4]

	for _, tileURL := range buildTileURLs(theme, z, x, y, state) {
		img, ok := downloadTileImage(client, tileURL)
		if !ok {
			continue
		}
		storeTileImage(key, img)
		return true
	}
	return false
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
