package scrapers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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

// FetchC1 - scrapes C1
func FetchC1(url, currency string) (ScrapeResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ScrapeResult{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ScrapeResult{}, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
		return ScrapeResult{}, fmt.Errorf("not found rates for: %s", currency)
	}
	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}

// FetchC2 - scrapes C2
func FetchC2(url, currency string) (ScrapeResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ScrapeResult{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ScrapeResult{}, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("table#AutoNumber2 tbody tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if i == 0 {
			return true
		} // Pomiń nagłówek

		currencyCell := s.Find("td").Eq(1)
		fullText := strings.TrimSpace(currencyCell.Find("b").Text())
		parts := strings.Fields(fullText)
		currentSymbol := ""
		if len(parts) > 0 {
			currentSymbol = strings.ToUpper(parts[len(parts)-1])
		}

		if currentSymbol == targetCurrency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(2).Find("b").Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(3).Find("b").Text())
			return false
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ScrapeResult{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ScrapeResult{}, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
		return ScrapeResult{}, fmt.Errorf("not found rates for: %s", currency)
	}

	return ScrapeResult{BuyRate: buyRate, SellRate: sellRate}, nil
}
