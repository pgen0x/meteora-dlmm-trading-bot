package robinhood

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// gatewayURL is Uniswap's own interface GraphQL gateway — the backend behind
// app.uniswap.org's Explore ▸ Pools table. Keyless and unauthenticated
// (verified 2026-07-14), and it speaks `chain: ROBINHOOD` natively.
//
// It exists here because GeckoTerminal's new_pools feed CANNOT serve the
// mature thesis: it is a launch feed, and a pool that has been printing fees
// for three days scrolled off it long ago. Conversely the gateway cannot serve
// the Fresh thesis — see topPoolsFirst.
const gatewayURL = "https://interface.gateway.uniswap.org/v1/graphql"

// topPoolsFirst caps the gateway page. The number is deliberately larger than
// the venue needs: `topV3Pools(chain: ROBINHOOD)` returns the ENTIRE indexed
// universe — 74 pools on 2026-07-14, sorted by TVL descending, bottoming out
// around $12.6k TVL, with ZERO pools younger than 24h. The gateway is a TVL
// leaderboard with an indexing lag, not a discovery feed. That is exactly the
// half of the venue GeckoTerminal is blind to, and exactly why Fresh must keep
// using new_pools.
const topPoolsFirst = 100

// maxEnrich bounds the GeckoTerminal enrichment call. GT's /pools/multi/ takes
// up to 30 comma-separated addresses in ONE request, which is the whole point
// of the prefilter: the keyless GT tier throttles at roughly 4 req/min (see
// discover.go), so a mature cycle must cost exactly one GT call, never a
// per-pool fan-out.
const maxEnrich = 30

// feeTierDenom converts a Uniswap v3 feeTier (hundredths of a bip: 500, 3000,
// 10000) to a percent (0.05, 0.3, 1.0).
const feeTierDenom = 10000.0

var gatewayClient = &http.Client{Timeout: 15 * time.Second}

// topPoolsQuery asks for everything the gateway can give us about a pool.
// Notably absent, and the reason enrichFromGT exists: h1 volume, the
// buys/sells/buyers/sellers breakdown, price-change windows, and FDV.
// `cumulativeVolume` rejects an HOUR duration on this schema — DAY is the
// finest window available.
const topPoolsQuery = `query TopV3Pools($chain: Chain!, $first: Int!) {
  topV3Pools(chain: $chain, first: $first) {
    address
    createdAtTimestamp
    feeTier
    totalLiquidity { value }
    cumulativeVolume(duration: DAY) { value }
    token0 { address symbol decimals }
    token1 { address symbol decimals }
  }
}`

// uniToken is one side of a gateway pool.
type uniToken struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// uniAmount is the gateway's {value} money wrapper. Nil for pools it has not
// finished indexing — read it through val().
type uniAmount struct {
	Value float64 `json:"value"`
}

// uniPool is one entry of topV3Pools.
type uniPool struct {
	Address            string     `json:"address"`
	CreatedAtTimestamp int64      `json:"createdAtTimestamp"`
	FeeTier            float64    `json:"feeTier"`
	TotalLiquidity     *uniAmount `json:"totalLiquidity"`
	CumulativeVolume   *uniAmount `json:"cumulativeVolume"`
	Token0             uniToken   `json:"token0"`
	Token1             uniToken   `json:"token1"`
}

// uniResponse is the GraphQL envelope. Errors are per-field and partial: the
// gateway returns data alongside errors for individual pools, so a non-empty
// Errors list is logged, not fatal.
type uniResponse struct {
	Data struct {
		TopV3Pools []uniPool `json:"topV3Pools"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (a *uniAmount) val() float64 {
	if a == nil {
		return 0
	}
	return a.Value
}

// FetchMaturePools discovers established fee-printing pools: pools too old for
// the Fresh window that are still generating outsized fee/TVL. It is the
// mature-mode analog of FetchNewPools and returns Pools ready for Screen.
//
// Two hops, two HTTP calls per cycle, on purpose:
//
//  1. Uniswap's gateway returns the venue's whole v3 universe with TVL, fee
//     tier, age and 24h volume — but no h1 flow and no FDV.
//  2. A LOCAL prefilter (no I/O) cuts that to the pools that could plausibly
//     pass, then ONE GeckoTerminal /pools/multi/ call fills in the h1
//     buys/sells/buyers, price-change windows and FDV that Screen gates on.
//
// The prefilter is what keeps step 2 to a single request; without it this would
// fan out per-pool and burn the GT budget the Fresh mode depends on.
func FetchMaturePools(mp ModeParams, now time.Time) ([]Pool, error) {
	raw, err := fetchTopV3Pools()
	if err != nil {
		return nil, err
	}

	shortlist := make([]Pool, 0, maxEnrich)
	for _, up := range raw {
		p, ok := toPool(up)
		if !ok {
			continue
		}
		if !prefilter(p, mp, now) {
			continue
		}
		shortlist = append(shortlist, p)
		if len(shortlist) >= maxEnrich {
			// The gateway sorts by TVL descending, so a truncated shortlist
			// drops the SMALLEST survivors — the ones furthest from the
			// liquidity we want anyway.
			break
		}
	}
	if len(shortlist) == 0 {
		return nil, nil
	}

	if err := enrichFromGT(shortlist); err != nil {
		// Hard failure, unlike discover.go's tolerated partial pages: every
		// remaining Screen gate (txns, buyers, no-sells honeypot shape,
		// momentum, FDV) reads a field that ONLY the enrichment supplies.
		// Passing un-enriched pools on would silently gate on zeros.
		return nil, fmt.Errorf("mature enrich: %w", err)
	}
	return shortlist, nil
}

// fetchTopV3Pools runs the gateway query and returns the raw pool list.
func fetchTopV3Pools() ([]uniPool, error) {
	body, err := json.Marshal(map[string]any{
		"operationName": "TopV3Pools",
		"query":         topPoolsQuery,
		"variables": map[string]any{
			"chain": strings.ToUpper(Chain),
			"first": topPoolsFirst,
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, gatewayURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The gateway wants a browser-shaped Origin; without it the edge can 403.
	req.Header.Set("Origin", "https://app.uniswap.org")

	resp, err := gatewayClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("uniswap gateway status %d", resp.StatusCode)
	}

	var ur uniResponse
	if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
		return nil, fmt.Errorf("uniswap gateway decode: %w", err)
	}
	if len(ur.Errors) > 0 {
		// Partial, not fatal — the gateway returns per-pool errors alongside
		// usable data. Only an empty result is worth failing on.
		log.Printf("robinhood: uniswap gateway returned %d field error(s), first: %s",
			len(ur.Errors), ur.Errors[0].Message)
	}
	if len(ur.Data.TopV3Pools) == 0 {
		return nil, fmt.Errorf("uniswap gateway returned no pools")
	}
	return ur.Data.TopV3Pools, nil
}

// toPool maps a gateway pool onto the venue's Pool, orienting it so the base
// side is the token and the quote side is WETH. The gateway orders token0 /
// token1 by address, not by role, so WETH lands on either side — Screen's WETH
// check would reject half the universe if we assumed token1.
//
// h1 flow, price changes and FDV stay zero here; enrichFromGT fills them.
func toPool(up uniPool) (Pool, bool) {
	if up.Address == "" || up.CreatedAtTimestamp == 0 {
		return Pool{}, false
	}

	base, quote := up.Token0, up.Token1
	if strings.EqualFold(up.Token0.Address, WETH) {
		base, quote = up.Token1, up.Token0
	}

	feePct := up.FeeTier / feeTierDenom
	return Pool{
		Address:   up.Address,
		Name:      fmt.Sprintf("%s / %s %g%%", base.Symbol, quote.Symbol, feePct),
		Dex:       "uniswap-v3",
		CreatedAt: time.Unix(up.CreatedAtTimestamp, 0).UTC(),

		BaseAddress:  base.Address,
		BaseSymbol:   base.Symbol,
		BaseDecimals: base.Decimals,
		QuoteAddress: quote.Address,
		QuoteSymbol:  quote.Symbol,

		FeePct:       feePct,
		ReserveUSD:   up.TotalLiquidity.val(),
		VolumeH24USD: up.CumulativeVolume.val(),
	}, true
}

// prefilter applies only the gates the gateway alone can answer, to shrink the
// enrichment batch. It is deliberately a SUBSET of Screen — never a substitute:
// anything that survives here still faces the full Screen once enriched, so a
// prefilter bug can only cost recall, never let a bad pool through.
func prefilter(p Pool, mp ModeParams, now time.Time) bool {
	if !strings.EqualFold(p.QuoteAddress, WETH) {
		return false
	}
	age := now.Sub(p.CreatedAt)
	if age < mp.MinAge {
		return false
	}
	if mp.MaxAge > 0 && age > mp.MaxAge {
		return false
	}
	if p.ReserveUSD < mp.MinReserveUSD {
		return false
	}
	if mp.MaxReserveUSD > 0 && p.ReserveUSD > mp.MaxReserveUSD {
		return false
	}
	if p.FeePct < mp.MinFeePct {
		return false
	}
	// Fee pace from the 24h window — the same number Screen recomputes for a
	// FeePaceH24 mode, so this rejects nothing Screen would have kept.
	if p.ReserveUSD > 0 {
		feeTVLDay := (p.VolumeH24USD * p.FeePct / 100) / p.ReserveUSD * 100
		if feeTVLDay < mp.MinFeeTVLDay {
			return false
		}
	}
	return true
}

// enrichFromGT fills the fields the Uniswap gateway does not expose — h1
// volume, the h1/h24 buys/sells/buyers/sellers breakdown, the price-change
// windows and FDV/market cap — from ONE GeckoTerminal /pools/multi/ call.
// Pools mutate in place, matched by address (GT lowercases; the gateway
// checksums — compare case-insensitively).
//
// A pool GT does not know about keeps its zero values and will fail Screen's
// txn/buyer gates, which is the correct outcome: no flow data, no trade.
func enrichFromGT(pools []Pool) error {
	addrs := make([]string, len(pools))
	for i, p := range pools {
		addrs[i] = p.Address
	}
	url := fmt.Sprintf("https://api.geckoterminal.com/api/v2/networks/%s/pools/multi/%s",
		Chain, strings.Join(addrs, ","))

	gr, err := fetchPage(url)
	if err != nil {
		return err
	}

	byAddr := make(map[string]gtPoolAttrs, len(gr.Data))
	for _, gp := range gr.Data {
		byAddr[strings.ToLower(gp.Attrs.Address)] = gp.Attrs
	}

	for i := range pools {
		a, ok := byAddr[strings.ToLower(pools[i].Address)]
		if !ok {
			continue
		}
		pools[i].FdvUSD = pfloat(a.FdvUSD)
		pools[i].McapUSD = pfloat(a.MarketCapUSD)
		pools[i].VolumeH1USD = pfloat(a.VolumeUSD.H1)
		pools[i].TxH1 = a.Transactions.H1
		pools[i].TxH24 = a.Transactions.H24
		pools[i].ChangeM5Pct = pfloat(a.PriceChangePct.M5)
		pools[i].ChangeH1Pct = pfloat(a.PriceChangePct.H1)
		pools[i].ChangeH6Pct = pfloat(a.PriceChangePct.H6)
		pools[i].ChangeH24Pct = pfloat(a.PriceChangePct.H24)
		// Prefer GT's reserve and 24h volume over the gateway's: Screen's
		// fee-pace and liquidity gates then read the SAME source the Fresh mode
		// gates on, so a threshold means one thing across both modes.
		if v := pfloat(a.ReserveUSD); v > 0 {
			pools[i].ReserveUSD = v
		}
		if v := pfloat(a.VolumeUSD.H24); v > 0 {
			pools[i].VolumeH24USD = v
		}
		if a.Name != "" {
			pools[i].Name = a.Name
		}
	}
	return nil
}
