package main

import (
	// Standard libraries
	"context"
	"log"
	"net"
	"strings"
	"time"

	// Gio utilities
	"gioui.org/app"

	// External utilities
	pb "github.com/Niutaq/Gix/api/proto/v1"
	"github.com/Niutaq/Gix/pkg/utilities"
	"storj.io/drpc/drpcconn"
)

// startDRPCStream establishes a connection to the dRPC server and processes streaming rate updates in real-time.
func startDRPCStream(window *app.Window, state *utilities.AppState, apiURL string) {
	target := deriveDRPCTarget(apiURL)

	for {
		log.Printf("Connecting to dRPC server at %s...", target)
		conn, err := net.Dial("tcp", target)
		if err != nil {
			log.Printf("dRPC dial error: %v. Retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		drpcConn := drpcconn.New(conn)
		client := pb.NewDRPCRatesServiceClient(drpcConn)

		stream, err := client.StreamRates(context.Background(), &pb.StreamRatesRequest{})
		if err != nil {
			log.Printf("dRPC stream error: %v. Retrying in 5s...", err)
			err := drpcConn.Close()
			if err != nil {
				return
			}
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Connected to dRPC stream!")

		for {
			rate, err := stream.Recv()
			if err != nil {
				log.Printf("dRPC receive error: %v", err)
				break
			}

			// Filter: only update if the currency matches what the user is looking at
			if rate.Currency != state.UI.Currency {
				continue
			}

			// Update state
			state.Vault.Mu.Lock()
			var cantorName string
			for name, info := range state.Cantors {
				if info.ID == int(rate.CantorId) {
					cantorName = name
					break
				}
			}

			if cantorName != "" {
				entry, ok := state.Vault.Rates[cantorName]
				if !ok {
					entry = &utilities.CantorEntry{}
					state.Vault.Rates[cantorName] = entry
				}
				entry.Rate.BuyRate = rate.BuyRate
				entry.Rate.SellRate = rate.SellRate
				entry.LoadedAt = time.Now()
				// Clear any previous error
				entry.Error = ""

				// Force UI refresh
				window.Invalidate()
			}
			state.Vault.Mu.Unlock()
		}
		err2 := drpcConn.Close()
		if err2 != nil {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// deriveDRPCTarget parses the given API URL, extracts the host, and appends a default port of 8081 for dRPC connections.
func deriveDRPCTarget(apiURL string) string {
	// Remove scheme
	host := apiURL
	if strings.HasPrefix(host, "http://") {
		host = host[7:]
	} else if strings.HasPrefix(host, "https://") {
		host = host[8:]
	}

	// Remove path if any
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	// Split host/port
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			h = host
		} else {
			// Fallback
			return "localhost:8081"
		}
	}

	if h == "" {
		return "localhost:8081"
	}

	return h + ":8081"
}
