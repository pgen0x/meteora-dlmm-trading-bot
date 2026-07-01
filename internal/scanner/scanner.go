package scanner

import (
	"context"
	"log"
	"time"

	"github.com/meteora-dlmm-signal/internal/config"
	"github.com/meteora-dlmm-signal/internal/meteora"
	"github.com/meteora-dlmm-signal/internal/store"
	"github.com/meteora-dlmm-signal/internal/webhook"
)

// Scanner polls the Meteora discovery API for each enabled mode, screens pools,
// dedups, and forwards newly-qualifying pools to the Hermes webhook.
type Scanner struct {
	cfg  config.Config
	seen *store.Seen
	fwd  *webhook.Forwarder
}

// New wires a Scanner from config.
func New(cfg config.Config) *Scanner {
	return &Scanner{
		cfg:  cfg,
		seen: store.New(cfg.RedisAddr, cfg.RedisSeenKey, cfg.SeenTTL),
		fwd:  webhook.New(cfg.WebhookURL, cfg.WebhookSecret),
	}
}

// modes returns the enabled screening modes.
func (s *Scanner) modes() []meteora.ModeParams {
	var out []meteora.ModeParams
	if s.cfg.EnableCasual {
		out = append(out, meteora.Casual)
	}
	if s.cfg.EnableMultiday {
		out = append(out, meteora.Multiday)
	}
	return out
}

// Run blocks, polling on PollInterval until ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
	log.Printf("scanner: started (interval=%v, casual=%v, multiday=%v, momentum=%v)",
		s.cfg.PollInterval, s.cfg.EnableCasual, s.cfg.EnableMultiday, s.cfg.EnableMomentumGate)

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	s.pollAll(ctx)
	for {
		select {
		case <-ticker.C:
			s.pollAll(ctx)
		case <-ctx.Done():
			log.Println("scanner: stopped")
			return
		}
	}
}

func (s *Scanner) pollAll(ctx context.Context) {
	for _, mp := range s.modes() {
		s.pollMode(ctx, mp)
	}
}

func (s *Scanner) pollMode(ctx context.Context, mp meteora.ModeParams) {
	pools, err := meteora.FetchTopPools(s.cfg.DiscoverURL, mp)
	if err != nil {
		log.Printf("scanner[%s]: fetch error: %v", mp.Mode, err)
		return
	}

	// Per-poll tally so a quiet cycle logs "scanned N, 0 passed" instead of
	// nothing — distinguishes "working, nothing qualified" from "API empty".
	var screened, deduped, momRejected, sent int
	for _, p := range pools {
		cand, reason := meteora.Screen(p, mp)
		if reason != "" {
			continue // failed a gate; per-pool detail too noisy, counted in summary
		}
		screened++

		// Dedup BEFORE the momentum fetch so we don't hit DexScreener for
		// pools we've already emitted this window.
		poolKey := mp.Mode + ":" + cand.Pool
		fresh, err := s.seen.MarkIfNew(ctx, poolKey)
		if err != nil {
			log.Printf("scanner[%s]: seen store error: %v", mp.Mode, err)
			continue
		}
		if !fresh {
			deduped++
			continue
		}

		// Momentum / downtrend gate (best-effort, fail-open).
		if s.cfg.EnableMomentumGate {
			if m, ok := meteora.GetMomentum(cand.BaseMint); ok {
				if r := meteora.MomentumReject(m); r != "" {
					momRejected++
					log.Printf("scanner[%s]: %s (%s) rejected on momentum: %s", mp.Mode, cand.BaseSymbol, cand.Pool[:8], r)
					continue
				}
			}
		}

		if err := s.fwd.Send("meteora_pool_discovery", cand, time.Now().Unix()); err != nil {
			// Delivery failed — unmark so this pool retries on the next poll
			// instead of being silently dropped for the whole SEEN_TTL window.
			s.seen.Unmark(ctx, poolKey)
			log.Printf("scanner[%s]: webhook error for %s (will retry): %v", mp.Mode, cand.BaseSymbol, err)
			continue
		}
		sent++
		log.Printf("scanner[%s]: SIGNAL sent %s pool=%s TVL=$%.0f fee/TVL=%.2f%% score=%.1f",
			mp.Mode, cand.BaseSymbol, cand.Pool[:8], cand.TVL, cand.FeeTVLRatio, cand.Score)
	}

	log.Printf("scanner[%s]: cycle done — fetched=%d passed_screen=%d deduped=%d mom_rejected=%d sent=%d",
		mp.Mode, len(pools), screened, deduped, momRejected, sent)
}
