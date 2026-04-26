package rpc

import (
	"context"
	"log"
	"net"

	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"
)

func StartDRPCServer(db *pgxpool.Pool, cache redis.UniversalClient) {
	lis, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("failed to listen for dRPC: %v", err)
	}
	mux := drpcmux.New()
	err = pb.DRPCRegisterRatesService(mux, &RatesDRPCServer{Cache: cache, DB: db})
	if err != nil {
		log.Fatalf("failed to register dRPC service: %v", err)
	}
	srv := drpcserver.New(mux)
	log.Println("dRPC server listening on :8081")
	if err := srv.Serve(context.Background(), lis); err != nil {
		log.Fatalf("dRPC serve error: %v", err)
	}
}
