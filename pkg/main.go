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
	"context"
	_ "image/png"
	"log"
	"os"
	"time"

	"github.com/Niutaq/Gix/fetching_data"
	"github.com/Niutaq/Gix/utilities"

	"gioui.org/f32"
	"gioui.org/op/paint"
	"github.com/Niutaq/Gix/reading_data"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// ++++++++++++++++++++ MAIN Function ++++++++++++++++++++
func main() {
	window := new(app.Window)
	window.Option(
		app.Title("Gix"),
		app.Size(unit.Dp(1000), unit.Dp(1000)),
	)

	go func() {
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

// Functions

// Font handling
// Credits: g45t345rt
func loadFontCollection() ([]font.FontFace, error) {
	PoppinsRegularTTF, err := reading_data.GetFont("fonts/Kanit-Regular.ttf")
	if err != nil {
		log.Fatalf("Error reading font: %v", err)
	}

	PoppinsRegular, err := opentype.Parse(PoppinsRegularTTF)
	if err != nil {
		return nil, err
	}
	// ---
	RubikScribbleRegularTTF, err := reading_data.GetFont("fonts/RubikScribble-Regular.ttf")
	if err != nil {
		log.Fatalf("Error reading font: %v", err)
	}

	RubikScribbleRegular, err := opentype.Parse(RubikScribbleRegularTTF)
	if err != nil {
		return nil, err
	}
	// ---
	NotoSansRegularTTF, err := reading_data.GetFont("fonts/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatalf("Error reading font: %v", err)
	}

	NotoSansRegular, err := opentype.Parse(NotoSansRegularTTF)
	if err != nil {
		return nil, err
	}
	// ---
	fontCollection := []font.FontFace{}
	fontCollection = append(fontCollection, font.FontFace{Font: font.Font{Weight: font.Normal}, Face: PoppinsRegular})
	fontCollection = append(fontCollection, font.FontFace{Font: font.Font{Weight: font.Bold}, Face: RubikScribbleRegular})
	fontCollection = append(fontCollection, font.FontFace{Font: font.Font{Weight: font.SemiBold}, Face: NotoSansRegular})
	return fontCollection, nil
}

// Function to handle window input
func run(window *app.Window) error {
	// OPERATION
	var ops op.Ops
	_ = ops

	// STATE
	/*state := &utilities.AppState{
		Vault:    &utilities.CantorVault{},
		Language: "EN",
		Currency: "EUR",
	}*/
	state := &utilities.AppState{
		Vault:                 &utilities.CantorVault{},
		Language:              "EN",
		Currency:              "EUR",
		LanguageOptions:       []string{"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ", "HR", "HU", "UA", "BU", "RO", "AL", "TR", "IC"},
		CurrencyOptions:       []string{"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF", "SEK", "CZK", "HRF", "HUF", "UAH", "BGN", "RON", "LEK", "TRY", "ISK"},
		LanguageOptionButtons: make([]widget.Clickable, 16),
		CurrencyOptionButtons: make([]widget.Clickable, 17),
	}
	// CURRENCY
	state.Currency = "EUR"

	// UI
	var input widget.Editor
	var addButton widget.Clickable

	_, _ = input, addButton

	// THEMES
	fontCollection, err := loadFontCollection()
	if err != nil {
		log.Fatal(err)
	}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(fontCollection))
	theme.FingerSize = 45

	for {
		switch e := window.Event().(type) {
		// Closing window
		case app.DestroyEvent:
			return e.Err
		// Running window
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			elapsed := time.Since(state.LastFrameTime).Seconds()
			state.LastFrameTime = time.Now()
			state.GradientOffset += float32(elapsed) * 0.75

			maxY := float32(gtx.Constraints.Max.Y)
			stopY := maxY - (maxY / 2) - 80

			gradient := paint.LinearGradientOp{
				Stop1:  f32.Pt(0, 400),
				Stop2:  f32.Pt(0, stopY/1.25),
				Color1: utilities.AppColors.Accent4,
				Color2: utilities.AppColors.Background,
			}

			gradient.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			// Events 'if'
			state.Vault.Mu.Lock()
			if state.IsLoading.Load() {
				window.Invalidate()
			}
			state.Vault.Mu.Unlock()

			// Cantor 1 - button handle
			if state.TadekButton.Clicked(gtx) {
				if state.SelectedCantor == "tadek" {
					state.SelectedCantor = ""
				} else {
					state.SelectedCantor = "tadek"

					state.IsLoading.Store(true)
					state.IsLoadingStart = time.Now()
					go func(w *app.Window) {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()

						rates, err := fetching_data.FetchRatesC1(ctx, "https://kantorstalowawola.tadek.pl", state.Currency, state)

						state.Vault.Mu.Lock()
						defer state.Vault.Mu.Unlock()
						state.IsLoading.Store(false)

						if err != nil {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:   "https://kantorstalowawola.tadek.pl",
								Error: err.Error(),
							}
						} else {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:  "https://kantorstalowawola.tadek.pl",
								Rate: rates,
							}
						}
						w.Invalidate()
					}(window)
				}
			}

			// Cantor 2 - button handle
			if state.KwadratButton.Clicked(gtx) {
				if state.SelectedCantor == "kwadrat" {
					state.SelectedCantor = ""
				} else {
					state.SelectedCantor = "kwadrat"
					state.IsLoading.Store(true)
					state.IsLoadingStart = time.Now()
					go func(w *app.Window) {
						ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
						defer cancel()

						rates, err := fetching_data.FetchRatesC2(ctx, "https://kantory-rzeszow.pl/tabela.htm", state.Currency, state)

						state.Vault.Mu.Lock()
						defer state.Vault.Mu.Unlock()
						state.IsLoading.Store(false)

						if err != nil {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:   "https://kantory-rzeszow.pl/tabela.htm",
								Error: err.Error(),
							}
						} else {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:  "https://kantory-rzeszow.pl/tabela.htm",
								Rate: rates,
							}
						}
						w.Invalidate()
					}(window)
				}
			}

			// Cantor 3 - button handle
			if state.SupersamButton.Clicked(gtx) {
				if state.SelectedCantor == "supersam" {
					state.SelectedCantor = ""
				} else {
					state.SelectedCantor = "supersam"
					state.IsLoading.Store(true)
					state.IsLoadingStart = time.Now()
					go func(w *app.Window) {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()

						rates, err := fetching_data.FetchRatesC3(ctx, "http://www.kantorsupersam.pl/", state.Currency, state)

						state.Vault.Mu.Lock()
						defer state.Vault.Mu.Unlock()
						state.IsLoading.Store(false)

						if err != nil {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:   "http://www.kantorsupersam.pl/",
								Error: err.Error(),
							}
						} else {
							state.Vault.LastEntry = &utilities.CantorEntry{
								URL:  "http://www.kantorsupersam.pl/",
								Rate: rates,
							}
						}
						w.Invalidate()
					}(window)
				}
			}
			utilities.LayoutUI(gtx, theme, &input, &addButton, state)
			e.Frame(gtx.Ops)
		}
	}
}
