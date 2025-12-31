package utilities

import (
	// Standard libraries
	"sync"
	"sync/atomic"
	"time"

	// Gio utilities
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
	URL      string
	Rate     ExchangeRates
	Error    string
	LoadedAt time.Time
}

// CantorVault stores the entries from all cantors
type CantorVault struct {
	Mu        sync.Mutex
	Rates     map[string]*CantorEntry
	LastEntry *CantorEntry
}

// UIState holds UI-specific state and widgets.
type UIState struct {
	ModalOpen           string
	LangModalButton     widget.Clickable
	CurrencyModalButton widget.Clickable
	ModalClick          widget.Clickable
	ModalList           widget.List
	ModalClose          widget.Clickable

	CurrencyList widget.List
	SearchEditor widget.Editor
	SearchText   string

	SelectedCantor        string
	SelectedLanguage      string
	LanguageOptions       []string
	CurrencyOptions       []string
	LanguageOptionButtons []widget.Clickable
	CurrencyOptionButtons []widget.Clickable

	Language string
	Currency string

	CantorsList widget.List
}

// Notification holds information about a notification banner.
type Notification struct {
	Message string
	Type    string
	Timeout time.Time
}

// AppState holds the overall state of the application.
type AppState struct {
	Vault          *CantorVault
	Cantors        map[string]*CantorInfo
	LastFrameTime  time.Time
	IsLoadingStart time.Time
	IsLoading      atomic.Bool
	Notifications  *Notification
	UI             UIState
}

// AppConfig stores app configuration
type AppConfig struct {
	APICantorsURL string
	APIRatesURL   string
}

// ApiCantorResponse for parsing JSON
type ApiCantorResponse struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
}
