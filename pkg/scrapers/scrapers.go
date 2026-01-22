package scrapers

import (
	// Standard libraries
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	// External utilities
	"github.com/PuerkitoBio/goquery"
)

// Constants
const (
	errorClosingResponseBody = "Error closing response body: %v\n"
	errorNotFoundRates       = "not found rates for: %s"
)

// ScrapeResult - struct for storing the scraped data
type ScrapeResult struct {
	BuyRate  string
	SellRate string
}

// ScrapeFunc defines the signature for a scraping function
type ScrapeFunc func(url, currency string) (ScrapeResult, error)

var (
	// registry stores available scraper strategies
	registry = make(map[string]ScrapeFunc)
	// mu protects the registry for concurrent access (though mainly used at startup)
	mu sync.RWMutex
)

// Register adds a scraper strategy to the registry
func Register(name string, scraper ScrapeFunc) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = scraper
}

// GetScraper retrieves a scraper strategy by name
func GetScraper(name string) (ScrapeFunc, error) {
	mu.RLock()
	defer mu.RUnlock()
	scraper, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("scraper strategy not found: %s", name)
	}
	return scraper, nil
}

// init registers the default scrapers
func init() {
	Register("C1", FetchC1)
	Register("C2", FetchC2)
	Register("C3", FetchC3)
	Register("C4", FetchC4)
	Register("C5", FetchC5)
	Register("C6", FetchC6)
	Register("C7", FetchGenericTable)
	Register("C8", FetchGenericTable)
	Register("C9", FetchGenericTable)
	Register("C10", FetchGenericTable)
}

// FetchGenericTable is a fallback scraper that looks for the currency code in any table row
func FetchGenericTable(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("tr, div.row, div.rate-row").EachWithBreak(func(i int, s *goquery.Selection) bool {
		text := strings.ToUpper(s.Text())
		if strings.Contains(text, targetCurrency) {
			// Find all numbers in this row
			var numbers []string
			s.Find("td, span, div").Each(func(j int, cell *goquery.Selection) {
				val := strings.TrimSpace(cell.Text())
				val = strings.ReplaceAll(val, ",", ".")
				// Simple regex-like check for number
				if len(val) >= 4 && (val[0] >= '0' && val[0] <= '9') {
					numbers = append(numbers, val)
				}
			})
			if len(numbers) >= 2 {
				buyRate = numbers[0]
				sellRate = numbers[1]
				return false
			}
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// httpClient - global http client
var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// fetchDocument - performs HTTP GET and returns a parsed HTML document
func fetchDocument(url string) (*goquery.Document, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			fmt.Printf(errorClosingResponseBody, err)
		}
	}(resp.Body)

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// FetchC1 - scrapes C1
func FetchC1(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	doc.Find("table.kursy_walut tbody tr").Each(func(i int, s *goquery.Selection) {
		symbol := strings.TrimSpace(s.Find("td").Eq(1).Text())
		if symbol == currency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(3).Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(4).Text())
		}
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC2 - scrapes C2
func FetchC2(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	log.Printf("DEBUG: --- Starting C2 Analysis for %s ---", url)

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find(".offerItem").EachWithBreak(func(i int, s *goquery.Selection) bool {
		text := strings.ToUpper(s.Text())

		if strings.Contains(text, targetCurrency) {

			buyRate = strings.TrimSpace(s.Find(".offerItem__exchangeBuy").Text())
			sellRate = strings.TrimSpace(s.Find(".offerItem__exchangeSell").Text())

			if buyRate != "" && sellRate != "" {
				log.Printf("DEBUG: Hit for %s! Buy: %s, Sell: %s", currency, buyRate, sellRate)
				return false
			}
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf("not found rates for: %s", currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC3 - scrapes C3
func FetchC3(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("table.mceItemTable:first-child tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if i == 0 {
			return true
		}

		currencyCell := s.Find("td").Eq(0)
		symbolSpan := currencyCell.Find("span[style='FONT-SIZE: medium']")
		currentText := strings.TrimSpace(symbolSpan.Text())

		currentText = strings.ReplaceAll(currentText, "\n", " ")
		currentText = strings.Join(strings.Fields(currentText), " ")

		currentSymbol := strings.TrimSpace(strings.Split(currentText, "(")[0])
		currentSymbolParts := strings.Fields(currentSymbol)
		if len(currentSymbolParts) > 0 {
			currentSymbol = strings.ToUpper(currentSymbolParts[len(currentSymbolParts)-1])
		} else {
			currentSymbol = ""
		}

		if currentSymbol == targetCurrency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(2).Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(3).Text())
			return false
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}

	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC4 - scrapes C4
func FetchC4(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	// Alex uses a specific div structure (Divi builder)
	doc.Find(".et_pb_column").EachWithBreak(func(i int, col *goquery.Selection) bool {
		text := strings.ToUpper(col.Text())
		if strings.Contains(text, targetCurrency) {
			// Inside this column there are multiple et_pb_text_inner
			// Usually: [0]=CurrencyName, [1]=BuyRate, [2]=SellRate
			rates := col.Find(".et_pb_text_inner")
			if rates.Length() >= 3 {
				buyRate = strings.TrimSpace(rates.Eq(1).Text())
				sellRate = strings.TrimSpace(rates.Eq(2).Text())
				return false
			}
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC5 - scrapes C5
func FetchC5(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("table tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		currencyCell := strings.ToUpper(strings.TrimSpace(s.Find("td").Eq(1).Text()))
		if strings.Contains(currencyCell, targetCurrency) {
			buyRate = strings.TrimSpace(s.Find("td").Eq(2).Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(3).Text())
			return false
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC6 - scrapes C6
func FetchC6(url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("table tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		found := false
		s.Find("td").EachWithBreak(func(j int, cell *goquery.Selection) bool {
			if strings.TrimSpace(strings.ToUpper(cell.Text())) == targetCurrency {
				buyRate = strings.TrimSpace(s.Find("td").Eq(j + 1).Text())
				sellRate = strings.TrimSpace(s.Find("td").Eq(j + 2).Text())
				found = true
				return false
			}
			return true
		})
		return !found
	})

	if buyRate == "" || sellRate == "" {
		return ScrapeResult{}, fmt.Errorf(errorNotFoundRates, currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}
