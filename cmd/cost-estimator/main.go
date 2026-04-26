package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/pkg/finops"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// main is the entry point for the cost-estimator microservice, setting up JetStream and TimescaleDB connections.
func main() {
	// 1. Configuration
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// 2. Connect to TimescaleDB
	dbPool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer dbPool.Close()

	// 3. Initialize FinOps Estimator
	estimator := finops.NewTimescaleEstimator(dbPool, finops.DefaultRates)

	// 4. Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Unable to connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("Unable to get JetStream context: %v", err)
	}

	// 4.5 Ensure the Stream exists (Self-healing infrastructure)
	streamName := "FINOPS_EVENTS"
	subject := "gix.scrape.v1.completed"
	_, err = js.StreamInfo(streamName)
	if err != nil {
		// Stream doesn't exist, create it
		log.Printf("Stream %s not found. Creating it...", streamName)
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{subject},
			Storage:  nats.FileStorage, // Durable storage for FinOps integrity
		})
		if err != nil {
			log.Fatalf("Could not create stream: %v", err)
		}
	}

	// 5. Subscribe to Scrape Events
	// We use a durable consumer to ensure no events are lost (FinOps integrity)
	sub, err := js.PullSubscribe(subject, "cost-estimator-worker")
	if err != nil {
		log.Fatalf("Unable to subscribe: %v", err)
	}

	log.Println("[Cost-Estimator] Service started. Listening for scrape events...")

	// 6. Processing Loop
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
				if err != nil {
					if err == nats.ErrTimeout {
						continue
					}
					log.Printf("Error fetching messages: %v", err)
					continue
				}

				for _, msg := range msgs {
					processEvent(msg, estimator)
				}
			}
		}
	}()

	<-ctx.Done()
	log.Println("[Cost-Estimator] Shutting down...")
}

// processEvent decodes a NATS JetStream message into a ScrapeCompletedEvent and calculates its cost via the estimator.
func processEvent(msg *nats.Msg, estimator *finops.TimescaleEstimator) {
	var event pb.ScrapeCompletedEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		log.Printf("Error unmarshaling event: %v", err)
		_ = msg.Term() // Poison pill
		return
	}

	unit := finops.UnitCost{
		Time:        time.Unix(event.Timestamp, 0),
		ProviderID:  event.ProviderId,
		ScraperType: finops.ScraperType(event.ScraperType),
		Duration:    time.Duration(event.DurationMs) * time.Millisecond,
		TraceID:     event.TraceId,
	}

	if err := estimator.Estimate(unit); err != nil {
		log.Printf("Error estimating cost for %s: %v", event.ProviderId, err)
		_ = msg.Nak() // Retry later
		return
	}

	if err := msg.Ack(); err != nil {
		log.Printf("Error acking message: %v", err)
	}
	fmt.Printf("[FinOps] Calculated unit cost for %s (%s): %vms\n", 
		event.ProviderId, event.ScraperType, event.DurationMs)
}
