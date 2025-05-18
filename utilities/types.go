package utilities

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/widget"
)

// Types
// Information for fetching data from cantors
type FetcherFunc func(ctx context.Context, url, currency string, state *AppState) (ExchangeRates, error)

type CantorInfo struct {
	ID                  string
	URL                 string
	Displayname         string
	Fetcher             FetcherFunc
	Button              widget.Clickable
	DefaultTimeout      time.Duration
	NeedsRateFormatting bool
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
	Cantors        []*CantorInfo
	SelectedCantor string

	// Modal widgets
	ModalOpen             string
	LangModalButton       widget.Clickable
	CurrencyModalButton   widget.Clickable
	ModalClick            widget.Clickable
	LanguageOptions       []string
	CurrencyOptions       []string
	LanguageOptionButtons []widget.Clickable
	CurrencyOptionButtons []widget.Clickable

	// Language widgets
	Language string

	// Exchange currency widgets
	Currency       string
	// TadekButton    widget.Clickable
	// KwadratButton  widget.Clickable
	// SupersamButton widget.Clickable

	// Erros, indicators, etc.
	LastInvalidation time.Time
	IsLoading        atomic.Bool
	IsLoadingStart   time.Time
	LastFrameTime    time.Time

	// Additional widgets
	GradientOffset float32
}
