package meteora

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// discoverClient timeout mirrors the Python urllib timeout=15.
var discoverClient = &http.Client{Timeout: 15 * time.Second}

// FetchTopPools pulls the trending pools for a timeframe ("30m" or "24h").
// Mirrors dlmm_pipeline.fetch_top_pools.
func FetchTopPools(baseURL, timeframe string) ([]Pool, error) {
	url := fmt.Sprintf("%s?page_size=50&timeframe=%s&category=trending", baseURL, timeframe)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := discoverClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("discover HTTP %d: %s", resp.StatusCode, string(b))
	}

	var out discoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode discover: %w", err)
	}
	return out.Data, nil
}
