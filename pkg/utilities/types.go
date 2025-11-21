package utilities

import (
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/widget"
)

// CantorInfo holds information about a cantor.
type CantorInfo struct {
	ID          int
	DisplayName string
	Button      widget.Clickable
}

// ExchangeRates holds the buy and sell rates for a currency.
type ExchangeRates struct {
	BuyRate  string `json:"buyRate"`
	SellRate string `json:"sellRate"`
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

type Notification struct {
	Message string
	Type    string
	Timeout time.Time
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

	// Notifications
	Notifications *Notification

	UI UIState
}
