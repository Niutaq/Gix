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
	"fmt"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/Niutaq/Gix/readers"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/PuerkitoBio/goquery"
)

// Structs
type ExchangeRates struct {
	BuyRate  string
	SellRate string
}

type CantorEntry struct {
	URL   string
	Rate  ExchangeRates
	Error string
}

type CantorVault struct {
	mu        sync.Mutex
	LastEntry *CantorEntry
}

// Language options
var translations = map[string]map[string]string{
	"EN": {
		"title":         "Gix",
		"languageLabel": "Language",
		"subtitle":      "Exchange rates from local currency exchange offices",
		"buyLabel":      "Buy",
		"sellLabel":     "Sell",
	},
	"PL": {
		"title":         "Gix",
		"languageLabel": "Język",
		"subtitle":      "Kursy wymiany walut od lokalnych kantorów",
		"buyLabel":      "Kupno",
		"sellLabel":     "Sprzedaż",
		"errorPrefix":   "Błąd",
	},
	"DE": {
		"title":         "Gix",
		"languageLabel": "Sprache",
		"subtitle":      "Die Wechselkurse der örtlichen Wechselstuben an",
		"buyLabel":      "Kaufen",
		"sellLabel":     "Verkaufen",
		"errorPrefix":   "Fehler",
	},
}

type AppState struct {
	// Main Vault
	Vault *CantorVault
	// Language widgets
	Language string
	enButton widget.Clickable
	plButton widget.Clickable
	deButton widget.Clickable

	// Currency widgets
	Currency  string
	eurButton widget.Clickable
	usdButton widget.Clickable
	gbpButton widget.Clickable

	// Exchange currency widgets
	tadekButton widget.Clickable

	// Erros, indicators, etc.
	errorMessage     string
	errorOpacity     float32
	errorDisplayTime time.Time
	lastInvalidation time.Time
	isLoading        bool
	lastFrameTime    time.Time
}

// Colors
var AppColors = struct {
	Background color.NRGBA
	Text       color.NRGBA
	Error      color.NRGBA
	Success    color.NRGBA
	Title      color.NRGBA
	Button     color.NRGBA
	Info       color.NRGBA
	Warning    color.NRGBA
	Primary    color.NRGBA
	Secondary  color.NRGBA
	Light      color.NRGBA
	Dark       color.NRGBA
	Accent1    color.NRGBA
	Accent2    color.NRGBA
	Accent3    color.NRGBA
}{
	Background: color.NRGBA{R: 18, G: 18, B: 18, A: 255},    // Dark background
	Text:       color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // White text
	Error:      color.NRGBA{R: 255, G: 0, B: 0, A: 255},     // Red error
	Success:    color.NRGBA{R: 0, G: 255, B: 0, A: 255},     // Green success
	Title:      color.NRGBA{R: 255, G: 255, B: 0, A: 255},   // Yellow title
	Button:     color.NRGBA{R: 80, G: 80, B: 80, A: 255},    // White button
	Info:       color.NRGBA{R: 0, G: 191, B: 255, A: 255},   // DeepSkyBlue info
	Warning:    color.NRGBA{R: 255, G: 165, B: 0, A: 255},   // Orange warning
	Primary:    color.NRGBA{R: 0, G: 123, B: 255, A: 255},   // Blue primary
	Secondary:  color.NRGBA{R: 108, G: 117, B: 125, A: 255}, // Gray secondary
	Light:      color.NRGBA{R: 248, G: 249, B: 250, A: 255}, // LightGray light
	Dark:       color.NRGBA{R: 15, G: 15, B: 15, A: 255},    // Dark accent
	Accent1:    color.NRGBA{R: 255, G: 255, B: 0, A: 255},   // Yellow accent
	Accent2:    color.NRGBA{R: 255, G: 235, B: 00, A: 255},  // Yellow/Orange accent
	Accent3:    color.NRGBA{R: 0, G: 50, B: 255, A: 255},    // Blue accent
}

// ++++++++++++++++++++ MAIN Function ++++++++++++++++++++
func main() {
	//cmd.Execute()
	go func() {
		window := new(app.Window)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

// Functions
// Adding link to links container + sorting
func layoutVaultLinks(gtx layout.Context, theme *material.Theme, state *AppState) layout.Dimensions {
	state.Vault.mu.Lock()
	defer state.Vault.mu.Unlock()

	if state.Vault.LastEntry == nil {
		return layout.Dimensions{}
	}

	entry := *state.Vault.LastEntry
	lang := state.Language
	t := translations[lang]

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			urlText := material.Body1(theme, "Link: "+entry.URL)
			urlText.Color = AppColors.Accent1
			urlText.TextSize = unit.Sp(18)
			urlText.Alignment = text.Middle
			return layout.Center.Layout(gtx, urlText.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if entry.Error != "" {
				errorText := material.Body2(theme, t["errorPrefix"]+": "+entry.Error)
				errorText.Color = AppColors.Error
				errorText.TextSize = unit.Sp(16)
				errorText.Alignment = text.Middle
				return layout.Center.Layout(gtx, errorText.Layout)
			} else {
				formattedBuy, _ := formatRate(entry.Rate.BuyRate)
				formattedSell, _ := formatRate(entry.Rate.SellRate)
				rateTextBuy := material.Body2(theme, fmt.Sprintf("%s: %s PLN",
					t["buyLabel"], formattedBuy))
				rateTextBuy.TextSize = unit.Sp(28)
				rateTextBuy.Color = AppColors.Success
				rateTextBuy.Alignment = text.Middle
				rateTextSell := material.Body2(theme, fmt.Sprintf("%s: %s PLN",
					t["sellLabel"], formattedSell))
				rateTextSell.TextSize = unit.Sp(28)
				rateTextSell.Color = AppColors.Error
				rateTextSell.Alignment = text.Middle
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(rateTextBuy.Layout),
						layout.Rigid(rateTextSell.Layout),
					)
				})
			}
		}),
	)
}

// GUI Elements creation - function
func layoutUI(gtx layout.Context, theme *material.Theme, input *widget.Editor, addButton *widget.Clickable, state *AppState) {
	lang := state.Language
	t := translations[lang]

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 15, Bottom: 15, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Baseline, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Currency buttons
						btn := material.Button(theme, &state.eurButton, "EUR")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "EUR" {
							btn.Background = AppColors.Accent1
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.usdButton, "USD")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "USD" {
							btn.Background = AppColors.Accent1
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.gbpButton, "GBP")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Currency == "GBP" {
							btn.Background = AppColors.Accent1
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					// Language buttons
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.enButton, "EN")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Language == "EN" {
							btn.Background = AppColors.Accent2
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.plButton, "PL")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Language == "PL" {
							btn.Background = AppColors.Accent2
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(theme, &state.deButton, "DE")
						btn.Background = AppColors.Dark
						btn.TextSize = unit.Sp(16)
						if state.Language == "DE" {
							btn.Background = AppColors.Accent2
							btn.Color = AppColors.Dark
						}
						return btn.Layout(gtx)
					}),
				)
			})
		}),

		// Title
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 100, Bottom: 15, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				title := material.H1(theme, t["title"])
				title.Alignment = text.Middle
				title.TextSize = unit.Sp(90)
				title.Font.Weight = font.Bold
				title.Color = AppColors.Title
				return title.Layout(gtx)
			})
		}),
		// Subtitle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			subtitle := material.Body1(theme, t["subtitle"])
			subtitle.Alignment = text.Middle
			subtitle.TextSize = unit.Sp(18)
			subtitle.Color = AppColors.Text
			subtitle.Font.Weight = font.Normal
			return subtitle.Layout(gtx)
		}),
		// Subtitle
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			subtitle := material.Body1(theme, "\n")
			subtitle.Alignment = text.Middle
			subtitle.TextSize = unit.Sp(22)
			subtitle.Color = AppColors.Text
			subtitle.Font.Weight = font.Normal
			return subtitle.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if state.isLoading {
				return drawProgressBar(gtx, theme)
			} else {
				button := material.Button(theme, &state.tadekButton, "Kantor Tadek")
				button.Background = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				button.Color = AppColors.Dark
				button.TextSize = unit.Sp(16)
				return button.Layout(gtx)
			}
		}),
	}

	if state.errorMessage != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				errorColor := AppColors.Error
				errorColor.A = uint8(255 * state.errorOpacity)

				errorText := material.Body2(theme, state.errorMessage)
				errorText.Color = errorColor
				return errorText.Layout(gtx)
			})
		}))
	}

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layoutVaultLinks(gtx, theme, state)
	}))

	layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)
}

func fetchRatesC1(ctx context.Context, url, currency string, state *AppState) (ExchangeRates, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ExchangeRates{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ExchangeRates{}, fmt.Errorf("Error with fetching: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return ExchangeRates{}, fmt.Errorf("Error with parsing: %w", err)
	}

	var buyRate, sellRate string

	doc.Find("table.kursy_walut tbody tr").Each(func(i int, s *goquery.Selection) {
		symbol := strings.TrimSpace(s.Find("td").Eq(1).Text())

		if symbol == currency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(3).Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(4).Text())
		}
	})

	if buyRate == "" || sellRate == "" {
		return ExchangeRates{}, fmt.Errorf("%s not found, try diffrent symbol.", currency)
	}

	return ExchangeRates{
		BuyRate:  buyRate,
		SellRate: sellRate,
	}, nil
}

// Font handling
// Credits: g45t345rt
func loadFontCollection() ([]font.FontFace, error) {
	PoppinsRegularTTF, err := readers.GetFont("fonts/Kanit-Regular.ttf")
	if err != nil {
		log.Fatalf("Error reading font: %v", err)
	}

	PoppinsRegular, err := opentype.Parse(PoppinsRegularTTF)
	if err != nil {
		return nil, err
	}

	RubikScribbleRegularTTF, err := readers.GetFont("fonts/RubikScribble-Regular.ttf")
	if err != nil {
		log.Fatalf("Error reading font: %v", err)
	}

	RubikScribbleRegular, err := opentype.Parse(RubikScribbleRegularTTF)
	if err != nil {
		return nil, err
	}

	fontCollection := []font.FontFace{}
	fontCollection = append(fontCollection, font.FontFace{Font: font.Font{Weight: font.Normal}, Face: PoppinsRegular})
	fontCollection = append(fontCollection, font.FontFace{Font: font.Font{Weight: font.Bold}, Face: RubikScribbleRegular})
	return fontCollection, nil

}

// Function fot exchange rates formatting
func formatRate(rate string) (string, error) {
	floatRate, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return "", fmt.Errorf("Upss: ", err)
	}

	formattedRate := floatRate / 100

	return fmt.Sprintf("%.2f", formattedRate), nil
}

// Function for loading progress bar
func drawProgressBar(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	baseTime := gtx.Now.Truncate(time.Second)
	elapsed := gtx.Now.Sub(baseTime).Seconds()
	progress := float32(elapsed)

	barHeight := gtx.Dp(unit.Dp(20))
	margin := gtx.Dp(unit.Dp(5))
	maxWidth := gtx.Constraints.Max.X - margin*2

	// Calculate RGB values for black to yellow gradient
	r := uint8(255 * progress)
	g := uint8(255 * progress)
	b := uint8(0)

	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 40, G: 40, B: 40, A: 255},
		clip.Rect{
			Min: image.Point{X: margin, Y: 0},
			Max: image.Point{X: maxWidth + margin, Y: barHeight},
		}.Op(),
	)

	animatedWidth := int(float32(maxWidth) * progress)
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: r, G: g, B: b, A: 255},
		clip.Rect{
			Min: image.Point{X: margin, Y: 0},
			Max: image.Point{X: margin + animatedWidth, Y: barHeight},
		}.Op(),
	)

	return layout.Dimensions{
		Size:     image.Point{X: gtx.Constraints.Max.X, Y: barHeight},
		Baseline: 0,
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Function to handle window input
func run(window *app.Window) error {
	// OPERATION
	var ops op.Ops

	// STATE
	state := &AppState{
		Vault:    &CantorVault{},
		Language: "EN",
		Currency: "EUR",
	}
	//state.langSelect.Value = "EN"

	// CURRENCY
	state.Currency = "EUR"

	// UI
	var input widget.Editor
	var addButton widget.Clickable

	// THEMES
	fontCollection, err := loadFontCollection()
	if err != nil {
		log.Fatal(err)
	}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(fontCollection))

	for {
		switch e := window.Event().(type) {
		// Closing window
		case app.DestroyEvent:
			return e.Err
		// Running window
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			now := time.Now()
			if state.errorMessage != "" {
				elapsed := now.Sub(state.errorDisplayTime).Seconds()

				if elapsed >= 0.75 {
					state.errorMessage = ""
					state.errorOpacity = 0
				} else {
					state.errorOpacity = 1 - float32(elapsed/2)
				}

				if now.Sub(state.lastInvalidation) > 16*time.Millisecond { // ~60 FPS
					window.Invalidate()
					state.lastInvalidation = now
				}
			}

			/*backgroundGradient(gtx)*/
			paint.Fill(gtx.Ops, AppColors.Background)

			// Events 'if'
			state.Vault.mu.Lock()
			if state.isLoading {
				window.Invalidate()
			}
			state.Vault.mu.Unlock()

			// Language changer
			if state.enButton.Clicked(gtx) {
				state.Language = "EN"
				window.Invalidate()
			}
			if state.plButton.Clicked(gtx) {
				state.Language = "PL"
				window.Invalidate()
			}
			if state.deButton.Clicked(gtx) {
				state.Language = "DE"
				window.Invalidate()
			}

			// Currency changing
			if state.eurButton.Clicked(gtx) {
				state.Currency = "EUR"
				window.Invalidate()
			}
			if state.usdButton.Clicked(gtx) {
				state.Currency = "USD"
				window.Invalidate()
			}
			if state.gbpButton.Clicked(gtx) {
				state.Currency = "GBP"
				window.Invalidate()
			}

			// Button event
			if state.tadekButton.Clicked(gtx) {
				state.Vault.mu.Lock()
				state.isLoading = true
				state.Vault.mu.Unlock()
				window.Invalidate()

				go func(w *app.Window) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					rates, err := fetchRatesC1(ctx, "https://kantorstalowawola.tadek.pl", state.Currency, state)

					state.Vault.mu.Lock()
					defer state.Vault.mu.Unlock()
					state.isLoading = false

					if err != nil {
						state.errorMessage = err.Error()
						state.errorDisplayTime = time.Now()
						state.errorOpacity = 1.0
						w.Invalidate()
					}

					if err != nil {
						state.Vault.LastEntry = &CantorEntry{
							URL:   "https://kantorstalowawola.tadek.pl",
							Error: err.Error(),
						}
					} else {
						state.Vault.LastEntry = &CantorEntry{
							URL:  "https://kantorstalowawola.tadek.pl",
							Rate: rates,
						}
					}
					w.Invalidate()
				}(window)
			}

			layoutUI(gtx, theme, &input, &addButton, state)
			e.Frame(gtx.Ops)
		}
	}
	return nil
}
