package main

import (
	// Standard libraries
	"context"
	"fmt"
	"log"
	"time"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// RatesDRPCServer implements the dRPC RatesService
type RatesDRPCServer struct {
	pb.DRPCRatesServiceServer
	Cache redis.UniversalClient
	DB    *pgxpool.Pool
}

// GetAllRates retrieves the latest rates for all cantors for a specific currency, including 24h change.
func (s *RatesDRPCServer) GetAllRates(ctx context.Context, req *pb.RateRequest) (*pb.RateListResponse, error) {
	currency := req.Currency
	if currency == "" {
		return nil, fmt.Errorf("currency is required")
	}

	query := `
		WITH latest AS (
			SELECT DISTINCT ON (cantor_id) cantor_id, buy_rate, sell_rate, time
			FROM rates WHERE currency = $1 ORDER BY cantor_id, time DESC
		),
		past AS (
			SELECT DISTINCT ON (cantor_id) cantor_id, buy_rate
			FROM rates
			WHERE currency = $1 AND time <= NOW() - INTERVAL '24 hours'
			ORDER BY cantor_id, time DESC
		)
		SELECT l.cantor_id, l.buy_rate, l.sell_rate, l.time, COALESCE(p.buy_rate, 0)
		FROM latest l
		LEFT JOIN past p ON l.cantor_id = p.cantor_id;
	`

	rows, err := s.DB.Query(ctx, query, currency)
	if err != nil {
		log.Printf("GetAllRates DB Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []*pb.RateResponse

	for rows.Next() {
		var cantorID int
		var buy, sell, pastBuy float64
		var t time.Time

		if err := rows.Scan(&cantorID, &buy, &sell, &t, &pastBuy); err != nil {
			log.Printf("GetAllRates Scan Error: %v", err)
			continue
		}

		change := 0.0
		if pastBuy > 0 {
			change = ((buy - pastBuy) / pastBuy) * 100
		}

		results = append(results, &pb.RateResponse{
			BuyRate:   fmt.Sprintf("%.4f", buy),
			SellRate:  fmt.Sprintf("%.4f", sell),
			CantorId:  int32(cantorID),
			Currency:  currency,
			FetchedAt: t.Unix(),
			Change24H: change,
		})
	}

	return &pb.RateListResponse{Results: results}, nil
}

// StreamRates streams real-time rate updates to the dRPC client, filtering by requested currencies if specified.
func (s *RatesDRPCServer) StreamRates(req *pb.StreamRatesRequest, stream pb.DRPCRatesService_StreamRatesStream) error {
	ctx := stream.Context()
	log.Println("New dRPC stream client connected")
	pubsub := s.Cache.Subscribe(ctx, "rates_updates")
	defer func() { _ = pubsub.Close() }()

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			log.Println("dRPC client disconnected")
			return ctx.Err()
		case msg := <-ch:
			var rate pb.RateResponse
			if err := proto.Unmarshal([]byte(msg.Payload), &rate); err != nil {
				log.Printf("Failed to unmarshal update: %v", err)
				continue
			}

			if !s.shouldSendRate(req, &rate) {
				continue
			}

			if err := stream.Send(&rate); err != nil {
				return err
			}
		}
	}
}

// shouldSendRate checks if the rate update matches the client's requested currencies.
func (s *RatesDRPCServer) shouldSendRate(req *pb.StreamRatesRequest, rate *pb.RateResponse) bool {
	if len(req.Currencies) == 0 {
		return true
	}
	for _, c := range req.Currencies {
		if c == rate.Currency {
			return true
		}
	}
	return false
}
