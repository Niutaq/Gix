package infrastructure

import (
	"time"

	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/Niutaq/Gix/pkg/search"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

const MoneyMultiplier = 1000

type AppState struct {
	DB         *pgxpool.Pool
	Cache      redis.UniversalClient
	JS         nats.JetStreamContext
	Search     *search.SearchEngine
	Governance *finops.GovernanceEngine
}

type CantorInfo struct {
	ID          int
	DisplayName string
	BaseURL     string
	Strategy    string
	Units       int
	Address     string
}

type CantorListResponse struct {
	ID          int     `json:"id"`
	DisplayName string  `json:"displayName"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Strategy    string  `json:"strategy"`
	Address     string  `json:"address"`
}

type HistoryParams struct {
	CantorID int
	Days     int
	Cutoff   time.Time
}

type ProcessedRates struct {
	Buy  int64
	Sell int64
}
