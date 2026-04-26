package workers

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/internal/services"
	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/Niutaq/Gix/pkg/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func StartBackgroundHarvester(app *infrastructure.AppState) {
	currencies := types.GlobalCurrencies

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	Harvest(app, currencies)

	for range ticker.C {
		Harvest(app, currencies)
	}
}

func Harvest(app *infrastructure.AppState, currencies []string) {
	log.Println("Background Harvest: Starting parallel collection cycle...")
	ctx := context.Background()

	cantors, err := FetchAllCantors(ctx, app.DB)
	if err != nil {
		log.Printf("Harvest Error (DB): %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, ci := range cantors {
		wg.Add(1)
		go func(info infrastructure.CantorInfo) {
			defer wg.Done()
			for _, curr := range currencies {
				ProcessCantorCurrency(ctx, app, info, curr)
			}
		}(ci)
	}
	wg.Wait()
	log.Println("Background Harvest: Parallel cycle completed.")
}

func FetchAllCantors(ctx context.Context, db *pgxpool.Pool) ([]infrastructure.CantorInfo, error) {
	rows, err := db.Query(ctx, "SELECT id, display_name, base_url, strategy, units FROM cantors")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cantors []infrastructure.CantorInfo
	for rows.Next() {
		var ci infrastructure.CantorInfo
		if err := rows.Scan(&ci.ID, &ci.DisplayName, &ci.BaseURL, &ci.Strategy, &ci.Units); err != nil {
			continue
		}
		cantors = append(cantors, ci)
	}
	return cantors, nil
}

func ProcessCantorCurrency(ctx context.Context, app *infrastructure.AppState, ci infrastructure.CantorInfo, curr string) {
	start := time.Now()
	time.Sleep(500 * time.Millisecond)

	_, rates, err := services.ScrapeAndProcess(ctx, app, ci, ci.ID, curr)
	duration := time.Since(start)

	if err != nil {
		return
	}

	if rates.Buy == 0 && rates.Sell == 0 {
		return
	}

	log.Printf("Harvesting: %s -> %s (%.3f / %.3f) [Perf: %v]", ci.DisplayName, curr, float64(rates.Buy)/infrastructure.MoneyMultiplier, float64(rates.Sell)/infrastructure.MoneyMultiplier, duration)
	finops.Stats.Record(ci.DisplayName, duration)

	services.SaveToArchive(app.DB, ci.ID, curr, rates.Buy, rates.Sell)
	services.UpdateCacheAndNotify(ctx, app, ci.ID, curr, rates)
}
