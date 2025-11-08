/*
               __'                     ',:'/¯/`:,             __        '        _  '
         ,.·:'´::::::::`'·-.             /:/_/::::/';'      .:'´:::::/`:·.          /.;/';°
         '/::::::::::::::::::';           /:'     '`:/::;‘   '/::::::::/:::::/`:,     /::/:`'; °
      /;:· '´ ¯¯  `' ·-:::/'           ;         ';:';‘  /· '´ ¯¯ `'~·./:::::`;:´¯'`:;:/'
      /.'´      _         ';/' ‘          |         'i::i   '`·.             `·:;:'/      ,'/' '  ‚
   ,:     ,:'´::;'`·.,_.·'´.,    ‘       ';        ;'::i       `·.            '`'      ,·' '  '  ‚
   /     /':::::/;::::_::::::::;‘        'i        'i::i'          ';              .,·'´   °
   ,'     ;':::::'/·´¯     ¯'`·;:::¦‘        ;       'i::;'       ,·´               i:';
   'i     ';::::::'\             ';:';‘        ';       i:/'     ,·´      ,           ';::'`:., °
   ;      '`·:;:::::`'*;:'´      |/'          ';     ;/ °    ,'      ,':´';           ';::::::'`:*;'
   \          '`*^*'´         /'  ‘          ';   / °      i      ';::/ '`·,         '`·:;:::::/
      `·.,               ,.-·´                `'´       °  ';      ';/     '`·.,          '`*;/
         '`*^~·~^*'´                       ‘            '`~-·'´            `*^·–·^*'´

*/

package main

import (
	// Main libraries
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	// Go files
	"github.com/Niutaq/Gix/pkg/reading_data"
	"github.com/Niutaq/Gix/pkg/utilities"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Global variables
//
//go:embed res/background_2k.png
var backgroundImageFS embed.FS

var (
	originalBackgroundImage  image.Image
	paintableBackgroundImage paint.ImageOp
	backgroundImageLoaded    bool
)

// Definiujemy stałe dla naszego API
const (
	apiBaseURL   = "http://localhost:8080"
	apiCantors   = apiBaseURL + "/api/v1/cantors"
	apiRatesBase = apiBaseURL + "/api/v1/rates"
)

// ApiCantorResponse - a structure for parsing data from /api/v1/cantors
type ApiCantorResponse struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
}

// Functions
// ++++++++++++++++++++ MAIN Function ++++++++++++++++++++
func main() {
	window := new(app.Window)
	window.Option(
		app.Title("Gix"),
		app.Size(unit.Dp(1280), unit.Dp(1280)),
	)

	go func() {
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

// initBackgroundImage - a function for a background image initialization
func initBackgroundImage() {
	if backgroundImageLoaded {
		return
	}
	fileData, err := backgroundImageFS.ReadFile("res/background_2k.png")
	if err != nil {
		log.Printf("Warning: Failed to read embedded background image: %v", err)
		return
	}

	img, _, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		log.Printf("Warning: Failed to decode embedded background image: %v", err)
		return
	}

	originalBackgroundImage = img
	paintableBackgroundImage = paint.NewImageOp(originalBackgroundImage)
	backgroundImageLoaded = true
	log.Println("Background image loaded successfully.")
}

// loadFontCollection - a function for font handling
func loadFontCollection() ([]font.FontFace, error) {
	var fontCollection []font.FontFace

	fontPaths := []string{
		"fonts/NotoSans-Regular.ttf",
	}

	for _, path := range fontPaths {
		face, err := reading_data.LoadAndParseFont(path)
		if err != nil {
			return nil, err
		}
		fontCollection = append(fontCollection, face)
	}

	return fontCollection, nil
}

// renderBackground - a function for rendering background
func renderBackground(gtx layout.Context, ops *op.Ops) {
	if backgroundImageLoaded && originalBackgroundImage != nil {
		imgBounds := originalBackgroundImage.Bounds()
		imgWidth := float32(imgBounds.Dx())
		imgHeight := float32(imgBounds.Dy())

		winWidth := float32(gtx.Constraints.Max.X)
		winHeight := float32(gtx.Constraints.Max.Y)

		if imgWidth == 0 || imgHeight == 0 || winWidth == 0 || winHeight == 0 {
			paint.Fill(gtx.Ops, utilities.AppColors.Background)
		} else {
			scaleX := winWidth / imgWidth
			scaleY := winHeight / imgHeight

			var finalScale float32
			var offsetX, offsetY float32

			if scaleX > scaleY {
				finalScale = scaleX
				scaledImgHeight := imgHeight * finalScale
				offsetY = (winHeight - scaledImgHeight) / 2
				offsetX = 0
			} else {
				finalScale = scaleY
				scaledImgWidth := imgWidth * finalScale
				offsetX = (winWidth - scaledImgWidth) / 2
				offsetY = 0
			}

			transform := op.Affine(f32.Affine2D{}.
				Scale(f32.Pt(0, 0), f32.Pt(finalScale, finalScale)).
				Offset(f32.Pt(offsetX, offsetY)))

			stack := transform.Push(gtx.Ops)

			paintableBackgroundImage.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			stack.Pop()
		}
	} else {
		paint.Fill(gtx.Ops, utilities.AppColors.Background)
	}
}

// handleCantorClicks - a function that handles clicks on cantors
func handleCantorClicks(gtx layout.Context, window *app.Window, state *utilities.AppState) {
	cantorKeys := make([]string, 0, len(state.Cantors))
	for key := range state.Cantors {
		cantorKeys = append(cantorKeys, key)
	}
	sort.Strings(cantorKeys)

	for _, key := range cantorKeys {
		cantorName := key
		cantor := state.Cantors[cantorName]

		if cantor.Button.Clicked(gtx) {
			if state.UI.SelectedCantor == cantorName {
				state.UI.SelectedCantor = ""
				state.Vault.LastEntry = nil
			} else {
				state.UI.SelectedCantor = cantorName
				state.IsLoading.Store(true)
				state.IsLoadingStart = time.Now()
				state.Vault.LastEntry = nil

				go func(w *app.Window, cInfo *utilities.CantorInfo, currentCurrency string,
					currentAppState *utilities.AppState) {

					ratesURL := fmt.Sprintf(
						"%s?cantor_id=%d&currency=%s",
						apiRatesBase,
						cInfo.ID,
						currentCurrency,
					)

					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					var rates utilities.ExchangeRates
					var fetchErr error

					req, err := http.NewRequestWithContext(ctx, "GET", ratesURL, nil)
					if err != nil {
						fetchErr = fmt.Errorf("err_api_connection") // ZMIANA
					} else {
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							fetchErr = fmt.Errorf("err_api_connection") // ZMIANA
						} else {
							defer resp.Body.Close()
							if resp.StatusCode != http.StatusOK {
								fetchErr = fmt.Errorf("err_api_response") // ZMIANA
							} else {
								if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
									fetchErr = fmt.Errorf("err_api_parsing") // ZMIANA
								}
							}
						}
					}

					currentAppState.Vault.Mu.Lock()
					if currentAppState.UI.SelectedCantor == cantorName {
						if fetchErr != nil {
							currentAppState.Vault.LastEntry = &utilities.CantorEntry{
								Error: fetchErr.Error(),
							}
						} else {
							currentAppState.Vault.LastEntry = &utilities.CantorEntry{
								Rate: rates,
							}
						}
					}
					currentAppState.Vault.Mu.Unlock()

					currentAppState.IsLoading.Store(false)
					w.Invalidate()
				}(window, cantor, state.UI.Currency, state)
			}
		}
	}
}

// handleFrameEvent - a function that handles a frame event
func handleFrameEvent(gtx layout.Context, window *app.Window, state *utilities.AppState,
	theme *material.Theme, ops *op.Ops) {
	renderBackground(gtx, ops)

	state.LastFrameTime = time.Now()

	if state.IsLoading.Load() {
		window.Invalidate()
	}

	// Cantor clicks handling
	handleCantorClicks(gtx, window, state)

	// UI rendering
	utilities.LayoutUI(gtx, theme, state)
}

// run - a function that runs the app
func run(window *app.Window) error {
	// Operations and background image initialization
	var ops op.Ops
	initBackgroundImage()

	log.Println("Fetching cantor list from API:", apiCantors)
	resp, err := http.Get(apiCantors)
	if err != nil {
		return fmt.Errorf("failed to connect to Gix API server (%s): %w", apiCantors, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned non-OK status (%s) for %s", resp.Status, apiCantors)
	}

	// Parsing a JSON from an API response
	var apiCantorsList []ApiCantorResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiCantorsList); err != nil {
		return fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(apiCantorsList) == 0 {
		return fmt.Errorf("API returned 0 cantors")
	}

	definedCantors := make(map[string]*utilities.CantorInfo)
	for _, cfg := range apiCantorsList {
		definedCantors[cfg.Name] = &utilities.CantorInfo{
			ID:          cfg.ID,
			DisplayName: cfg.DisplayName,
		}
		log.Printf("Loaded cantor: %s (ID: %d)", cfg.DisplayName, cfg.ID)
	}

	// State initialization and UI rendering
	state := &utilities.AppState{
		Vault:         &utilities.CantorVault{},
		Cantors:       definedCantors,
		LastFrameTime: time.Now(),
		UI: utilities.UIState{
			Language: "EN",
			Currency: "EUR",
			LanguageOptions: []string{"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ", "HR", "HU", "UA",
				"BU", "RO", "AL", "TR", "IC"},
			CurrencyOptions: []string{"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK", "CZK",
				"HRF", "HUF", "UAH", "BGN", "RON", "LEK", "TRY", "ISK"},
			LanguageOptionButtons: make([]widget.Clickable, len([]string{
				"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ", "HR", "HU", "UA", "BU", "RO", "AL",
				"TR", "IC",
			})),
			CurrencyOptionButtons: make([]widget.Clickable, len([]string{
				"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK", "CZK", "HRF", "HUF", "UAH",
				"BGN", "RON", "LEK", "TRY", "ISK",
			})),
		},
	}

	fontCollection, err := loadFontCollection()
	if err != nil {
		log.Printf("Warning: Failed to load font collection: %v", err)
		fontCollection = []font.FontFace{}
	}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(fontCollection))
	theme.FingerSize = 48

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			handleFrameEvent(gtx, window, state, theme, &ops)

			e.Frame(gtx.Ops)
		}
	}
}
