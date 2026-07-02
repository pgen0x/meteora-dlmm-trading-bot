# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-07-02

Initial beta release. Everything below was consolidated from pre-release
development history into this first tagged version.

### Added
- Go daemon (`mds`): continuous poll ▸ screen ▸ dedup ▸ forward loop against
  Meteora's public pool-discovery API.
- Dual-mode screening: `casual` (30m, volume-spike plays) and `multiday`
  (24h, quality holds), each with independent thresholds and isolated
  position budgets.
- Layered risk gates: TVL, fee/TVL, market cap, holder count, organic score,
  top-10/dev supply concentration, mint/freeze authority, Jupiter shield
  status, and a best-effort DexScreener downtrend filter (Meridian
  degen-score and bin-step gates ported from the upstream Python pipeline).
- Batch AI-Pick signalling: one HMAC-signed webhook per cycle carries every
  qualifying pool as an array, replacing first-come-per-pool sends so the
  agent compares the full batch before deploying.
- Pluggable dedup store: in-memory, or Redis with a per-pool rolling TTL
  (`SetNX`, not a shared-set `SAdd`+`Expire`, so pools don't dedupe forever).
- `solana-dlmm` skill: `dlmm_pipeline.py` (ingestion/deploy), `dlmm_monitor.py`
  (exit management — stop-loss, trailing take-profit, out-of-range, and a
  Close GUARD that refuses to close a healthy in-range high-fee position),
  `dlmm_executor.js` (on-chain execution).
- `install.sh`: wires the skill, webhook subscription, SOUL.md section-9
  template, and DLMM-relevant cron job templates (5m position monitor, daily
  self-improvement review) into a Hermes profile, and builds the daemon.
- `docs/SIGNAL_SCHEMA.md`: the webhook payload contract.
- `CLAUDE.md`: architecture and convention notes for AI-assisted development.
- `./mds -version` / `main.Version` for reporting the running build version.

### Changed
- `assets/skill/scripts/` is symlinked into installed profiles instead of
  copied + `sed`-rewritten — edits in this repo now take effect in every
  installed profile immediately. Scripts resolve their own profile directory
  at runtime instead of relying on an install-time path substitution.
- `dlmm_executor.js` resolves its own path via `process.argv[1]` rather than
  `__dirname`, since Node always resolves symlinks for the latter (unlike
  Python's `__file__`), which broke wallet-key lookup once scripts became
  symlinked.
- npm dependencies for the skill are installed at the repo level
  (`assets/skill/node_modules`), not per-profile, since `require()` resolution
  follows the same realpath-through-symlinks behavior as `__dirname`.

### Fixed
- Webhook report formatting switched to a native pipe-table (code-block
  fencing was falling back to legacy MarkdownV2 and leaking literal escape
  characters).
- Audit-token reject gate loosened to stop over-rejecting on non-critical
  risk levels.

### Security
- Removed hardcoded Helius/QuickNode RPC provider keys that had been
  committed in plaintext since the initial commit. `dlmm_executor.js` now
  reads `SOLANA_RPC_URLS` from the profile `.env`, falling back to the public
  mainnet-beta RPC if unset. Git history was scrubbed
  (`git filter-repo --replace-text`) and force-pushed; the original keys were
  rotated at the provider regardless, since history rewrites don't un-expose
  something GitHub already cached.
