package scrapers

import (
	// Standard libraries
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	awsMetadataIP            = "169.254.169.254"
)

// ScrapeResult - struct for storing the scraped data
type ScrapeResult struct {
	BuyRate         string
	SellRate        string
	UsedScraperType string // "static", "heuristic", or "llm"
}

// ScrapeFunc defines the signature for a scraping function
type ScrapeFunc func(ctx context.Context, url, currency string) (ScrapeResult, error)

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
	// Register("C4", FetchC4) // Kantor Alex - Disabled due to poor performance (expensive task)
	Register("C5", FetchC5)
	Register("C6", FetchC6)
	Register("HEURISTIC", HeuristicScrape)
	Register("C7", FetchGenericTable)
	Register("C8", FetchGenericTable)
	Register("C9", FetchGenericTable)
	Register("C10", FetchGenericTable)
}

// FetchGenericTable is a fallback scraper that looks for the currency code in any table row
func FetchGenericTable(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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

// httpClient - global http client with security timeouts
var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// AllowLocalhostForTesting is a flag used to bypass SSRF protections during unit tests.
var AllowLocalhostForTesting bool

// validateURL checks if the URL is safe to fetch (SSRF protection)
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}

	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, we might still allow if it's a direct IP, but we'll be strict
		ip := net.ParseIP(host)
		if ip != nil {
			ips = []net.IP{ip}
		} else {
			return fmt.Errorf("could not resolve host: %s", host)
		}
	}

	for _, ip := range ips {
		if AllowLocalhostForTesting && ip.IsLoopback() {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.To4() == nil && !ip.IsGlobalUnicast() {
			return fmt.Errorf("access to internal/private IP blocked: %s", ip.String())
		}
		// Metadata service blocking (AWS/GCP/Azure)
		if ip.String() == awsMetadataIP {
			return fmt.Errorf("access to metadata service blocked")
		}
	}

	return nil
}

type cachedDoc struct {
	data      []byte
	expiresAt time.Time
}

var (
	docCache   = make(map[string]cachedDoc)
	docCacheMu sync.RWMutex
)

// fetchDocument - performs HTTP GET and returns a parsed HTML document
func fetchDocument(ctx context.Context, url string) (*goquery.Document, error) {
	// Check cache first
	docCacheMu.RLock()
	cached, exists := docCache[url]
	docCacheMu.RUnlock()

	if exists && time.Now().Before(cached.expiresAt) {
		// Parse document from cached bytes
		return goquery.NewDocumentFromReader(bytes.NewReader(cached.data))
	}

	// Security: Validate URL before fetching
	if err := validateURL(url); err != nil {
		return nil, fmt.Errorf("security block: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Gix-App/1.2.8; Security-Audited)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			fmt.Printf(errorClosingResponseBody, err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	// Read body into bytes for caching
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Store in cache for 30 seconds
	docCacheMu.Lock()
	docCache[url] = cachedDoc{
		data:      bodyBytes,
		expiresAt: time.Now().Add(30 * time.Second),
	}
	docCacheMu.Unlock()

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// FetchC1 - scrapes C1
func FetchC1(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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
func FetchC2(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
	if err != nil {
		return ScrapeResult{}, err
	}

	var buyRate, sellRate string
	targetCurrency := strings.ToUpper(strings.TrimSpace(currency))

	doc.Find(".offerItem").EachWithBreak(func(i int, s *goquery.Selection) bool {
		text := strings.ToUpper(s.Text())

		if strings.Contains(text, targetCurrency) {

			buyRate = strings.TrimSpace(s.Find(".offerItem__exchangeBuy").Text())
			sellRate = strings.TrimSpace(s.Find(".offerItem__exchangeSell").Text())

			if buyRate != "" && sellRate != "" {
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
func FetchC3(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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
func FetchC4(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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
func FetchC5(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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
func FetchC6(ctx context.Context, url, currency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
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
