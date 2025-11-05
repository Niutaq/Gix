package utilities

import (
	"context"
	"sync"
	"sync/atomic"
	_ "sync/atomic"
	"time"

	"gioui.org/widget"
)

// Types
// Information for fetching data from cantors
type FetcherFunc func(ctx context.Context, url, currency string, state *AppState) (ExchangeRates, error)

type CantorInfo struct {
	ID                  string
	URL                 string
	DisplayName         string
	Fetcher             FetcherFunc
	DefaultTimeout      time.Duration
	NeedsRateFormatting bool
	Button              widget.Clickable
}

// ExchangeRates holds the buy and sell rates for a currency.
type ExchangeRates struct {
	BuyRate  string
	SellRate string
}

// CantorEntry represents data fetched from a single cantor.
type CantorEntry struct {
	URL   string
	Rate  ExchangeRates
	Error string
}

// CantorVault stores the latest entry from cantors, protected by a mutex.
type CantorVault struct {
	Mu        sync.Mutex
	LastEntry *CantorEntry
}

// AppState holds the overall state of the application.
type AppState struct {
	// Main Vault
	Vault *CantorVault

	// Cantor(s) information
	Cantors        map[string]*CantorInfo
	LastFrameTime  time.Time
	IsLoadingStart time.Time
	IsLoading      atomic.Bool

	// Additional widgets
	// GradientOffset float32

	UI UIState
}

// UIState holds UI-specific state and widgets.
type UIState struct {
	ModalOpen             string
	LangModalButton       widget.Clickable
	CurrencyModalButton   widget.Clickable
	ModalClick            widget.Clickable
	ModalList             widget.List
	SelectedCantor        string
	SelectedLanguage      string
	LanguageOptions       []string
	CurrencyOptions       []string
	LanguageOptionButtons []widget.Clickable
	CurrencyOptionButtons []widget.Clickable
	GradientOffset        float32
	Language              string
	Currency              string
	IsLoading             time.Time
}
