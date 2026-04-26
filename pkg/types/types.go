package types

// Global constants for supported currencies and languages
var (
	GlobalCurrencies = []string{
		"EUR", "USD", "GBP", "CHF", "AUD", "DKK", "NOK", "SEK",
		"CZK", "HUF", "UAH", "BGN", "RON", "TRY", "ISK", "LEK",
	}
	GlobalLanguages = []string{
		"EN", "PL", "DE", "DA", "NO", "FR", "SW", "CZ",
		"HR", "HU", "UA", "BU", "RO", "AL", "TR", "IC",
	}
)

// ExchangeRates holds the buy and sell rates for a currency.
type ExchangeRates struct {
	BuyRate   string  `json:"buyRate"`
	SellRate  string  `json:"sellRate"`
	Change24h float64 `json:"change24h"`
}

// FinOpsStatus holds real-time infrastructure metrics.
type FinOpsStatus struct {
	DailySpendUSD    string   `json:"real_spend_24h_usd"`
	BurnRateStatus   string   `json:"burn_rate_status"`
	BlockedProviders []string `json:"blocked_providers"`
	SystemTime       string   `json:"system_time"`
	IsActive         bool     `json:"is_governance_active"`
}

// ApiCantorResponse for parsing JSON
type ApiCantorResponse struct {
	ID          int     `json:"id"`
	DisplayName string  `json:"displayName"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Strategy    string  `json:"strategy"`
}

// CityRecord represents a city in Elasticsearch
type CityRecord struct {
	Name     string   `json:"name"`
	Province string   `json:"province"`
	Location GeoPoint `json:"location"`
}

// GeoPoint for Elasticsearch
type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// ScrapeCompletedEvent mapping for internal use if needed
type ScrapeCompletedEvent struct {
	ProviderID  string
	ScraperType string
	DurationMs  int64
	Timestamp   int64
}
