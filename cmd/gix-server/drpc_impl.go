package main

import (
	// Standard libraries
	"log"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

// RatesDRPCServer implements the dRPC RatesService
type RatesDRPCServer struct {
	pb.DRPCRatesServiceServer
	Cache *redis.Client
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
