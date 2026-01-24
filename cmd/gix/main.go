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
	// Standard libraries
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/trace"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	// External utilities
	"github.com/Niutaq/Gix/pkg/utilities"
)

// the main is the entry point of the application that initializes configuration, sets up the main window, and starts the app loop.
func main() {
	apiBase := flag.String("api", "http://165.227.246.100:8080", "API base URL")
	flag.Parse()

	base := *apiBase
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}

	config := utilities.AppConfig{
		APICantorsURL: base + "/api/v1/cantors",
		APIRatesURL:   base + "/api/v1/rates",
		APIHistoryURL: base + "/api/v1/history",
	}
	window := new(app.Window)
	window.Option(
		app.Title("Gix"),
		app.Size(unit.Dp(1280), unit.Dp(1280)),
	)

	go func() {
		if err := run(window, config); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	app.Main()
}

// run starts the application event loop, handling window events, UI rendering, and asynchronous data loading.
func run(window *app.Window, config utilities.AppConfig) error {
	cleanupTrace := setupTrace()
	defer cleanupTrace()

	state, theme, cantorChan := setupApplication(window, config)

	// Start dRPC client
	go utilities.StartDRPCStream(window, state, config.APICantorsURL)

	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			handleFrame(window, &ops, e, state, config, cantorChan, theme)
		}
	}
}

// setupTrace sets up tracing for the application, returning a function to stop it.
func setupTrace() func() {
	fileTrace, err := os.Create("trace.out")
	if err != nil {
		log.Fatal(err)
	}
	if err := trace.Start(fileTrace); err != nil {
		log.Fatal(err)
	}
	return func() {
		trace.Stop()
		if err := fileTrace.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

// setupApplication sets up the application state and theme, loading cantors asynchronously, and returns the theme and application state.
func setupApplication(window *app.Window, config utilities.AppConfig) (*utilities.AppState, *material.Theme, chan []utilities.ApiCantorResponse) {
	utilities.InitTranslations()

	state := initializeAppState()
	utilities.LoadCache(state)
	cantorChan := make(chan []utilities.ApiCantorResponse, 1)
	loadCantorsAsync(window, cantorChan, config)

	fonts, err := utilities.LoadFontCollection()
	if err != nil {
		log.Printf("Font load failed: %v", err)
	} else {
		log.Println("Font collection loaded successfully.")
	}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(fonts))
	theme.FingerSize = 48

	log.Println("Application started.")
	return state, theme, cantorChan
}

// initializeAppState initializes the application state with default values.
func initializeAppState() *utilities.AppState {
	return &utilities.AppState{
		Vault:   &utilities.CantorVault{},
		Cantors: make(map[string]*utilities.CantorInfo),
		UI: utilities.UIState{
			Language: "EN",
			Currency: "EUR",
			LanguageOptions: []string{
				"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ",
				"HR", "HU", "UA", "BU", "RO", "AL", "TR", "IC",
			},
			CurrencyOptions: []string{
				"EUR", "USD", "GBP", "AUD", "DKK", "NOK", "CHF",
				"SEK", "CZK", "HRF", "HUF", "UAH", "BGN",
				"RON", "LEK", "TRY", "ISK",
			},
			LanguageOptionButtons: make([]widget.Clickable, 16),
			CurrencyOptionButtons: make([]widget.Clickable, 17),
			ChartMode:             "BUY",
			ChartModeButtons:      make([]widget.Clickable, 2),
		},
	}
}

// handleFrame processes the current frame event by updating state, adjusting scaling, and rendering the UI.
func handleFrame(window *app.Window, ops *op.Ops, e app.FrameEvent, state *utilities.AppState, config utilities.AppConfig, cantorChan chan []utilities.ApiCantorResponse, theme *material.Theme) {
	updateCantors(window, state, config, cantorChan)

	// Adjust scaling for Windows and Linux if UI is too small
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		e.Metric.PxPerDp *= 1.5
		e.Metric.PxPerSp *= 1.5
	}

	ops.Reset()
	gtx := app.NewContext(ops, e)

	utilities.LayoutUI(gtx, window, theme, state, config)

	e.Frame(gtx.Ops)
}

// updateCantors updates the list of cantors from the provided channel, if any.
func updateCantors(window *app.Window, state *utilities.AppState, config utilities.AppConfig, cantorChan chan []utilities.ApiCantorResponse) {
	select {
	case list := <-cantorChan:
		state.CantorsMu.Lock()
		for _, c := range list {
			state.Cantors[c.Name] = &utilities.CantorInfo{
				ID:          c.ID,
				DisplayName: c.DisplayName,
				Latitude:    c.Latitude,
				Longitude:   c.Longitude,
			}
		}
		state.CantorsMu.Unlock()
		utilities.SaveCache(state)
		utilities.FetchAllRates(window, state, config)

	default:
	}
}

// loadCantorsAsync fetches cantor data from the API asynchronously, decodes it, and sends it to the provided channel.
func loadCantorsAsync(window *app.Window, out chan<- []utilities.ApiCantorResponse, config utilities.AppConfig) {
	go func() {
		client := http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(config.APICantorsURL)
		if err != nil {
			log.Println("Error fetching cantors:", err)
			return
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				log.Println("Error closing response body:", err)
			}
		}(resp.Body)

		var list []utilities.ApiCantorResponse
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			log.Println("Error parsing cantors:", err)
			return
		}

		out <- list
		window.Invalidate()
	}()
}
