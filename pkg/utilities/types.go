package utilities

import (
	// Standard libraries
	"sync"
	"sync/atomic"
	"time"

	// Gio utilities
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/widget"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/pkg/search"
	"github.com/Niutaq/Gix/pkg/types"
)

// CantorInfo holds information about a cantor.
type CantorInfo struct {
	ID          int
	DisplayName string
	Address     string
	Latitude    float64
	Longitude   float64
	Strategy    string
	Button      widget.Clickable `json:"-"`
	DeleteBtn   widget.Clickable `json:"-"`

	// Long Press Logic
	PressStart         time.Time `json:"-"`
	IsPressing         bool      `json:"-"`
	LongPressTriggered bool      `json:"-"`
}

// HoverInfo holds data for the dynamic notch display
type HoverInfo struct {
	Active   bool
	Title    string
	Subtitle string
	Extra    string
}

// IntroAnim holds state for the intro animation.
type IntroAnim struct {
	Active    bool
	StartTime time.Time
	Progress  float32
}

// MenuAnim holds state for the mobile menu animation.
type MenuAnim struct {
	CurrentAlpha float32
	LastTime     time.Time
}

// ExchangeRates holds the buy and sell rates for a currency.
type ExchangeRates struct {
	BuyRate   string  `json:"buyRate"`
	SellRate  string  `json:"sellRate"`
	Change24h float64 `json:"change24h"`
}

// CantorEntry represents data fetched from a single cantor.
type CantorEntry struct {
	URL              string
	Rate             ExchangeRates
	Error            string
	LoadedAt         time.Time
	AppearanceSpring Spring

	// Display cache (optimizes GC by avoiding fmt.Sprintf in every frame)
	DisplayBuy    string
	DisplaySell   string
	DisplaySpread string
	DisplayChange string

	// Real-time flash effect
	LastUpdate  time.Time
	UpdatePulse float32 // 0 to 1, fades out
}

// CantorVault stores the entries from all cantors grouped by currency.
type CantorVault struct {
	Mu        sync.Mutex
	Rates     map[string]map[string]*CantorEntry // Currency -> CantorName -> Entry
	LastEntry *CantorEntry
}

// Spring holds state for physics-based animations.
type Spring struct {
	Target   float32
	Current  float32
	Velocity float32
	Tension  float32 // e.g., 150
	Friction float32 // e.g., 15
}

// ViewMode represents the responsive layout state
type ViewMode int

const (
	ViewDesktop ViewMode = iota
	ViewMid
	ViewMobile
)

// UIState holds UI-specific state and widgets.
type UIState struct {
	IsFinOpsDashboardOpen bool
	FinOpsBtn             widget.Clickable
	ModalOpen             string
	ModalAnimStart        time.Time
	LangModalButton       widget.Clickable
	CurrencyModalButton   widget.Clickable
	ModalClick            widget.Clickable
	ModalList             widget.List
	ModalClose            widget.Clickable

	MobileMenuOpen       bool
	MobileMenuBtn        widget.Clickable
	MobileMenuBackdrop   widget.Clickable
	BgClick              widget.Clickable
	IsMobile             bool // Kept for legacy logic, but updated automatically
	CurrentViewMode      ViewMode
	MapVisibleMobile     bool
	MapBtnMobile         widget.Clickable
	LayoutTransitionTime time.Time

	CurrencyList    widget.List
	SearchEditor    widget.Editor
	SearchText      string
	SearchActive    bool
	SearchClickable widget.Clickable
	FilteredIDs     []string
	CityResults     []types.CityRecord
	CityClickables  []widget.Clickable

	ChartMode        string             // "BUY" or "SELL"
	ChartModeButtons []widget.Clickable // [0] -> Buy, [1] -> Sell
	ChartHoverTag    int                // Unique tag for chart input events
	ChartHoverX      float32
	ChartHoverActive bool

	UserLocation struct {
		Latitude  float64
		Longitude float64
		Active    bool
	}
	MapFocus struct {
		Latitude  float64
		Longitude float64
		CityName  string
	}

	MapState struct {
		CenterLat  float64
		CenterLon  float64
		Zoom       float32
		DragStart  f32.Point
		Dragging   bool
		ZoomInBtn  widget.Clickable
		ZoomOutBtn widget.Clickable
	}

	PinnedIDs       []string
	GeocodingActive bool
	ScannedChunks   map[string]bool
	LocalHistory    map[string][]float64
	MaxDistance     float64
	DistanceSlider  widget.Float
	LocateButton    widget.Clickable
	// Long Press for Locate
	LocatePressStart         time.Time
	LocateIsPressing         bool
	LocateLongPressTriggered bool

	StatusClickable widget.Clickable
	HoverInfo       HoverInfo
	NotchState      struct {
		CurrentAlpha   float32
		LastContent    HoverInfo
		LastTime       time.Time
		LastHoverTime  time.Time
		HoverStartTime time.Time
	}

	PulseState struct {
		ShowGainer bool      // True = Gainer, False = Loser
		LastSwitch time.Time // Last switch?
	}

	TopMovers struct {
		GainerID   string
		LoserID    string
		LastUpdate time.Time
	}

	IntroAnim IntroAnim
	MenuAnim  MenuAnim

	SelectedCantor        string
	SelectedLanguage      string
	LanguageOptions       []string
	CurrencyOptions       []string
	LanguageOptionButtons []widget.Clickable
	CurrencyOptionButtons []widget.Clickable

	Language string
	Currency string

	LightMode   bool
	ThemeButton widget.Clickable

	SortMode         string // "NAME", "BUY", "SELL", "DIST"
	LastSortMode     string // To detect changes
	SortButtons      []widget.Clickable
	Timeframe        string // "1D", "7D", "30D"
	TimeframeButtons []widget.Clickable

	CantorsList widget.List
}

// Notification holds information about a notification banner.
type Notification struct {
	Message string
	Type    string
	Timeout time.Time
}

// FinOpsStatus holds real-time infrastructure metrics for the UI terminal.
type FinOpsStatus struct {
	DailySpendUSD    string            `json:"real_spend_24h_usd"`
	BurnRateStatus   string            `json:"burn_rate_status"`
	BlockedProviders []string          `json:"blocked_providers"`
	SystemTime       string            `json:"system_time"`
	IsActive         bool              `json:"is_governance_active"`
	ServiceBreakdown map[string]string `json:"service_breakdown"` // FOCUS 1.0 categories
}

// AppState holds the overall state of the application.
type AppState struct {
	Window         *app.Window
	Vault          *CantorVault
	CantorsMu      sync.RWMutex
	Cantors        map[string]*CantorInfo
	Search         *search.SearchEngine
	History        *pb.HistoryResponse
	FinOps         FinOpsStatus
	ChartAnimStart time.Time // Tracks when the chart data was last updated for animation
	LastFrameTime  time.Time
	IsLoadingStart time.Time
	IsLoading      atomic.Bool
	LoadingAlpha   float32 // For smooth loading fade in/out
	IsConnected    atomic.Bool
	Notifications  *Notification
	UI             UIState
}

// AppConfig stores app configuration
type AppConfig struct {
	APICantorsURL  string
	APIRatesURL    string
	APIHistoryURL  string
	APIFinOpsURL   string
	APIDiscoverURL string
	DRPCServerURL  string
}

// ApiCantorResponse for parsing JSON
type ApiCantorResponse struct {
	ID          int     `json:"id"`
	DisplayName string  `json:"displayName"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Strategy    string  `json:"strategy"`
	Address     string  `json:"address"`
}

// ModalConfig represents the configuration for a modal UI component.
type ModalConfig struct {
	Title    string
	Options  []string
	Buttons  []widget.Clickable
	OnSelect func(string)
}

// CantorRowConfig groups data required to render a single cantor row
type CantorRowConfig struct {
	CantorID string
	BestBuy  float64
	BestSell float64
}
