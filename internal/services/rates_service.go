package services

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/Niutaq/Gix/pkg/scrapers"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

func ScrapeAndProcess(ctx context.Context, app *infrastructure.AppState, ci infrastructure.CantorInfo, id int, currency string) (*pb.RateResponse, infrastructure.ProcessedRates, error) {
	providerIDStr := fmt.Sprintf("%d", id)

	if app.Governance != nil && !app.Governance.IsAllowed(providerIDStr) {
		return nil, infrastructure.ProcessedRates{}, fmt.Errorf("provider %s is currently blocked due to exceeding FinOps budget", providerIDStr)
	}

	start := time.Now()
	scrapeResult, err := runScrapeStrategy(ctx, ci, currency)
	duration := time.Since(start)

	if app.JS != nil {
		st := "static"
		if ci.Strategy == "HEURISTIC" {
			st = "heuristic"
		}
		event := &pb.ScrapeCompletedEvent{
			ProviderId:  providerIDStr,
			ScraperType: st,
			DurationMs:  duration.Milliseconds(),
			Timestamp:   time.Now().Unix(),
			TraceId:     "",
		}
		protoBytes, _ := proto.Marshal(event)
		_, _ = app.JS.Publish("gix.scrape.v1.completed", protoBytes)
	}

	if err != nil {
		return nil, infrastructure.ProcessedRates{}, err
	}

	rates, err := processRates(scrapeResult, ci.Units)
	if err != nil {
		return nil, infrastructure.ProcessedRates{}, fmt.Errorf("rates parsing error: %w", err)
	}

	response := &pb.RateResponse{
		BuyRate:   fmt.Sprintf("%.3f", float64(rates.Buy)/infrastructure.MoneyMultiplier),
		SellRate:  fmt.Sprintf("%.3f", float64(rates.Sell)/infrastructure.MoneyMultiplier),
		CantorId:  int32(id),
		Currency:  currency,
		FetchedAt: time.Now().Unix(),
	}

	if prevBuy, err := GetPreviousRate(app.DB, id, currency); err == nil && prevBuy > 0 {
		change := ((rates.Buy - prevBuy) * 10000) / prevBuy
		response.Change24H = change
	}

	return response, rates, nil
}

func runScrapeStrategy(ctx context.Context, ci infrastructure.CantorInfo, currency string) (scrapers.ScrapeResult, error) {
	scraper, err := scrapers.GetScraper(ci.Strategy)
	if err != nil {
		return scrapers.ScrapeResult{}, err
	}
	return scraper(ctx, ci.BaseURL, currency)
}

func processRates(result scrapers.ScrapeResult, units int) (infrastructure.ProcessedRates, error) {
	buyRateF, errB := strconv.ParseFloat(cleanRate(result.BuyRate), 64)
	sellRateF, errS := strconv.ParseFloat(cleanRate(result.SellRate), 64)

	if errB != nil || errS != nil {
		return infrastructure.ProcessedRates{}, fmt.Errorf("couldn't parse data")
	}

	buyRateInt := int64(buyRateF * infrastructure.MoneyMultiplier)
	sellRateInt := int64(sellRateF * infrastructure.MoneyMultiplier)

	if units > 1 {
		buyRateInt = buyRateInt / int64(units)
		sellRateInt = sellRateInt / int64(units)
	}

	return infrastructure.ProcessedRates{Buy: buyRateInt, Sell: sellRateInt}, nil
}

func cleanRate(raw string) string {
	s := strings.ReplaceAll(raw, ",", ".")
	s = strings.TrimSpace(s)
	var cleaned strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			cleaned.WriteRune(r)
		}
	}
	return cleaned.String()
}

func GetPreviousRate(db *pgxpool.Pool, id int, currency string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var buyRateFloat float64
	err := db.QueryRow(ctx,
		"SELECT buy_rate FROM rates WHERE cantor_id=$1 AND currency=$2 AND time <= NOW() - INTERVAL '24 hours' ORDER BY time DESC LIMIT 1",
		id, currency).Scan(&buyRateFloat)
	return int64(buyRateFloat * infrastructure.MoneyMultiplier), err
}

func SaveToArchive(db *pgxpool.Pool, cantorID int, currency string, buyRate, sellRate int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	buyF := float64(buyRate) / infrastructure.MoneyMultiplier
	sellF := float64(sellRate) / infrastructure.MoneyMultiplier

	_, err := db.Exec(ctx,
		"INSERT INTO rates (time, cantor_id, currency, buy_rate, sell_rate) VALUES (NOW(), $1, $2, $3, $4)",
		cantorID, currency, buyF, sellF,
	)
	if err != nil {
		if strings.Contains(err.Error(), "23503") || strings.Contains(err.Error(), "foreign key constraint") {
			log.Printf("Archive Skip: Cantor %d was deleted, ignoring rate save.", cantorID)
		} else {
			log.Printf("Archive Error: %v", err)
		}
	}
}

func CacheAndArchive(ctx context.Context, app *infrastructure.AppState, key string, id int, curr string,
	resp *pb.RateResponse, rates infrastructure.ProcessedRates) {
	if protoBytes, err := proto.Marshal(resp); err == nil {
		app.Cache.Set(ctx, key, protoBytes, 60*time.Second)
	} else {
		log.Printf("Marshal Error: %v", err)
	}
	go SaveToArchive(app.DB, id, curr, rates.Buy, rates.Sell)
}

func UpdateCacheAndNotify(ctx context.Context, app *infrastructure.AppState, cantorID int, curr string, rates infrastructure.ProcessedRates) {
	cacheKey := fmt.Sprintf("rates:proto%d:%s", cantorID, curr)
	response := &pb.RateResponse{
		BuyRate:   fmt.Sprintf("%.3f", float64(rates.Buy)/infrastructure.MoneyMultiplier),
		SellRate:  fmt.Sprintf("%.3f", float64(rates.Sell)/infrastructure.MoneyMultiplier),
		CantorId:  int32(cantorID),
		Currency:  curr,
		FetchedAt: time.Now().Unix(),
	}

	protoBytes, err := proto.Marshal(response)
	if err != nil {
		log.Printf("Marshal Error: %v", err)
		return
	}

	app.Cache.Set(ctx, cacheKey, protoBytes, 60*time.Second)
	app.Cache.Publish(ctx, "rates_updates", protoBytes)

	if app.JS != nil {
		subject := fmt.Sprintf("rates.%s", curr)
		_, err := app.JS.Publish(subject, protoBytes)
		if err != nil {
			log.Printf("NATS Publish Error: %v", err)
		}
	}
}

func FetchCantorInfo(ctx context.Context, db *pgxpool.Pool, id int) (infrastructure.CantorInfo, error) {
	var ci infrastructure.CantorInfo
	err := db.QueryRow(ctx, "SELECT base_url, strategy, units FROM cantors where id = $1", id).
		Scan(&ci.BaseURL, &ci.Strategy, &ci.Units)
	return ci, err
}
