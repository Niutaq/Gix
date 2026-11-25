package utilities

import (
	"math"
	"testing"
)

func TestIsExcludedDiscoveryURL(t *testing.T) {
	// Aggregator kantor.pl and kantory.pl must be excluded, but independent *kantor.pl domains must NOT be excluded.
	if !isExcludedDiscoveryURL("https://kantor.pl/kursy") {
		t.Fatalf("expected kantor.pl to be excluded")
	}
	if !isExcludedDiscoveryURL("https://www.kantory.pl/warszawa") {
		t.Fatalf("expected www.kantory.pl to be excluded")
	}
	if isExcludedDiscoveryURL("https://superkantor.pl/kursy-walut") {
		t.Fatalf("expected superkantor.pl NOT to be excluded by kantor.pl substring")
	}
	if isExcludedDiscoveryURL("https://tavex.pl/kursy-walut/") {
		t.Fatalf("expected tavex.pl NOT to be excluded")
	}
	if isExcludedDiscoveryURL("https://danexchange.pl/kantor-warszawa-kursy-walut/") {
		t.Fatalf("expected danexchange.pl NOT to be excluded")
	}
}

func TestDDGSearchTermAndPinCantorDoNotCorruptTags(t *testing.T) {
	state := &AppState{Cantors: make(map[string]*CantorInfo)}
	s := &citySearch{city: "Stalowa Wola", state: state}

	poi := osmCantorPOI{
		Lat: 50.5826,
		Lon: 22.0536,
		Tags: map[string]string{
			"name":          "Kantor",
			osmTagStreet:    "Aleja Jana Pawła II 25",
			"addr:city":     "Stalowa Wola",
			"amenity":       "bureau_de_change",
		},
	}

	s.pinCantor(&poi)
	if poi.Tags["name"] != "Kantor" {
		t.Fatalf("pinCantor mutated raw OSM name tag to %q", poi.Tags["name"])
	}

	target := newDiscoveryTarget(poi, "Stalowa Wola")
	term := ddgSearchTerm(target.name, target.addr, "Stalowa Wola")
	want := "Kantor Aleja Jana Pawła II 25 Stalowa Wola"
	if term != want {
		t.Fatalf("ddgSearchTerm() = %q, want %q", term, want)
	}
}

func TestDecodeDDGHref(t *testing.T) {
	ddg := "//duckduckgo.com/l/?uddg=https%3A%2F%2Fdanexchange.pl%2Fkantor-warszawa%2F&rut=abc"
	if got := decodeDDGHref(ddg); got != "https://danexchange.pl/kantor-warszawa/" {
		t.Fatalf("decodeDDGHref(ddg) = %q", got)
	}

	yahoo := "https://r.search.yahoo.com/_ylt=Awr/RV=2/RE=123/RO=10/RU=https%3a%2f%2fkantor1913.pl%2fkursy-warszawa/RK=2/RS=xyz-"
	if got := decodeDDGHref(yahoo); got != "https://kantor1913.pl/kursy-warszawa" {
		t.Fatalf("decodeDDGHref(yahoo) = %q", got)
	}
}

func TestCityDistanceSurvivesMapFocusConsume(t *testing.T) {
	state := &AppState{}
	// User is in Stalowa Wola area (50.627, 22.176).
	state.UI.UserLocation.Latitude = 50.6270
	state.UI.UserLocation.Longitude = 22.1760
	state.UI.UserLocation.Active = false

	state.UI.MapFocus.CityName = "Stalowa Wola"
	state.UI.MapFocus.Latitude = 50.5826
	state.UI.MapFocus.Longitude = 22.0536
	state.UI.MapFocus.CityLat = 50.5826
	state.UI.MapFocus.CityLon = 22.0536

	// Simulate LayoutMap consuming one-shot MapFocus.Latitude/Longitude on first frame.
	state.UI.MapFocus.Latitude = 0
	state.UI.MapFocus.Longitude = 0

	cantor := &CantorInfo{
		DisplayName: "Kantor · Aleja Jana Pawła II 25",
		Latitude:    50.5790,
		Longitude:   22.0580,
	}

	dist, ok := cantorDistanceKm(state, cantor)
	if !ok {
		t.Fatalf("expected distance to be available")
	}
	if dist > 12.0 || math.IsNaN(dist) {
		t.Fatalf("expected local Stalowa Wola distance (<12 km), got %.2f km", dist)
	}
	if !isWithinDistance(state, cantor) {
		t.Fatalf("expected Stalowa Wola cantor to remain within city radius after LayoutMap consumed MapFocus")
	}

	// Now simulate searching "Warszawa" while user is still in Stalowa Wola:
	// Warsaw cantor must pass the Warsaw city filter, AND show ~180-220 km distance from user (not ~5 km from Warsaw center).
	state.UI.MapFocus.CityName = "Warszawa"
	state.UI.MapFocus.CityLat = 52.2297
	state.UI.MapFocus.CityLon = 21.0122

	warsawCantor := &CantorInfo{
		DisplayName: "DealFin",
		Latitude:    52.2309,
		Longitude:   21.0181,
	}
	if !isWithinDistance(state, warsawCantor) {
		t.Fatalf("expected Warsaw cantor to be within searched city radius of Warszawa")
	}
	wDist, ok := cantorDistanceKm(state, warsawCantor)
	if !ok || wDist < 170.0 || wDist > 230.0 {
		t.Fatalf("expected Warsaw cantor distance from Stalowa Wola user to be ~195 km, got %.2f km", wDist)
	}
}

func TestPruneUnresolvedOSMPins(t *testing.T) {
	state := &AppState{
		Vault: &CantorVault{
			Rates: make(map[string]map[string]*CantorEntry),
		},
		Cantors: map[string]*CantorInfo{
			"osm_50.5826_22.0536": {
				ID:          0,
				DisplayName: "Kantor · Aleja Jana Pawła II 25",
				Latitude:    50.5826,
				Longitude:   22.0536,
				Strategy:    "OSM",
			},
			"230": {
				ID:          230,
				DisplayName: "Kantor Stalowa Wola",
				Latitude:    50.5800,
				Longitude:   22.0550,
				Strategy:    "HEURISTIC",
			},
		},
	}
	state.UI.MapFocus.CityName = "Stalowa Wola"
	state.UI.MapFocus.CityLat = 50.5826
	state.UI.MapFocus.CityLon = 22.0536

	s := newCitySearch("Stalowa Wola", state, AppConfig{}, nil)
	s.pruneUnresolvedOSMPins()

	if _, exists := state.Cantors["osm_50.5826_22.0536"]; exists {
		t.Fatalf("expected unresolved OSM pin to be pruned when a real cantor exists in the city")
	}
	if _, exists := state.Cantors["230"]; !exists {
		t.Fatalf("expected real heuristic cantor 230 to remain")
	}
}

func TestBuildNominatimCandidateUnnamedHouseAndRoad(t *testing.T) {
	var r1 nominatimPlace
	r1.Name = "1"
	r1.DisplayName = "1, Generała Leopolda Okulickiego, Stalowa Wola"
	r1.Address.Road = "Generała Leopolda Okulickiego"
	r1.Address.HouseNumber = "1"
	r1.Address.City = "Stalowa Wola"
	r1.Type = "bureau_de_change"

	c1 := buildNominatimCandidate(r1, 50.58, 22.05)
	if got := osmDisplayName(c1, "Stalowa Wola"); got != "Kantor · Generała Leopolda Okulickiego 1" {
		t.Fatalf("osmDisplayName(house number) = %q, want %q", got, "Kantor · Generała Leopolda Okulickiego 1")
	}

	var r2 nominatimPlace
	r2.Name = "Juliusza Słowackiego"
	r2.DisplayName = "Juliusza Słowackiego, Stalowa Wola"
	r2.Address.Road = "Juliusza Słowackiego"
	r2.Address.City = "Stalowa Wola"
	r2.Type = "bureau_de_change"

	c2 := buildNominatimCandidate(r2, 50.58, 22.05)
	if got := osmDisplayName(c2, "Stalowa Wola"); got != "Kantor · Juliusza Słowackiego" {
		t.Fatalf("osmDisplayName(road name) = %q, want %q", got, "Kantor · Juliusza Słowackiego")
	}
}


