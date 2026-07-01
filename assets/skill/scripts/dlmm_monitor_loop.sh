#!/bin/bash
# dlmm_monitor_loop.sh — Continuous DLMM position monitor (20s interval).
# Runs the one-shot dlmm_monitor.py scan in a loop so auto-close + auto-swap-to-SOL
# fire reliably instead of depending on ad-hoc gateway LLM invocations.

set -a
source __PROFILE__/.env 2>/dev/null
set +a

SCRIPT_DIR="__PROFILE__/skills/solana-dlmm/scripts"
PYTHON="python3"

echo "Starting DLMM Position Monitor Loop (20s interval)..."

cd "$SCRIPT_DIR" || exit 1
# Sweep stranded tokens every SWEEP_EVERY iterations (30 × 20s ≈ 10min).
# On-close auto-swap only fires once; if it aborts on the impact guard the token is
# orphaned (no longer an active position), so this re-sweeps to liquidate it.
SWEEP_EVERY=30
i=0
while true; do
    "$PYTHON" "$SCRIPT_DIR/dlmm_monitor.py"
    i=$((i + 1))
    if [ "$i" -ge "$SWEEP_EVERY" ]; then
        "$PYTHON" "$SCRIPT_DIR/dlmm_monitor.py" --cleanup-tokens
        i=0
    fi
    sleep 20
done
