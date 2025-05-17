package utilities

import (
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/widget"
)

// Types
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
	TadekButton    widget.Clickable
	KwadratButton  widget.Clickable
	SupersamButton widget.Clickable
	SelectedCantor string

	// Erros, indicators, etc.
	LastInvalidation time.Time
	IsLoading        atomic.Bool
	IsLoadingStart   time.Time
	LastFrameTime    time.Time

	// Additional widgets
	GradientOffset float32
}
