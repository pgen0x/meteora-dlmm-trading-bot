# Webhook Signal Schema

The daemon POSTs one envelope per newly-qualifying pool to your Hermes webhook.

## Transport

- **Method:** `POST`
- **URL:** `HERMES_WEBHOOK_URL` (default `http://127.0.0.1:8646/webhooks/dlmm-signal`)
- **Header:** `X-Webhook-Signature: <hex(HMAC-SHA256(secret, body))>`
  - `secret` = `HERMES_WEBHOOK_SECRET`, and must equal the `secret` in your
    Hermes `webhook_subscriptions.json` entry.
- **Content-Type:** `application/json`

## Envelope

```json
{
  "type": "alert",
  "timestamp": 1782873031,
  "source": "meteora_pool_discovery",
  "payload": {
    "mode": "casual",
    "timeframe": "30m",
    "pool": "sz2UJhf8KWxa115KmwcDuJYnUZx1fxDBetcAxXSboKi",
    "name": "CATWIF-SOL",
    "base_mint": "5pYB12kEhfhSFXJjZ7JtyqDpt6uUqhsF6iu6Ee9spump",
    "base_symbol": "CATWIF",
    "sol_is_x": false,
    "tvl": 105596.0,
    "fee_tvl_ratio": 27.43,
    "fee_active_tvl_ratio": 41.2,
    "fee_tvl_ratio_change_pct": 12.0,
    "daily_fee_usd": 289.0,
    "volatility": 3.4,
    "bin_step": 100,
    "organic_score": 82.0,
    "mcap": 540000.0,
    "holders": 1240,
    "top_holders_pct": 38.0,
    "dev_balance_pct": 2.0,
    "score": 91.3
  }
}
```

## Payload fields

| Field | Meaning |
|-------|---------|
| `mode` | `casual` (30m plays) or `multiday` (24h+ holds) — drives which budget/params the agent uses |
| `timeframe` | discovery timeframe the pool trended on |
| `pool` | Meteora DLMM pool address — pass to `dlmm_pipeline.py --pool` |
| `base_mint` | non-SOL token mint — use for audit / momentum re-checks |
| `sol_is_x` | true if SOL is token_x (deploy orientation) |
| `tvl` / `fee_tvl_ratio` / `fee_active_tvl_ratio` | liquidity + yield metrics |
| `daily_fee_usd` | absolute fees/day (already past the mode floor) |
| `volatility` | 0–15 band (IL risk); >15 already rejected |
| `organic_score` / `mcap` / `holders` | base-token quality |
| `top_holders_pct` / `dev_balance_pct` | supply concentration (already gated) |
| `score` | daemon's ranking score (higher = stronger candidate) |

## Screening already applied (agent can trust these)

Only pools passing **all** of these are emitted:

- SOL-paired; TVL ≥ mode floor; fee/TVL ≥ mode floor; daily fee ≥ mode floor
- `0 < volatility ≤ 15`; organic ≥ 60; mcap ≥ mode floor; holders ≥ mode floor
- fee/TVL change ≥ −40%; top-10 ≤ 60%; dev ≤ 20%
- no freeze/mint authority; `is_verified` not false; no critical/warning flags
- (if enabled) not dumping: 5m > −5%, 1h > −15%, 6h > −12%, 24h > −25%

The agent still does final live checks (audit, portfolio slots, balance) before deploying.
