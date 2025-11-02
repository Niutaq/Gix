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
	"fmt"
	"image"
	_ "image/png"
	"log"
	"os"
	"time"

	// Go files
	"github.com/Niutaq/Gix/pkg/config"
	"github.com/Niutaq/Gix/pkg/fetching_data"
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
//go:embed res/background.png
var backgroundImageFS embed.FS

//go:embed config.json
var configFS embed.FS

var (
	originalBackgroundImage  image.Image
	paintableBackgroundImage paint.ImageOp
	backgroundImageLoaded    bool
)

// Functions
// ++++++++++++++++++++ MAIN Function ++++++++++++++++++++
func main() {
	window := new(app.Window)
	window.Option(
		app.Title("Gix"),
		app.Size(unit.Dp(1000), unit.Dp(1000)),
		//app.Decorated(false),
	)

	go func() {
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

// Background image initialization
func initBackgroundImage() {
	if backgroundImageLoaded {
		return
	}
	fileData, err := backgroundImageFS.ReadFile("res/background.png")
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

// Font handling
// Credits: g45t345rt
func loadFontCollection() ([]font.FontFace, error) {
	var fontCollection []font.FontFace

	fontPaths := []string{
		"fonts/Kanit-Regular.ttf",
		"fonts/RubikScribble-Regular.ttf",
		"fonts/NotoSans-Regular.ttf",
	}

	for _, path := range fontPaths {
		face, err := reading_data.LoadAndParseFont(path)
		if err != nil {
			return nil, err // Return error to caller
		}
		fontCollection = append(fontCollection, face)
	}

	return fontCollection, nil
}

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

// Function to handle window input
func run(window *app.Window) error {
	// Operations and background image initialization
	var ops op.Ops
	initBackgroundImage()

	// Load cantor configurations from config.json
	configData, err := configFS.ReadFile("config.json")
	if err != nil {
		return fmt.Errorf("failed to read config.json: %w", err)
	}

	cantorConfigs, err := config.LoadCantorConfigFromBytes(configData)
	if err != nil {
		return fmt.Errorf("failed to load cantor configurations: %w", err)
	}

	definedCantors := make([]*utilities.CantorInfo, len(cantorConfigs))
	for i, cfg := range cantorConfigs {
		var fetcher utilities.FetcherFunc
		switch cfg.ID {
		case "tadek":
			fetcher = fetching_data.FetchRatesC1
		case "kwadrat":
			fetcher = fetching_data.FetchRatesC2
		case "supersam":
			fetcher = fetching_data.FetchRatesC3
		default:
			return fmt.Errorf("unknown cantor ID in config: %s", cfg.ID)
		}

		definedCantors[i] = &utilities.CantorInfo{
			ID:                  cfg.ID,
			Displayname:         cfg.Displayname,
			URL:                 cfg.URL,
			Fetcher:             fetcher,
			DefaultTimeout:      cfg.DefaultTimeout,
			NeedsRateFormatting: cfg.NeedsRateFormatting,
		}
	}

	state := &utilities.AppState{
		Vault:           &utilities.CantorVault{},
		Language:        "EN",
		Currency:        "EUR",
		LanguageOptions: []string{"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ", "HR", "HU", "UA", "BU", "RO", "AL", "TR", "IC"},
		CurrencyOptions: []string{"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK", "CZK", "HRF", "HUF", "UAH", "BGN", "RON", "LEK", "TRY", "ISK"},
		LanguageOptionButtons: make([]widget.Clickable, len([]string{
			"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ", "HR", "HU", "UA", "BU", "RO", "AL", "TR", "IC",
		})),
		CurrencyOptionButtons: make([]widget.Clickable, len([]string{
			"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK", "CZK", "HRF", "HUF", "UAH", "BGN", "RON", "LEK", "TRY", "ISK",
		})),
		Cantors:       definedCantors,
		LastFrameTime: time.Now(),
	}

	fontCollection, err := loadFontCollection()
	if err != nil {
		log.Printf("Warning: Failed to load font collection: %v", err)
		// Optionally, you can return the error or use a default font collection
		// For now, we'll proceed with an empty font collection if loading fails
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
		
					renderBackground(gtx, &ops)
			// // Set the background color
			// screenHeight := float32(gtx.Constraints.Max.Y)

			// colorAtMiddle := color.NRGBA{R: 5, G: 5, B: 0, A: 255}
			// colorAtBottom := utilities.AppColors.Accent1

			// paint.LinearGradientOp{
			// 	Stop1:  f32.Pt(0, screenHeight*0.99),
			// 	Stop2:  f32.Pt(0, screenHeight),
			// 	Color1: colorAtMiddle,
			// 	Color2: colorAtBottom,
			// }.Add(gtx.Ops)
			// paint.PaintOp{}.Add(gtx.Ops)

			state.LastFrameTime = time.Now()

			if state.IsLoading.Load() {
				window.Invalidate()
			}

			for i := range state.Cantors {
				cantor := state.Cantors[i]

				if cantor.Button.Clicked(gtx) {
					if state.SelectedCantor == cantor.ID {
						state.SelectedCantor = ""
						state.Vault.LastEntry = nil
					} else {
						state.SelectedCantor = cantor.ID
						state.IsLoading.Store(true)
						state.IsLoadingStart = time.Now()
						state.Vault.LastEntry = nil

						go func(w *app.Window, cInfo *utilities.CantorInfo, currentCurrency string, currentAppState *utilities.AppState) {
							ctx, cancel := context.WithTimeout(context.Background(), cInfo.DefaultTimeout)
							defer cancel()

							rates, fetchErr := cInfo.Fetcher(ctx, cInfo.URL, currentCurrency, currentAppState)

							currentAppState.Vault.Mu.Lock()
							if currentAppState.SelectedCantor == cInfo.ID {
								if fetchErr != nil {
									currentAppState.Vault.LastEntry = &utilities.CantorEntry{
										URL:   cInfo.URL,
										Error: fetchErr.Error(),
									}
								} else {
									currentAppState.Vault.LastEntry = &utilities.CantorEntry{
										URL:  cInfo.URL,
										Rate: rates,
									}
								}
							}
							currentAppState.Vault.Mu.Unlock()

							currentAppState.IsLoading.Store(false)
							w.Invalidate()
						}(window, cantor, state.Currency, state)
					}
				}
			}

			// UI rendering
			utilities.LayoutUI(gtx, theme, state)
			e.Frame(gtx.Ops)
		}
	}
}
