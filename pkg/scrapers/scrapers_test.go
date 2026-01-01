package scrapers

import (
	// Standard libraries
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockServer - creates a mock server that returns the given body
func mockServer(body string) *httptest.Server {
	f := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprintln(w, body)
		if err != nil {
			return
		}
	}

	return httptest.NewServer(http.HandlerFunc(f))
}

// TestFetchC1 - tests FetchC1
func TestFetchC1(t *testing.T) {
	html := `
	<html>
		<table class="kursy_walut">
			<tbody>
				<tr>
					<td>ignore</td>
					<td>EUR</td>
					<td>ignore</td>
					<td>4.30</td>
					<td>4.35</td>
				</tr>
				<tr>
					<td>ignore</td>
					<td>USD</td>
					<td>ignore</td>
					<td>3.95</td>
					<td>4.05</td>
				</tr>
			</tbody>
		</table>
	</html>
	`
	server := mockServer(html)
	defer server.Close()

	result, err := FetchC1(server.URL, "EUR")
	if err != nil {
		t.Fatalf("Oczekiwano sukcesu, otrzymano błąd: %v", err)
	}
	if result.BuyRate != "4.30" || result.SellRate != "4.35" {
		t.Errorf("Oczekiwano 4.30/4.35, otrzymano %s/%s", result.BuyRate, result.SellRate)
	}

	_, err = FetchC1(server.URL, "GBP")
	if err == nil {
		t.Error("Oczekiwano błędu dla nieistniejącej waluty, ale go nie otrzymano")
	}
}

// TestFetchC2 - tests FetchC2
func TestFetchC2(t *testing.T) {
	html := `
	<html>
		<table id="AutoNumber2">
			<tbody>
				<tr><td>Nagłówek</td></tr>
				<tr>
					<td>ignore</td>
					<td><b>Euro EUR</b></td>
					<td><b>4.40</b></td>
					<td><b>4.50</b></td>
				</tr>
			</tbody>
		</table>
	</html>
	`
	server := mockServer(html)
	defer server.Close()

	result, err := FetchC2(server.URL, "EUR")
	if err != nil {
		t.Fatalf("FetchC2 failed: %v", err)
	}
	if result.BuyRate != "4.40" || result.SellRate != "4.50" {
		t.Errorf("Błędne parsowanie C2. Oczekiwano 4.40/4.50, jest %s/%s", result.BuyRate, result.SellRate)
	}
}

// TestFetchC3 - tests FetchC3
func TestFetchC3(t *testing.T) {
	html := `
	<html>
		<table class="mceItemTable">
			<tbody>
				<tr><td>Nagłówek</td></tr>
				<tr>
					<td><span style="FONT-SIZE: medium">USD (Dolar)</span></td>
					<td>ignore</td>
					<td>3.90</td>
					<td>4.00</td>
				</tr>
			</tbody>
		</table>
	</html>
	`
	server := mockServer(html)
	defer server.Close()

	result, err := FetchC3(server.URL, "USD")
	if err != nil {
		t.Fatalf("FetchC3 failed: %v", err)
	}
	if result.BuyRate != "3.90" || result.SellRate != "4.00" {
		t.Errorf("Błędne parsowanie C3. Oczekiwano 3.90/4.00, jest %s/%s", result.BuyRate, result.SellRate)
	}
}
