package utilities

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
	"storj.io/drpc/drpcconn"
)

// StartDRPCStream establishes a connection to the dRPC server and processes streaming rate updates in real-time.
func StartDRPCStream(window *app.Window, state *AppState, apiURL string) {
	target := DeriveDRPCTarget(apiURL)

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
			state.IsConnected.Store(false)
			if err := drpcConn.Close(); err != nil {
				log.Printf("Error closing dRPC connection (stream error): %v", err)
			}
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Connected to dRPC stream!")
		state.IsConnected.Store(true)
		processStreamUpdates(window, state, stream)

		state.IsConnected.Store(false)
		if err := drpcConn.Close(); err != nil {
			log.Printf("Error closing dRPC connection (stream end): %v", err)
		}
		time.Sleep(1 * time.Second)
	}
}

// FetchAllRatesRPC performs a single dRPC call to fetch all rates for the given currency.
func FetchAllRatesRPC(window *app.Window, state *AppState, apiURL string) {
	target := DeriveDRPCTarget(apiURL)

	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("FetchAllRatesRPC dial error: %v", err)
		return
	}

	drpcConn := drpcconn.New(conn)
	defer func() {
		// Closing drpcConn also closes the underlying net.Conn
		if err := drpcConn.Close(); err != nil {
			log.Printf("Error closing dRPC connection: %v", err)
		}
	}()

	client := pb.NewDRPCRatesServiceClient(drpcConn)

	log.Printf("Fetching all rates via dRPC for %s...", state.UI.Currency)
	resp, err := client.GetAllRates(context.Background(), &pb.RateRequest{Currency: state.UI.Currency})
	if err != nil {
		log.Printf("FetchAllRatesRPC call error: %v", err)
		return
	}

	updateStateWithRates(state, resp.Results)

	SaveCache(state)

	state.IsLoading.Store(false)
	window.Invalidate()
	log.Printf("Fetched %d rates via dRPC.", len(resp.Results))
}

func updateStateWithRates(state *AppState, results []*pb.RateResponse) {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()

	for _, rate := range results {
		cantorName := findCantorNameByID(state, int(rate.CantorId))
		if cantorName == "" {
			continue
		}

		entry, ok := state.Vault.Rates[cantorName]
		if !ok {
			entry = &CantorEntry{}
			state.Vault.Rates[cantorName] = entry
		}

		entry.Rate.BuyRate = rate.BuyRate
		entry.Rate.SellRate = rate.SellRate
		entry.Rate.Change24h = float64(rate.Change24H) / 100.0
		entry.LoadedAt = time.Now()
		entry.Error = ""
	}
}

func findCantorNameByID(state *AppState, id int) string {
	state.CantorsMu.RLock()
	defer state.CantorsMu.RUnlock()
	for name, info := range state.Cantors {
		if info.ID == id {
			return name
		}
	}
	return ""
}

// processStreamUpdates handles the message loop for the dRPC stream.
func processStreamUpdates(window *app.Window, state *AppState, stream pb.DRPCRatesService_StreamRatesClient) {
	for {
		rate, err := stream.Recv()
		if err != nil {
			log.Printf("dRPC receive error: %v", err)
			return
		}

		if rate.Currency != state.UI.Currency {
			continue
		}

		UpdateStateWithRate(window, state, rate)
	}
}

// UpdateStateWithRate safely updates the application state with a new rate and triggers a UI refresh.
func UpdateStateWithRate(window *app.Window, state *AppState, rate *pb.RateResponse) {
	state.Vault.Mu.Lock()
	defer state.Vault.Mu.Unlock()

	cantorName := findCantorNameByID(state, int(rate.CantorId))
	if cantorName == "" {
		return
	}

	entry, ok := state.Vault.Rates[cantorName]
	if !ok {
		entry = &CantorEntry{}
		state.Vault.Rates[cantorName] = entry
	}

	entry.Rate.BuyRate = rate.BuyRate
	entry.Rate.SellRate = rate.SellRate
	entry.Rate.Change24h = float64(rate.Change24H) / 100.0
	entry.LoadedAt = time.Now()
	entry.Error = ""

	window.Invalidate()
}


// DeriveDRPCTarget parses the given API URL, extracts the host, and appends a default port of 8081 for dRPC connections.
func DeriveDRPCTarget(apiURL string) string {
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
