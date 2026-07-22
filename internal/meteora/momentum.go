package meteora

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var momentumClient = &http.Client{Timeout: 10 * time.Second}

// dexResponse partially models the DexScreener token endpoint.
type dexResponse struct {
	Pairs []struct {
		PriceChange struct {
			M5  float64 `json:"m5"`
			H1  float64 `json:"h1"`
			H6  float64 `json:"h6"`
			H24 float64 `json:"h24"`
		} `json:"priceChange"`
	} `json:"pairs"`
}

// Momentum holds recent price-change percentages for a base mint.
type Momentum struct {
	M5, H1, H6, H24 float64
}

// GetMomentum fetches DexScreener price momentum for a base mint.
// Best-effort: on any error it returns ok=false and the caller fails open.
func GetMomentum(baseMint string) (Momentum, bool) {
	url := fmt.Sprintf("https://api.dexscreener.com/latest/dex/tokens/%s", baseMint)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Momentum{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := momentumClient.Do(req)
	if err != nil {
		return Momentum{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return Momentum{}, false
	}
	var dr dexResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil || len(dr.Pairs) == 0 {
		return Momentum{}, false
	}
	pc := dr.Pairs[0].PriceChange
	return Momentum{M5: pc.M5, H1: pc.H1, H6: pc.H6, H24: pc.H24}, true
}

// MomentumReject applies the pipeline's momentum + downtrend gates.
// Returns a non-empty reason when the pool should be rejected.
func MomentumReject(m Momentum) string {
	// Strategy overhaul 2026-07-20: entry failures were mostly downtrend catches.
	// Tighten short-horizon gates so we stop entering tokens already bleeding.
	if m.M5 <= -3 {
		return fmt.Sprintf("5m %.1f%% <= -3%% (dumping)", m.M5)
	}
	if m.H1 <= -7 {
		return fmt.Sprintf("1h %.1f%% <= -7%% (dumping)", m.H1)
	}
	// Sustained downtrend gate.
	if m.H6 <= -10 {
		return fmt.Sprintf("6h %.1f%% <= -10%% (downtrend)", m.H6)
	}
	if m.H24 <= -20 {
		return fmt.Sprintf("24h %.1f%% <= -20%% (downtrend)", m.H24)
	}
	return ""
}
