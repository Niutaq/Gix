package scrapers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// HeuristicResult extends ScrapeResult with metadata
type HeuristicResult struct {
	ScrapeResult
	CurrencyCode string
	Confidence   float64 // 0.0 to 1.0
}

var (
	// Regex to find numbers in text (e.g., 4.1234, 4,12, or 4.255)
	numberRegex = regexp.MustCompile(`\d+[.,]\d{2,4}`)
	// Currency code detector used to avoid selecting a whole multi-currency block as one row.
	currencyCodeRegex = regexp.MustCompile(`\b[A-Z]{3}\b`)
	// Keywords that suggest a table column is for buying or selling (Includes Polish and English)
	buyKeywords  = []string{"KUPNO", "BUY", "BID", "MY KUPUJEMY", "WE BUY", "SKUP"}
	sellKeywords = []string{"SPRZEDAŻ", "SELL", "ASK", "MY SPRZEDAJEMY", "WE SELL", "SPRZEDAZ", "SPRZEDA"}
)

// DiscoveredCantor holds metadata found during heuristic discovery.
type DiscoveredCantor struct {
	DisplayName string
	Address     string
	Latitude    float64
	Longitude   float64
}

// HeuristicDiscoverCantor attempts to find cantor name and address from its URL.
func HeuristicDiscoverCantor(urlStr string) (*DiscoveredCantor, error) {
	return HeuristicDiscoverCantorWithContext(context.Background(), urlStr)
}

// addressFromLDItem extracts a postal address from a single JSON-LD node.
func addressFromLDItem(item interface{}) string {
	m, ok := item.(map[string]interface{})
	if !ok {
		return ""
	}

	// Look for address or postalAddress
	addr, ok := m["address"].(map[string]interface{})
	if !ok {
		return ""
	}
	street, _ := addr["streetAddress"].(string)
	city, _ := addr["addressLocality"].(string)
	zip, _ := addr["postalCode"].(string)
	if street != "" && city != "" {
		return fmt.Sprintf("%s, %s %s", street, zip, city)
	}
	return ""
}

// addressFromJSONLD extracts a postal address from a JSON-LD script body.
func addressFromJSONLD(raw string) string {
	var ldMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &ldMap); err != nil {
		return ""
	}
	// Handle both single object and list of objects
	ldList := []interface{}{ldMap}
	if graph, ok := ldMap["@graph"].([]interface{}); ok {
		ldList = graph
	}
	for _, item := range ldList {
		if addr := addressFromLDItem(item); addr != "" {
			return addr
		}
	}
	return ""
}

// findAddressJSONLD looks for an address in JSON-LD (Schema.org) blocks.
func findAddressJSONLD(doc *goquery.Document) string {
	rawAddress := ""
	doc.Find("script[type='application/ld+json']").EachWithBreak(func(i int, s *goquery.Selection) bool {
		rawAddress = addressFromJSONLD(s.Text())
		return rawAddress == ""
	})
	return rawAddress
}

// findAddressMeta looks for an address in meta tags.
func findAddressMeta(doc *goquery.Document) string {
	rawAddress := ""
	doc.Find("meta[name='og:street-address'], meta[property='business:contact_data:street_address']").Each(func(i int, s *goquery.Selection) {
		if content, exists := s.Attr("content"); exists {
			rawAddress = content
		}
	})
	return rawAddress
}

// findAddressRegex falls back to regex searching in common containers.
func findAddressRegex(doc *goquery.Document) string {
	postcodeRegex := regexp.MustCompile(`[0-9]{2}-[0-9]{3}`)
	// Refined street regex to avoid "ulubione" etc.
	streetRegex := regexp.MustCompile(`(?i)(ul\.|al\.|pl\.|ulica|aleja|plac)\s+[A-ZĄĆĘŁŃÓŚŹŻ]`)

	rawAddress := ""
	doc.Find("address, .contact-info, .address, .contact, footer p, footer div").EachWithBreak(func(i int, s *goquery.Selection) bool {
		txt := strings.TrimSpace(s.Text())
		// Ignore very short or very long blocks
		if len(txt) < 10 || len(txt) > 200 {
			return true
		}

		if streetRegex.MatchString(txt) && postcodeRegex.MatchString(txt) {
			rawAddress = txt
			return false
		}
		return true
	})
	return rawAddress
}

// findAddressLLM is the final fallback: offline LLM address extraction.
func findAddressLLM(ctx context.Context, doc *goquery.Document) string {
	llmAddr, err := LLMExtractAddressWithContext(ctx, doc)
	if err != nil {
		log.Printf("Discovery: LLM address extraction failed: %v", err)
		return ""
	}
	return llmAddr
}

// geocodeDiscoveredAddress stores the address on info and geocodes it with Nominatim (OSM).
func geocodeDiscoveredAddress(info *DiscoveredCantor, rawAddress string) {
	info.Address = rawAddress
	// Clean the address (remove multiple spaces, newlines)
	rawAddress = strings.ReplaceAll(rawAddress, "\n", " ")
	re := regexp.MustCompile(`\s+`)
	rawAddress = re.ReplaceAllString(rawAddress, " ")

	// Geocode with Nominatim (OSM).
	lat, lon, err := GeocodeAddress(rawAddress)
	if err != nil {
		log.Printf("Discovery: geocoding failed for %q: %v", rawAddress, err)
		return
	}
	info.Latitude = lat
	info.Longitude = lon
}

// HeuristicDiscoverCantorWithContext attempts to find cantor name and address from its URL.
func HeuristicDiscoverCantorWithContext(ctx context.Context, urlStr string) (*DiscoveredCantor, error) {
	doc, err := fetchDocument(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	info := &DiscoveredCantor{}

	// 1. Try to find the Cantor Name (from h1, title, or .logo)
	title := doc.Find("h1").First().Text()
	if title == "" {
		title = doc.Find("title").First().Text()
	}
	info.DisplayName = cleanHeuristicName(strings.TrimSpace(title), urlStr)

	// 2. Try to find an Address
	// A. Try JSON-LD (Schema.org)
	rawAddress := findAddressJSONLD(doc)
	if rawAddress == "" {
		// B. Try Meta tags
		rawAddress = findAddressMeta(doc)
	}
	if rawAddress == "" {
		// C. Fallback to Regex searching in common containers
		rawAddress = findAddressRegex(doc)
	}
	if rawAddress == "" {
		// D. Final Fallback: offline LLM address extraction
		rawAddress = findAddressLLM(ctx, doc)
	}

	if rawAddress != "" {
		geocodeDiscoveredAddress(info, rawAddress)
	}

	return info, nil
}

// cleanHeuristicName cleans up the raw title from the HTML or falls back to parsing the URL path to generate a clean cantor name.
func cleanHeuristicName(title, link string) string {
	parts := strings.FieldsFunc(title, func(r rune) bool {
		return r == '|' || r == '-' || r == '–' || r == '—' || r == ','
	})

	var bestPart string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// If part has seo words, clean it up but don't discard the whole title
		lowerP := strings.ToLower(p)
		seoWords := []string{"kursy walut", "najlepsze ceny", "detaliczne", "hurtowe"}
		for _, w := range seoWords {
			if strings.Contains(lowerP, w) {
				idx := strings.Index(lowerP, w)
				p = strings.TrimSpace(p[:idx])
				lowerP = strings.ToLower(p)
			}
		}
		if p == "" {
			continue
		}

		if bestPart == "" {
			bestPart = p
		}
		if strings.Contains(lowerP, "kantor") && len(p) < 35 {
			bestPart = p
			break
		}
	}

	if bestPart == "" {
		bestPart = title
	}

	lowerBest := strings.ToLower(bestPart)
	if len(bestPart) > 40 || lowerBest == "kantor" || lowerBest == "" || lowerBest == "mapy.com" || lowerBest == "mapy pl" {
		u, err := url.Parse(link)
		if err == nil && u.Host != "" {
			hostParts := strings.Split(u.Host, ".")
			if len(hostParts) >= 2 {
				if hostParts[0] == "www" {
					hostParts = hostParts[1:]
				}
				hostParts = hostParts[:len(hostParts)-1]

				name := strings.Join(hostParts, " ")

				pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
				if len(pathParts) > 0 && pathParts[len(pathParts)-1] != "" {
					lastSegment := pathParts[len(pathParts)-1]

					// Remove common extensions
					for _, ext := range []string{".html", ".php", ".htm", ".aspx"} {
						lastSegment = strings.TrimSuffix(lastSegment, ext)
					}

					reNum := regexp.MustCompile(`^\d+-`)
					lastSegment = reNum.ReplaceAllString(lastSegment, "")

					genericSegments := map[string]bool{
						"adres": true, "kantor": true, "kantory": true, "waluty": true, "wymiana": true,
						"kontakt": true, "cennik": true, "kursy": true, "kursy-walut": true,
					}

					if !genericSegments[strings.ToLower(lastSegment)] {
						name = lastSegment
					}
				}

				name = strings.ReplaceAll(name, "-", " ")
				name = strings.ReplaceAll(name, "_", " ")
				name = strings.ReplaceAll(name, ",", " ")
				name = strings.ReplaceAll(name, "kantor", "Kantor ")
				name = strings.ReplaceAll(name, "Kantor", "Kantor ")

				words := strings.Fields(name)
				for i, w := range words {
					if len(w) > 0 {
						words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
					}
				}
				name = strings.Join(words, " ")

				if !strings.Contains(strings.ToLower(name), "kantor") {
					name = "Kantor " + name
				}

				if name != "" && name != "Kantor " {
					bestPart = name
				}
			}
		}
	}

	re := regexp.MustCompile(`\s+`)
	bestPart = re.ReplaceAllString(strings.TrimSpace(bestPart), " ")

	if len(bestPart) > 45 {
		bestPart = bestPart[:42] + "..."
	}

	return bestPart
}

// GeocodeAddress converts a text address to Lat/Lon using Nominatim (OSM).
func GeocodeAddress(address string) (float64, float64, error) {
	// Nominatim expects URL-encoded address
	encodedAddr := url.QueryEscape(address)
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1", encodedAddr)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, 0, err
	}
	// Security & Policy: Identification of the application as per Nominatim Usage Policy
	req.Header.Set("User-Agent", "Gix-Currency-App/1.2.8 (Security-Audited; contact@gix.example.com)")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("nominatim error: status %d", resp.StatusCode)
	}

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, err
	}

	if len(results) == 0 {
		return 0, 0, fmt.Errorf("address not found: %s", address)
	}

	lat, errLat := strconv.ParseFloat(results[0].Lat, 64)
	lon, errLon := strconv.ParseFloat(results[0].Lon, 64)
	if errLat != nil || errLon != nil {
		return 0, 0, fmt.Errorf("failed to parse coordinates from geocoder")
	}

	return lat, lon, nil
}

// HeuristicScrape attempts to find exchange rates on any page without prior knowledge of its structure
func HeuristicScrape(ctx context.Context, url, targetCurrency string) (ScrapeResult, error) {
	doc, err := fetchDocument(ctx, url)
	if err != nil {
		return ScrapeResult{}, err
	}

	targetCurrency = strings.ToUpper(strings.TrimSpace(targetCurrency))

	// Strategy 1: Find tables and analyze their structure (Fastest)
	results := analyzeTables(doc, targetCurrency)
	if len(results) > 0 {
		res := results[0].ScrapeResult
		if ok, buy, sell := sanitizePairForCurrency(targetCurrency, res.BuyRate, res.SellRate); ok {
			res.BuyRate, res.SellRate = buy, sell
			res.UsedScraperType = "heuristic"
			return res, nil
		}
		log.Printf("HeuristicScrape (table): rejected out-of-range %s pair %s/%s", targetCurrency, res.BuyRate, res.SellRate)
	}

	// Strategy 2: Look for list-like structures (divs, spans) using Regex/Fallback (Fast)
	res, err := fallbackRowSearch(doc, targetCurrency)
	if err == nil {
		if ok, buy, sell := sanitizePairForCurrency(targetCurrency, res.BuyRate, res.SellRate); ok {
			res.BuyRate, res.SellRate = buy, sell
			res.UsedScraperType = "heuristic"
			return res, nil
		}
		log.Printf("HeuristicScrape (fallback): rejected out-of-range %s pair %s/%s", targetCurrency, res.BuyRate, res.SellRate)
	}

	// Strategy 3: offline LLM only for major FX (keeps harvest + FinOps quiet for exotic pairs).
	if !llmFallbackCurrency(targetCurrency) {
		return ScrapeResult{}, fmt.Errorf("heuristics miss for %s (LLM skipped for %s)", url, targetCurrency)
	}
	log.Printf("Heuristics miss %s [%s] — offline LLM (%s/%s)", url, targetCurrency, llmProvider(), ollamaModel())
	llmRes, llmErr := LLMScrapeFallbackWithContext(ctx, doc, targetCurrency)
	if llmErr != nil {
		return ScrapeResult{}, llmErr
	}
	if ok, buy, sell := sanitizePairForCurrency(targetCurrency, llmRes.BuyRate, llmRes.SellRate); ok {
		llmRes.BuyRate, llmRes.SellRate = buy, sell
		return llmRes, nil
	}
	return ScrapeResult{}, fmt.Errorf("LLM pair out of range for %s: %s/%s", targetCurrency, llmRes.BuyRate, llmRes.SellRate)
}

func llmFallbackCurrency(code string) bool {
	// Keep LLM rare — Ollama on Docker is slow under parallel harvest.
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "EUR", "USD":
		return true
	default:
		return false
	}
}

// analyzeTables analyzes tables on the page and returns a list of heuristic results
func analyzeTables(doc *goquery.Document, target string) []HeuristicResult {
	var results []HeuristicResult

	doc.Find("table").Each(func(i int, table *goquery.Selection) {
		buyCol, sellCol, currencyCol := -1, -1, -1

		// Look at the first few rows to find headers
		rows := table.Find("tr")
		limit := rows.Length()
		if limit > 5 {
			limit = 5
		}
		rows.Slice(0, limit).Each(func(j int, row *goquery.Selection) {
			row.Find("th, td").Each(func(k int, cell *goquery.Selection) {
				txt := strings.ToUpper(strings.TrimSpace(cell.Text()))
				// Match against Polish and English keywords
				if containsAny(txt, buyKeywords) && buyCol == -1 {
					buyCol = k
				} else if containsAny(txt, sellKeywords) && sellCol == -1 {
					sellCol = k
				} else if (strings.Contains(txt, "WALUTA") || strings.Contains(txt, "KOD") || strings.Contains(txt, "CURRENCY") || strings.Contains(txt, "CODE")) && currencyCol == -1 {
					currencyCol = k
				}
			})
		})

		// Search for target in all rows — token match (not substring: "RON" ⊂ "Kroner").
		table.Find("tr").Each(func(j int, row *goquery.Selection) {
			rowText := strings.ToUpper(row.Text())
			if !hasCurrencyToken(rowText, target) {
				return
			}
			// Prefer a dedicated currency column when present.
			if currencyCol >= 0 {
				cells := row.Find("td, th")
				if cells.Length() > currencyCol {
					cellTxt := strings.ToUpper(strings.TrimSpace(cells.Eq(currencyCol).Text()))
					if !hasCurrencyToken(cellTxt, target) {
						return
					}
				}
			} else if containsOtherCurrencyCode(rowText, target) {
				// Ambiguous multi-currency blob without a currency column.
				return
			}

			cells := row.Find("td")
			var buyVal, sellVal string

			if buyCol != -1 && sellCol != -1 && cells.Length() > max(buyCol, sellCol) {
				buyVal = cleanNumber(cells.Eq(buyCol).Text())
				sellVal = cleanNumber(cells.Eq(sellCol).Text())
			} else {
				var nums []string
				cells.Each(func(k int, cell *goquery.Selection) {
					n := cleanNumber(cell.Text())
					if isProbableRate(n) {
						nums = append(nums, n)
					}
				})
				buyVal, sellVal = pickBestPair(target, nums)
			}

			if buyVal != "" && sellVal != "" {
				if ok, b, s := sanitizePairForCurrency(target, buyVal, sellVal); ok {
					results = append(results, HeuristicResult{
						ScrapeResult: ScrapeResult{BuyRate: b, SellRate: s},
						Confidence:   0.8,
					})
				}
			}
		})
	})

	return results
}

// fallbackRowSearch searches for the target currency code in the document and returns the best buy/sell rates found
func fallbackRowSearch(doc *goquery.Document, target string) (ScrapeResult, error) {
	var bestBuy, bestSell string
	found := false

	// Strategy: Find elements that DIRECTLY contain the target currency code
	doc.Find("div, tr, p, li, .rate-item, .row").EachWithBreak(func(i int, s *goquery.Selection) bool {
		// Only consider small elements to avoid capturing the whole table
		txt := strings.TrimSpace(s.Text())
		if len(txt) > 150 || len(txt) < 5 {
			return true
		}

		upperTxt := strings.ToUpper(txt)
		if !hasCurrencyToken(upperTxt, target) {
			return true
		}
		if containsOtherCurrencyCode(upperTxt, target) {
			return true
		}
		// Find numbers in this block or close ancestors (grid layouts).
		var matches []string
		curr := s
		for depth := 0; depth < 3; depth++ {
			if curr == nil {
				break
			}

			rawText := curr.Text()
			rawText = strings.ReplaceAll(rawText, ",", ".")

			reLetter := regexp.MustCompile(`[a-zA-Z:\-]+`)
			rawText = reLetter.ReplaceAllString(rawText, " ")

			reConcat1 := regexp.MustCompile(`(\d+\.\d{3,4})(\d+\.\d+)`)
			for i := 0; i < 3; i++ {
				rawText = reConcat1.ReplaceAllString(rawText, "$1 $2")
			}
			reConcat2 := regexp.MustCompile(`(\d+\.\d{2})(\d+\.\d+)`)
			for i := 0; i < 3; i++ {
				rawText = reConcat2.ReplaceAllString(rawText, "$1 $2")
			}

			matches = numberRegex.FindAllString(rawText, -1)

			if len(matches) >= 2 {
				break
			}
			curr = curr.Parent()
			if curr != nil && len(curr.Text()) > 1000 {
				break
			}
		}

		var validNums []string
		for _, m := range matches {
			mNorm := normalizeRateString(m)
			if isProbableRate(mNorm) {
				validNums = append(validNums, mNorm)
			}
		}

		if len(validNums) >= 2 {
			buy, sell := pickBestPair(target, validNums)
			if buy != "" && sell != "" {
				bestBuy, bestSell = buy, sell
				found = true
				return false
			}
		}
		return true
	})

	if !found {
		return ScrapeResult{}, fmt.Errorf("heuristic search failed for %s", target)
	}
	return ScrapeResult{BuyRate: bestBuy, SellRate: bestSell}, nil
}

func containsOtherCurrencyCode(text, target string) bool {
	skip := map[string]bool{"BUY": true, "BID": true, "ASK": true, "PDF": true, "WWW": true, "PLN": true, "ALL": true}
	for _, code := range currencyCodeRegex.FindAllString(text, -1) {
		if code == target || skip[code] {
			continue
		}
		if hasCurrencyToken(text, code) {
			return true
		}
	}
	return false
}

// hasCurrencyToken reports a real ISO-like code token, not a substring ("RON" in "Kroner").
func hasCurrencyToken(text, code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return false
	}
	re := regexp.MustCompile(`(^|[^A-Z0-9])` + regexp.QuoteMeta(code) + `([^A-Z0-9]|$)`)
	return re.MatchString(strings.ToUpper(text))
}

// expectedPLNRange is a sanity band for kantor quotes vs PLN (unit rate, not per-100).
func expectedPLNRange(code string) (lo, hi float64, ok bool) {
	switch strings.ToUpper(code) {
	case "EUR":
		// Keep EUR/USD bands mostly disjoint so mislabeled rows are rejected.
		return 4.05, 5.20, true
	case "USD":
		// Slight headroom above EUR floor so real kantor ask spikes still pass.
		return 3.20, 4.20, true
	case "GBP":
		return 4.50, 6.20, true
	case "CHF":
		return 4.20, 5.50, true
	case "AUD":
		return 2.0, 3.8, true
	case "CAD":
		return 2.2, 3.8, true
	case "DKK":
		return 0.45, 0.75, true
	case "NOK", "SEK":
		return 0.25, 0.55, true
	case "CZK":
		return 0.12, 0.28, true
	case "HUF":
		return 0.008, 0.02, true
	case "UAH":
		return 0.05, 0.15, true
	case "RON":
		return 0.70, 1.20, true
	case "TRY":
		return 0.05, 0.20, true
	case "BGN":
		return 1.8, 2.8, true
	case "ISK":
		return 0.02, 0.06, true
	case "LEK", "ALL":
		return 0.03, 0.08, true
	default:
		return 0, 0, false
	}
}

// normalizeUnitRate converts common "per 100 units" quotes into a unit PLN rate.
func normalizeUnitRate(code string, v float64) float64 {
	lo, hi, ok := expectedPLNRange(code)
	if !ok || v <= 0 {
		return v
	}
	if v >= lo && v <= hi {
		return v
	}
	// Per-100 quotes (e.g. DKK 55.00 → 0.55).
	if v/100 >= lo && v/100 <= hi {
		return v / 100
	}
	// UAH sometimes shown as 8.35 instead of 0.0835.
	if code == "UAH" && v/100 >= lo && v/100 <= hi {
		return v / 100
	}
	return v
}

func orderBuySell(b, s float64) (float64, float64) {
	if b > s {
		return s, b
	}
	return b, s
}

// isValidPair checks buy/sell are a tight kantor spread (not random distant numbers).
func isValidPair(b, s float64) bool {
	if b <= 0 || s <= 0 {
		return false
	}
	b, s = orderBuySell(b, s)
	ratio := s / b
	return ratio <= 1.08 // ~8% max spread
}

// pairCandidate is a scored buy/sell pair.
type pairCandidate struct {
	b, s, score float64
}

// evalPairCandidate parses, normalizes and validates two adjacent numbers as a buy/sell pair.
func evalPairCandidate(code, bStr, sStr string, mid float64) (pairCandidate, bool) {
	b, err1 := strconv.ParseFloat(bStr, 64)
	s, err2 := strconv.ParseFloat(sStr, 64)
	if err1 != nil || err2 != nil {
		return pairCandidate{}, false
	}
	b = normalizeUnitRate(code, b)
	s = normalizeUnitRate(code, s)
	b, s = orderBuySell(b, s)
	if !isValidPair(b, s) {
		return pairCandidate{}, false
	}
	if lo, hi, ok := expectedPLNRange(code); ok && (b < lo || s > hi) {
		return pairCandidate{}, false
	}
	score := 0.0
	if mid > 0 {
		score = -mathAbs((b+s)/2 - mid)
	}
	return pairCandidate{b: b, s: s, score: score}, true
}

func pickBestPair(code string, nums []string) (buy, sell string) {
	var best *pairCandidate
	mid := 0.0
	if lo, hi, ok := expectedPLNRange(code); ok {
		mid = (lo + hi) / 2
	}
	for i := 0; i < len(nums)-1; i++ {
		c, ok := evalPairCandidate(code, nums[i], nums[i+1], mid)
		if !ok {
			continue
		}
		if best == nil || c.score > best.score {
			best = &c
		}
	}
	if best == nil {
		return "", ""
	}
	return fmt.Sprintf("%.4f", best.b), fmt.Sprintf("%.4f", best.s)
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// SanitizePairForCurrency normalizes + validates a scraped buy/sell vs PLN expectations.
// Exported for harvest/API ingest (reject garbage before Redis/Timescale).
func SanitizePairForCurrency(code, buyStr, sellStr string) (bool, string, string) {
	return sanitizePairForCurrency(code, buyStr, sellStr)
}

func sanitizePairForCurrency(code, buyStr, sellStr string) (bool, string, string) {
	b, err1 := strconv.ParseFloat(strings.TrimSpace(buyStr), 64)
	s, err2 := strconv.ParseFloat(strings.TrimSpace(sellStr), 64)
	if err1 != nil || err2 != nil || b <= 0 || s <= 0 {
		return false, "", ""
	}
	b = normalizeUnitRate(code, b)
	s = normalizeUnitRate(code, s)
	b, s = orderBuySell(b, s)
	if !isValidPair(b, s) {
		return false, "", ""
	}
	if lo, hi, ok := expectedPLNRange(code); ok {
		// Strict band — soft margins caused EUR↔USD swaps (e.g. 3.93 accepted as EUR).
		if b < lo || s > hi {
			return false, "", ""
		}
	} else if b > 100 || s > 100 {
		return false, "", ""
	}
	return true, fmt.Sprintf("%.4f", b), fmt.Sprintf("%.4f", s)
}

// containsAny checks if the text contains any of the keywords
func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if text == kw || strings.Contains(text, " "+kw) || strings.Contains(text, kw+" ") {
			return true
		}
	}
	return false
}

// normalizeRateString checks if a parsed value is abnormally high (e.g. x100) and divides it.
func normalizeRateString(val string) string {
	if val == "" {
		return val
	}
	f, err := strconv.ParseFloat(val, 64)
	if err == nil {
		if f > 100.0 && f < 10000.0 {
			f = f / 100.0
		}
		// Format carefully to remove trailing zeros without losing precision
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return val
}

// cleanNumber cleans a number string by replacing commas with dots and extracting the number
func cleanNumber(val string) string {
	val = strings.ReplaceAll(val, ",", ".")
	match := numberRegex.FindString(val)
	return normalizeRateString(match)
}

// isProbableRate checks if the value is a probable rate (between 0.01 and 100.0)
func isProbableRate(val string) bool {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return false
	}
	// Ignore exact 1, 10, 100 as they are usually quantities
	if f == 1.0 || f == 10.0 || f == 100.0 || f == 1000.0 {
		return false
	}
	return f >= 0.01 && f <= 100.0
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sanitizeLLMAddress(raw string) string {
	addr := strings.TrimSpace(raw)
	addr = strings.TrimPrefix(addr, "```")
	addr = strings.TrimSuffix(addr, "```")
	addr = strings.Join(strings.Fields(addr), " ")
	addr = strings.Trim(addr, "\"'")
	if addr == "" || len(addr) > 200 {
		return ""
	}
	lower := strings.ToLower(addr)
	if strings.Contains(lower, "nie znaleziono") || strings.Contains(lower, "not found") ||
		strings.Contains(lower, "brak adresu") || strings.Contains(lower, "{") || strings.Contains(lower, "<") ||
		strings.Contains(lower, "ignore previous") || strings.Contains(lower, "system prompt") ||
		strings.Contains(lower, "<<<") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		return ""
	}
	// Require a digit (street no / postal) so free-form LLM chatter does not become a Nominatim query.
	hasDigit := false
	for _, r := range addr {
		if r >= '0' && r <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return ""
	}
	return addr
}
