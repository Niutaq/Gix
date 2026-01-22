package main

import (
	"fmt"
	"log"
	"github.com/Niutaq/Gix/pkg/scrapers"
)

func main() {
	testCases := []struct {
		ID   string
		S    string
		URL  string
	}{
		{"Tadek SW", "C1", "https://kantorstalowawola.tadek.pl/"},
		{"Exchange", "C2", "https://kantorywalut-rzeszow.pl/kursy-walut"},
		{"Supersam", "C3", "http://www.kantorsupersam.pl/"},
		{"Alex", "C4", "https://kantoralex.rzeszow.pl/"},
		{"Grosz", "C5", "https://kantorgrosz.pl/"},
		{"Centrum", "C6", "https://kantor-centrum.pl/"},
		{"Lider", "C7", "http://kantorlider.pl/"},
		{"Baks", "C8", "https://kantorbaks.pl/"},
		{"Waluciarz", "C9", "https://waluciarz.pl/"},
		{"Joker", "C10", "http://kantorjoker.pl/"},
	}

	fmt.Println("=== Final Real Cantors Test (1-10) ===")
	for _, tc := range testCases {
		scraper, err := scrapers.GetScraper(tc.S)
		if err != nil {
			log.Printf("[%s] Error: %v", tc.ID, err)
			continue
		}
		res, err := scraper(tc.URL, "EUR")
		if err != nil {
			log.Printf("[%s] Failed: %v", tc.ID, err)
		} else {
			fmt.Printf("[%s] OK! EUR: %s / %s\n", tc.ID, res.BuyRate, res.SellRate)
		}
	}
}