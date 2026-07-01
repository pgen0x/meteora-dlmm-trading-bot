package meteora

import "fmt"

// ModeParams are the per-mode screening thresholds, ported verbatim from
// dlmm_pipeline.py MODE_DEFAULTS / SOUL.md section 9.
type ModeParams struct {
	Mode         string
	Timeframe    string  // discovery timeframe to query
	MinTVL       float64 // MIN_TVL_USD
	MinFeeTVL    float64 // MIN_FEE_TVL_24H (percent)
	MinMcap      float64 // MIN_MCAP_USD
	MinHolders   int     // MIN_HOLDERS
	MinDailyFee  float64 // absolute daily-fee floor (USD)
	MinOrganic   float64 // shared MIN_ORGANIC_SCORE
}

// Casual and Multiday mirror the two isolated budgets in the pipeline.
var (
	Casual = ModeParams{
		Mode: "casual", Timeframe: "30m",
		MinTVL: 5000, MinFeeTVL: 0.3, MinMcap: 250000, MinHolders: 500,
		MinDailyFee: 20, MinOrganic: 60,
	}
	Multiday = ModeParams{
		Mode: "multiday", Timeframe: "24h",
		MinTVL: 50000, MinFeeTVL: 1.0, MinMcap: 1000000, MinHolders: 1000,
		MinDailyFee: 150, MinOrganic: 60,
	}
)

// SkipReason is returned (non-empty) when a pool fails a gate, for logging.
// A returned Candidate is only valid when reason == "".
func Screen(p Pool, mp ModeParams) (*Candidate, string) {
	// Orientation: exactly one side must be SOL.
	var base, quote Token
	var solIsX bool
	switch {
	case p.TokenY.Address == SolMint:
		base, quote, solIsX = p.TokenX, p.TokenY, false
	case p.TokenX.Address == SolMint:
		base, quote, solIsX = p.TokenY, p.TokenX, true
	default:
		return nil, "non-SOL pool"
	}
	_ = quote

	if p.TVL < mp.MinTVL {
		return nil, fmt.Sprintf("TVL $%.0f < $%.0f", p.TVL, mp.MinTVL)
	}
	if p.FeeTVLRatio < mp.MinFeeTVL {
		return nil, fmt.Sprintf("fee/TVL %.2f%% < %.2f%%", p.FeeTVLRatio, mp.MinFeeTVL)
	}
	dailyFeeUSD := p.TVL * p.FeeTVLRatio / 100.0
	if dailyFeeUSD < mp.MinDailyFee {
		return nil, fmt.Sprintf("daily fees $%.0f < $%.0f", dailyFeeUSD, mp.MinDailyFee)
	}
	if p.Volatility <= 0 {
		return nil, "volatility <= 0"
	}
	if p.Volatility > 15 {
		return nil, fmt.Sprintf("volatility %.2f > 15 (IL risk)", p.Volatility)
	}
	if base.OrganicScore < mp.MinOrganic {
		return nil, fmt.Sprintf("organic %.0f < %.0f", base.OrganicScore, mp.MinOrganic)
	}
	if base.MarketCap < mp.MinMcap {
		return nil, fmt.Sprintf("mcap $%.0f < $%.0f", base.MarketCap, mp.MinMcap)
	}
	if base.Holders < mp.MinHolders {
		return nil, fmt.Sprintf("holders %d < %d", base.Holders, mp.MinHolders)
	}
	if p.FeeTVLRatioChangePct < -40.0 {
		return nil, fmt.Sprintf("yield declining %.0f%%", p.FeeTVLRatioChangePct)
	}

	// Supply-concentration safety gates.
	if base.TopHoldersPct > 60.0 {
		return nil, fmt.Sprintf("top10 own %.1f%% (>60%%)", base.TopHoldersPct)
	}
	if base.DevBalancePct > 20.0 {
		return nil, fmt.Sprintf("dev owns %.1f%% (>20%%)", base.DevBalancePct)
	}

	// Authority gates.
	if base.HasFreezeAuth {
		return nil, "freeze authority enabled"
	}
	if base.HasMintAuth {
		return nil, "mint authority enabled"
	}

	// Verified + Jupiter shield, fail-open when absent.
	if !boolOr(base.Verified, true) {
		return nil, "not verified"
	}
	jupShield := base.JupShieldVerified
	if jupShield == nil {
		jupShield = base.JupShield
	}
	if !boolOr(jupShield, true) {
		return nil, "failed Jupiter shield"
	}

	// Critical / warning severity gate.
	for _, w := range base.Warnings {
		if w.Severity == "critical" || w.Severity == "warning" {
			return nil, "warning: " + w.Message
		}
	}

	// Score: organic + active-TVL yield - volatility (IL risk). Smart-wallet
	// term from the Python pipeline is omitted (needs on-chain lookups the agent
	// does at review time). Accelerating-yield bonus preserved.
	score := base.OrganicScore + (p.FeeActiveTVLRatio * 10) - (p.Volatility * 1.5)
	if p.FeeTVLRatioChangePct > 30 {
		score += 10
	}

	return &Candidate{
		Mode:                 mp.Mode,
		Timeframe:            mp.Timeframe,
		Pool:                 p.PoolAddress,
		Name:                 p.Name,
		BaseMint:             base.Address,
		BaseSymbol:           base.Symbol,
		SolIsX:               solIsX,
		TVL:                  p.TVL,
		FeeTVLRatio:          p.FeeTVLRatio,
		FeeActiveTVLRatio:    p.FeeActiveTVLRatio,
		FeeTVLRatioChangePct: p.FeeTVLRatioChangePct,
		DailyFeeUSD:          dailyFeeUSD,
		Volatility:           p.Volatility,
		BinStep:              p.DlmmParams.BinStep,
		OrganicScore:         base.OrganicScore,
		Mcap:                 base.MarketCap,
		Holders:              base.Holders,
		TopHoldersPct:        base.TopHoldersPct,
		DevBalancePct:        base.DevBalancePct,
		Score:                score,
	}, ""
}
