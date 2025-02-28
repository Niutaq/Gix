package fetching_data

import (
	"context"
	"fmt"
	"github.com/Niutaq/Gix/utilities"
	"github.com/PuerkitoBio/goquery"
	"net/http"
	"strings"
	"time"
)

// HTTP client
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	},
}

// Fetch function : Cantors C1, C2, C3
func FetchRatesC1(ctx context.Context, url, currency string, state *utilities.AppState) (utilities.ExchangeRates, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return utilities.ExchangeRates{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("Error with fetching: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("Error with parsing: %w", err)
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
		return utilities.ExchangeRates{}, fmt.Errorf("%s ...", currency)
	}

	return utilities.ExchangeRates{
		BuyRate:  buyRate,
		SellRate: sellRate,
	}, nil
}

func FetchRatesC2(ctx context.Context, url, currency string, state *utilities.AppState) (utilities.ExchangeRates, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return utilities.ExchangeRates{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("error fetching rates: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("error parsing HTML: %w", err)
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find("table#AutoNumber2 tbody tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if i == 0 {
			return true
		}
		currencyCell := s.Find("td").Eq(1)
		fullText := strings.TrimSpace(currencyCell.Find("b").Text())
		parts := strings.Fields(fullText)
		currentSymbol := strings.ToUpper(parts[len(parts)-1])

		if currentSymbol == targetCurrency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(2).Find("b").Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(3).Find("b").Text())
			return false
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return utilities.ExchangeRates{}, fmt.Errorf("%s ...", currency)
	}

	return utilities.ExchangeRates{
		BuyRate:  buyRate,
		SellRate: sellRate,
	}, nil
}

func FetchRatesC3(ctx context.Context, url, currency string, state *utilities.AppState) (utilities.ExchangeRates, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return utilities.ExchangeRates{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("error fetching rates: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return utilities.ExchangeRates{}, fmt.Errorf("error parsing HTML: %w", err)
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))
	doc.Find("table.mceItemTable:first-child[class*='cellPadding=4'][class*='width=90%'] tr").EachWithBreak(func(i int, s *goquery.Selection) bool {
		if i == 0 {
			return true
		}

		currencyCell := s.Find("td").Eq(0)
		symbolSpan := currencyCell.Find("span[style='FONT-SIZE: medium']")
		currentText := strings.TrimSpace(symbolSpan.Text())

		currentText = strings.ReplaceAll(currentText, "\n", " ")
		currentText = strings.Join(strings.Fields(currentText), " ")

		currentSymbol := strings.TrimSpace(strings.Split(currentText, "(")[0])
		currentSymbol = strings.Split(currentSymbol, " ")[0]

		if currentSymbol == targetCurrency {
			buyRate = strings.TrimSpace(s.Find("td").Eq(2).Text())
			sellRate = strings.TrimSpace(s.Find("td").Eq(3).Text())
			return false
		}
		return true
	})

	if buyRate == "" || sellRate == "" {
		return utilities.ExchangeRates{}, fmt.Errorf("%s ...", currency)
	}

	return utilities.ExchangeRates{
		BuyRate:  buyRate,
		SellRate: sellRate,
	}, nil
}
