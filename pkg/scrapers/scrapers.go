package scrapers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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

// httpClient - global http client
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// fetchDocument - performs HTTP GET and returns a parsed HTML document
func fetchDocument(url string) (*goquery.Document, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

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
