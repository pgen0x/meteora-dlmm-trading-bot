# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A standalone Go daemon (`mdtb`) that polls Meteora's public DLMM pool-discovery
API, screens pools through quality gates, dedups, and forwards each poll cycle's
*batch* of newly-qualifying pools as one HMAC-signed webhook to a Hermes agent.
The agent (logic in `assets/hermes/`) ranks the batch, picks one pool + strategy,
and deploys via the skill in `assets/skill/`. This daemon owns **entry signals
only** — exits are the `dlmm_monitor.py` cron's job.

## Build / run

```bash
go build -o mdtb .              # build the daemon
go vet ./...                   # vet
set -a && . ./.env && set +a   # load env (fish: use bass or `env` prefix)
./mdtb                          # run (reads config from environment)
./install.sh ~/.hermes/profiles/dlmm   # wire assets into a Hermes profile + build
```

There are **no Go tests** and no Makefile. All config is environment-driven
(`internal/config`, defaults in `.env.example`); nothing is hardcoded except the
screening thresholds.

## Architecture

Pipeline is a single loop in `internal/scanner`: `poll ▸ screen ▸ dedup ▸ forward`,
one pass per enabled mode per `POLL_INTERVAL`.

- `internal/config` — env → `Config`. Only enable-toggles live here; screening
  thresholds live in `internal/meteora`.
- `internal/meteora`
  - `discover.go` — builds the discovery API query. `buildFilters` pushes mode
    thresholds into the API's `filter_by` param (API-side prefilter), but
    `Screen` re-checks **every** gate locally — the API filter is best-effort.
  - `screen.go` — the gate logic and `ModeParams` (`Casual`, `Multiday`). This
    is a **verbatim port** of the Python `dlmm_pipeline.py` / Meridian config.
    When changing gates or thresholds, keep them in sync with that upstream, or
    note the divergence — the comments cite where each value came from.
  - `momentum.go` — best-effort DexScreener downtrend gate (fail-open).
  - `types.go` — JSON structs mirroring the discovery API response exactly.
- `internal/store` — `Seen` dedup set: Redis (`SetNX`, one key + TTL per pool)
  or in-memory map. Empty `REDIS_ADDR` selects in-memory.
- `internal/webhook` — HMAC-SHA256 forwarder. Signature scheme
  (`hex(HMAC-SHA256(secret, body))` in `X-Webhook-Signature`) must match the
  Hermes/gobot side; the shared secret is `HERMES_WEBHOOK_SECRET`.
- `assets/` — copied into a Hermes profile by `install.sh`, which rewrites the
  literal `__PROFILE__` token to the target path. `assets/skill` = solana-dlmm
  skill (Python pipeline/monitor + JS executor); `assets/hermes` = the webhook
  subscription (agent decision prompt).

## Conventions that matter

- **Batch, not per-pool.** One cycle emits all fresh candidates as a single
  signal array so the agent compares the set. Don't revert to first-come
  per-pool sends.
- **Dedup before momentum fetch** — avoids hitting DexScreener for already-seen
  pools. On webhook failure, the whole batch is `Unmark`ed to retry next cycle.
- **Fail-open gates.** `verified` / Jupiter shield / momentum treat missing data
  as passing (`boolOr(..., true)`). Preserve this — the API omits these fields
  for some tokens and failing closed would over-reject.
- **Redis TTL is per-key on purpose.** `SetNX` per pool gives each an independent
  rolling `SEEN_TTL`. The old `SAdd`+`Expire` refreshed the whole set's TTL every
  write so pools were deduped forever — see the comment in `store.go`; don't
  reintroduce a single-key set.
- **No hidden clock reads.** `webhook.Send` takes `nowUnix` from the caller.
  Keep time injection at the edges.
- **Webhook payload is a contract** documented in `docs/SIGNAL_SCHEMA.md`.
  Update that doc when changing the emitted shape.

## Security

Wallet keys never live in this repo — the skill reads `SOLANA_PUBLIC_KEY` /
`SOLANA_PRIVATE_KEY` from the Hermes profile `.env` at runtime. Keep
`HERMES_WEBHOOK_SECRET` matched on both daemon and subscription. Trades real funds.
