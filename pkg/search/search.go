package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/Niutaq/Gix/pkg/types"
	"github.com/elastic/go-elasticsearch/v8"
)

// CantorRecord represents a cantor indexed in Elasticsearch.
type CantorRecord struct {
	ID          int            `json:"id"`
	DisplayName string         `json:"display_name"`
	Name        string         `json:"name"`
	Location    types.GeoPoint `json:"location"`
}

// SearchEngine handles interactions with Elasticsearch.
type SearchEngine struct {
	client *elasticsearch.Client
}

// NewSearchEngine initializes a new Elasticsearch client.
func NewSearchEngine(address string) (*SearchEngine, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{address},
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &SearchEngine{client: client}, nil
}

// CreateIndices ensures that the necessary indices exist in Elasticsearch.
func (se *SearchEngine) CreateIndices() error {
	indices := []string{"cantors", "cities"}
	for _, idx := range indices {
		res, err := se.client.Indices.Exists([]string{idx})
		if err != nil {
			return err
		}
		if res.StatusCode == 404 {
			_, err := se.client.Indices.Create(idx)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SeedInitialData populates the cities index with some default values.
func (se *SearchEngine) SeedInitialData() error {
	cities := []types.CityRecord{
		{Name: "Warsaw", Province: "Masovian", Location: types.GeoPoint{Lat: 52.2297, Lon: 21.0122}},
		{Name: "Krakow", Province: "Lesser Poland", Location: types.GeoPoint{Lat: 50.0647, Lon: 19.9450}},
		{Name: "Wroclaw", Province: "Lower Silesian", Location: types.GeoPoint{Lat: 51.1079, Lon: 17.0385}},
		{Name: "Poznan", Province: "Greater Poland", Location: types.GeoPoint{Lat: 52.4064, Lon: 16.9252}},
		{Name: "Gdansk", Province: "Pomeranian", Location: types.GeoPoint{Lat: 54.3520, Lon: 18.6466}},
	}

	for _, c := range cities {
		if err := se.IndexCity(c); err != nil {
			return err
		}
	}
	return nil
}

// IndexCantor indexes a cantor record into Elasticsearch.
func (se *SearchEngine) IndexCantor(c CantorRecord) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}

	res, err := se.client.Index(
		"cantors",
		&buf,
		se.client.Index.WithContext(context.Background()),
		se.client.Index.WithDocumentID(fmt.Sprintf("%d", c.ID)),
	)
	if err != nil {
		return err
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	return nil
}

// IndexCity indexes a city record into Elasticsearch.
func (se *SearchEngine) IndexCity(c types.CityRecord) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}

	res, err := se.client.Index(
		"cities",
		&buf,
		se.client.Index.WithContext(context.Background()),
	)
	if err != nil {
		return err
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	return nil
}

// SearchCantors searches for cantors by name or address.
func (se *SearchEngine) SearchCantors(query string) ([]CantorRecord, error) {
	var buf bytes.Buffer
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"display_name", "name"},
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, err
	}

	res, err := se.client.Search(
		se.client.Search.WithContext(context.Background()),
		se.client.Search.WithIndex("cantors"),
		se.client.Search.WithBody(&buf),
		se.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, err
	}
	if res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.Status())
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return nil, err
	}

	var results []CantorRecord
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		source := hit.(map[string]interface{})["_source"]
		var c CantorRecord
		b, _ := json.Marshal(source)
		_ = json.Unmarshal(b, &c)
		results = append(results, c)
	}

	return results, nil
}

// SearchCantorsNearby searches for cantors within a certain distance from a location.
func (se *SearchEngine) SearchCantorsNearby(lat, lon float64, distanceKM float64) ([]CantorRecord, error) {
	var buf bytes.Buffer
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"geo_distance": map[string]interface{}{
				"distance": fmt.Sprintf("%.1fkm", distanceKM),
				"location": map[string]interface{}{
					"lat": lat,
					"lon": lon,
				},
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, err
	}

	res, err := se.client.Search(
		se.client.Search.WithContext(context.Background()),
		se.client.Search.WithIndex("cantors"),
		se.client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}
	if res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.Status())
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return nil, err
	}

	var results []CantorRecord
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		source := hit.(map[string]interface{})["_source"]
		var c CantorRecord
		b, _ := json.Marshal(source)
		_ = json.Unmarshal(b, &c)
		results = append(results, c)
	}

	return results, nil
}

// SearchCity searches for cities by name.
func (se *SearchEngine) SearchCity(query string) ([]types.CityRecord, error) {
	var buf bytes.Buffer
	searchQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"name": query,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, err
	}

	res, err := se.client.Search(
		se.client.Search.WithContext(context.Background()),
		se.client.Search.WithIndex("cities"),
		se.client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}
	if res.Body != nil {
		defer func() { _ = res.Body.Close() }()
	}

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.Status())
	}

	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return nil, err
	}

	var results []types.CityRecord
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		source := hit.(map[string]interface{})["_source"]
		var c types.CityRecord
		b, _ := json.Marshal(source)
		_ = json.Unmarshal(b, &c)
		results = append(results, c)
	}

	return results, nil
}
