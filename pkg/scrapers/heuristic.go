package scrapers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"log"

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
	doc, err := fetchDocument(context.TODO(), urlStr)
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
	var rawAddress string

	// A. Try JSON-LD (Schema.org)
	doc.Find("script[type='application/ld+json']").EachWithBreak(func(i int, s *goquery.Selection) bool {
		var ldMap map[string]interface{}
		if err := json.Unmarshal([]byte(s.Text()), &ldMap); err == nil {
			// Handle both single object and list of objects
			ldList := []interface{}{ldMap}
			if graph, ok := ldMap["@graph"].([]interface{}); ok {
				ldList = graph
			}

			for _, item := range ldList {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				// Look for address or postalAddress
				if addr, ok := m["address"].(map[string]interface{}); ok {
					street, _ := addr["streetAddress"].(string)
					city, _ := addr["addressLocality"].(string)
					zip, _ := addr["postalCode"].(string)
					if street != "" && city != "" {
						rawAddress = fmt.Sprintf("%s, %s %s", street, zip, city)
						return false
					}
				}
			}
		}
		return true
	})

	if rawAddress == "" {
		// B. Try Meta tags
		doc.Find("meta[name='og:street-address'], meta[property='business:contact_data:street_address']").Each(func(i int, s *goquery.Selection) {
			if content, exists := s.Attr("content"); exists {
				rawAddress = content
			}
		})
	}

	if rawAddress == "" {
		// C. Fallback to Regex searching in common containers
		postcodeRegex := regexp.MustCompile(`[0-9]{2}-[0-9]{3}`)
		// Refined street regex to avoid "ulubione" etc.
		streetRegex := regexp.MustCompile(`(?i)(ul\.|al\.|pl\.|ulica|aleja|plac)\s+[A-ZĄĆĘŁŃÓŚŹŻ]`)

		doc.Find("address, .contact-info, .address, .contact, footer p, footer div").EachWithBreak(func(i int, s *goquery.Selection) bool {
			txt := strings.TrimSpace(s.Text())
			// Ignore very short or very long blocks
			if len(txt) < 10 || len(txt) > 200 {
				return true
			}

			hasStreet := streetRegex.MatchString(txt)
			hasPostcode := postcodeRegex.MatchString(txt)

			if hasStreet && hasPostcode {
				rawAddress = txt
				return false
			}
			return true
		})
	}

	if rawAddress == "" {
		// D. Final Fallback: Use Gemini LLM to extract the address
		log.Printf("Discovery: Regex failed. Using LLM (Gemini) for address extraction...")
		llmAddr, err := LLMExtractAddress(doc)
		if err == nil && llmAddr != "" {
			rawAddress = llmAddr
			log.Printf("Discovery: LLM found address: '%s'", rawAddress)
		}
	}

	if rawAddress != "" {
		info.Address = rawAddress
		// Clean the address (remove multiple spaces, newlines)
		rawAddress = strings.ReplaceAll(rawAddress, "\n", " ")
		re := regexp.MustCompile(`\s+`)
		rawAddress = re.ReplaceAllString(rawAddress, " ")

		log.Printf("Discovery: Found address for geocoding: '%s'", rawAddress)

		// 3. Geocode with Nominatim (OSM)
		lat, lon, err := GeocodeAddress(rawAddress)
		if err == nil {
			log.Printf("Discovery: Geocoding successful: %f, %f", lat, lon)
			info.Latitude = lat
			info.Longitude = lon
		} else {
			log.Printf("Discovery: Geocoding failed for '%s': %v", rawAddress, err)
		}
	}

	return info, nil
}

// cleanHeuristicName cleans up the raw title from the HTML or falls back to parsing the URL path to generate a clean cantor name.
func cleanHeuristicName(title, link string) string {
	log.Printf("Discovery: cleanHeuristicName input title='%s', link='%s'", title, link)
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

	log.Printf("Discovery: cleanHeuristicName output='%s'", bestPart)
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
		res.UsedScraperType = "heuristic"
		return res, nil
	}

	// Strategy 2: Look for list-like structures (divs, spans) using Regex/Fallback (Fast)
	res, err := fallbackRowSearch(doc, targetCurrency)
	if err == nil {
		res.UsedScraperType = "heuristic"
		return res, nil
	}

	// Strategy 3: LLM Fallback (Slow, final attempt using AI)
	log.Printf("Heuristics failed for %s. Calling LLM model (Gemini)...", url)
	return LLMScrapeFallback(doc, targetCurrency)
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

		// Search for target in all rows
		table.Find("tr").Each(func(j int, row *goquery.Selection) {
			rowText := strings.ToUpper(row.Text())
			if strings.Contains(rowText, target) {
				cells := row.Find("td")
				var buyVal, sellVal string

				if buyCol != -1 && sellCol != -1 && cells.Length() > max(buyCol, sellCol) {
					buyVal = cleanNumber(cells.Eq(buyCol).Text())
					sellVal = cleanNumber(cells.Eq(sellCol).Text())
				} else {
					// Guess based on any numbers in the row
					var nums []string
					cells.Each(func(k int, cell *goquery.Selection) {
						n := cleanNumber(cell.Text())
						if isProbableRate(n) {
							nums = append(nums, n)
						}
					})
					if len(nums) >= 2 {
						for n := 0; n < len(nums)-1; n++ {
							b, _ := strconv.ParseFloat(nums[n], 64)
							s, _ := strconv.ParseFloat(nums[n+1], 64)
							if isValidPair(b, s) {
								if b > s {
									buyVal, sellVal = fmt.Sprintf("%.4f", s), fmt.Sprintf("%.4f", b)
								} else {
									buyVal, sellVal = fmt.Sprintf("%.4f", b), fmt.Sprintf("%.4f", s)
								}
								log.Printf("HeuristicScrape (table): guessed from %v -> buy: %s, sell: %s", nums, buyVal, sellVal)
								break
							}
						}
						if buyVal == "" {
							log.Printf("HeuristicScrape (table): no valid pair found in %v", nums)
						}
					}
				}

				if buyVal != "" && sellVal != "" {
					b, _ := strconv.ParseFloat(buyVal, 64)
					s, _ := strconv.ParseFloat(sellVal, 64)
					if isValidPair(b, s) {
						if b > s {
							buyVal, sellVal = fmt.Sprintf("%.4f", s), fmt.Sprintf("%.4f", b)
						} else {
							buyVal, sellVal = fmt.Sprintf("%.4f", b), fmt.Sprintf("%.4f", s)
						}
						log.Printf("HeuristicScrape (table): target '%s' found valid pair buy: %s, sell: %s", target, buyVal, sellVal)
						results = append(results, HeuristicResult{
							ScrapeResult: ScrapeResult{BuyRate: buyVal, SellRate: sellVal},
							Confidence:   0.8,
						})
					} else {
						log.Printf("HeuristicScrape (table): target '%s' ignored invalid pair buy: %s, sell: %s", target, buyVal, sellVal)
					}
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

		// Check if this specific element (or its immediate text) contains the target
		upperTxt := strings.ToUpper(txt)
		if strings.Contains(upperTxt, target) {
			// Find all numbers in this block or its ancestors (to handle grid layouts)
			// Strategy: search in the element itself, then its parent, then its grandparent.
			var matches []string
			curr := s
			for depth := 0; depth < 3; depth++ {
				if curr == nil {
					break
				}

				// Fix spaced thousands like 4 123.00, or 4.2554 right next to word
				rawText := curr.Text()
				rawText = strings.ReplaceAll(rawText, ",", ".")

				// Fix sites that concatenate numbers like "4.25544.6154" or "4.2554.28" by inserting spaces before decimals
				reLetter := regexp.MustCompile(`[a-zA-Z:\-]+`)
				rawText = reLetter.ReplaceAllString(rawText, " ")

				// 1. Separate digits grouped with a single dot from the next sequence
				// Example: "4.25544.6154" -> "4.2554 4.6154"
				// Match a dot followed by digits, then another digit followed by a dot
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
				// Don't look at too large containers (like body)
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

			// If we found at least 2 numbers in a small container with the currency code,
			// it's very likely our row
			if len(validNums) >= 2 {
				for n := 0; n < len(validNums)-1; n++ {
					b, _ := strconv.ParseFloat(validNums[n], 64)
					s, _ := strconv.ParseFloat(validNums[n+1], 64)
					if isValidPair(b, s) {
						if b > s {
							bestBuy, bestSell = fmt.Sprintf("%.4f", s), fmt.Sprintf("%.4f", b)
						} else {
							bestBuy, bestSell = fmt.Sprintf("%.4f", b), fmt.Sprintf("%.4f", s)
						}
						log.Printf("HeuristicScrape (fallback): target '%s' in text '%s' found valid pair from %v -> buy: %s, sell: %s", target, txt, validNums, bestBuy, bestSell)
						found = true
						break
					}
				}
				if found {
					return false
				} else {
					log.Printf("HeuristicScrape (fallback): ignored row, no valid pair in %v", validNums)
				}
			}
		}
		return true
	})

	if !found {
		return ScrapeResult{}, fmt.Errorf("heuristic search failed for %s", target)
	}
	return ScrapeResult{BuyRate: bestBuy, SellRate: bestSell}, nil
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

// isValidPair checks if the two rates are close enough to be a buy/sell pair
func isValidPair(b, s float64) bool {
	if b <= 0 || s <= 0 {
		return false
	}
	ratio := s / b
	if ratio < 1.0 {
		ratio = b / s
	}
	return ratio <= 1.5 // Max 50% spread, otherwise it's probably a rate and a commission/spread
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GeminiRequest represents the request body for Google's Gemini API.
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiResponse represents the simplified response from Gemini API.
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// LLMScrapeFallback uses artificial intelligence when heuristics fail.
func LLMScrapeFallback(doc *goquery.Document, targetCurrency string) (ScrapeResult, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return ScrapeResult{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Extract clean text from the page to avoid clogging the LLM with HTML tags.
	cleanText := strings.Join(strings.Fields(doc.Find("body").Text()), " ")

	// Shorten the text to stay within token limits.
	if len(cleanText) > 6000 {
		cleanText = cleanText[:6000]
	}

	prompt := fmt.Sprintf(`Jesteś precyzyjnym ekstraktorem danych. Znajdź aktualny kurs kupna i sprzedaży dla waluty %s w podanym tekście z polskiego kantoru.
Zwróć TYLKO I WYŁĄCZNIE czysty obiekt JSON w formacie: {"buy": "4.25", "sell": "4.30"}.
Jeśli nie ma tych danych w tekście, zwróć {"buy": "", "sell": ""}.
Tekst: %s`, targetCurrency, cleanText)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{{Text: prompt}},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return ScrapeResult{}, err
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return ScrapeResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	// LLM needs more time than standard scrapers.
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("LLM Fallback Error (Gemini connection): %v", err)
		return ScrapeResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ScrapeResult{}, fmt.Errorf("gemini returned status: %d", resp.StatusCode)
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return ScrapeResult{}, err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return ScrapeResult{}, fmt.Errorf("gemini returned empty response")
	}

	rawJSON := geminiResp.Candidates[0].Content.Parts[0].Text
	// Remove markdown blocks if present
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var extractedData map[string]string
	if err := json.Unmarshal([]byte(rawJSON), &extractedData); err != nil {
		return ScrapeResult{}, fmt.Errorf("llm returned invalid json: %s", rawJSON)
	}

	buy := extractedData["buy"]
	sell := extractedData["sell"]

	if buy == "" || sell == "" {
		return ScrapeResult{}, fmt.Errorf("llm did not find rates for %s", targetCurrency)
	}

	log.Printf("LLM Success! Extracted for %s: Buy %s, Sell %s", targetCurrency, buy, sell)
	return ScrapeResult{BuyRate: buy, SellRate: sell, UsedScraperType: "llm"}, nil
}

// LLMExtractAddress uses Gemini to find a physical address in the HTML text.
func LLMExtractAddress(doc *goquery.Document) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Focus on likely areas for address
	searchArea := doc.Find("footer, .contact, .address, address, #contact").Text()
	if len(searchArea) < 100 {
		searchArea = doc.Find("body").Text()
	}

	cleanText := strings.Join(strings.Fields(searchArea), " ")
	if len(cleanText) > 4000 {
		cleanText = cleanText[:4000]
	}

	prompt := fmt.Sprintf(`Znajdź dokładny adres fizyczny kantoru (ulica, numer, kod pocztowy, miasto) w poniższym tekście. 
Zwróć TYLKO I WYŁĄCZNIE czysty adres jako tekst. Jeśli nie znaleziono adresu, zwróć pusty ciąg.
Tekst: %s`, cleanText)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{{Text: prompt}},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return "", err
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
	}

	return "", nil
}
