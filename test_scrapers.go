package main

import (
	"fmt"
	"log"
	"github.com/Niutaq/Gix/pkg/scrapers"
)

func main() {
	testCases := []struct {
		Strategy string
		Name     string
		URL      string
		Currency string
	}{
		{"C4", "Alex", "https://kantoralex.rzeszow.pl/", "EUR"},
		{"C5", "Grosz", "https://kantorgrosz.pl/", "EUR"},
		{"C6", "Centrum", "https://kantor-centrum.pl/", "EUR"},
	}

	fmt.Println("=== Testing New Scrapers (C4, C5, C6) ===")
	for _, tc := range testCases {
		scraper, err := scrapers.GetScraper(tc.Strategy)
		if err != nil {
			log.Printf("[%s] Error getting scraper: %v", tc.Name, err)
			continue
		}

		result, err := scraper(tc.URL, tc.Currency)
		if err != nil {
			log.Printf("[%s] Error scraping %s: %v", tc.Name, tc.Currency, err)
			continue
		}

		fmt.Printf("[%s] Success! %s: Buy=%s, Sell=%s\n", tc.Name, tc.Currency, result.BuyRate, result.SellRate)
	}
}