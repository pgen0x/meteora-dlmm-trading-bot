#!/usr/bin/env python3
"""
audit_token.py — Solana token security audit using Binance Web3 API.
"""

import json
import sys
import uuid
import subprocess

def run_command(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        return result.stdout.strip(), result.stderr.strip()
    except Exception as e:
        return "", str(e)

def main():
    if len(sys.argv) < 2:
        print("Usage: audit_token.py <MINT>")
        sys.exit(1)

    mint = sys.argv[1]
    request_id = str(uuid.uuid4())

    payload = {
        "binanceChainId": "CT_501",
        "contractAddress": mint,
        "requestId": request_id
    }

    cmd = f"""curl -s --location 'https://web3.binance.com/bapi/defi/v1/public/wallet-direct/security/token/audit' \
--header 'Content-Type: application/json' \
--header 'source: agent' \
--header 'Accept-Encoding: identity' \
--header 'User-Agent: binance-web3/1.4 (Skill)' \
--data '{json.dumps(payload)}'"""

    out, err = run_command(cmd)
    if err:
        print(json.dumps({"verdict": "FAIL", "reason": f"API Error: {err}"}))
        sys.exit(0)

    try:
        data = json.loads(out)
        if not data.get("success"):
            print(json.dumps({"verdict": "FAIL", "reason": f"API returned success=false: {data.get('message')}"}))
            sys.exit(0)

        audit = data.get("data", {})
        if not audit.get("hasResult") or not audit.get("isSupported"):
             print(json.dumps({"verdict": "FAIL", "reason": "Security audit not available/supported for this token."}))
             sys.exit(0)

        risk_level = audit.get("riskLevel", 0)
        risk_enum = audit.get("riskLevelEnum", "UNKNOWN")

        if risk_level >= 4:
            print(json.dumps({
                "verdict": "FAIL", 
                "reason": f"High risk detected (Level {risk_level}: {risk_enum})",
                "risk_level": risk_level,
                "risk_items": audit.get("riskItems", [])
            }))
        else:
            # Dev holding check — fetch token metrics from Binance pulse API
            dev_payload = json.dumps({"chainId": "CT_501", "contractAddress": mint, "limit": 1})
            dev_cmd = f"""curl -s -X POST 'https://web3.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/market/token/pulse/rank/list/ai' \
-H 'Content-Type: application/json' -H 'User-Agent: binance-web3/1.4 (Skill)' \
-d '{dev_payload}'"""
            dev_out, _ = run_command(dev_cmd)
            token_info = None
            dev_pct = 0
            try:
                dev_data = json.loads(dev_out)
                token_list = dev_data.get("data", [])
                token_info = next((t for t in token_list if t.get("contractAddress") == mint), None)
                if token_info:
                    dev_pct = float(token_info.get("holdersDevPercent", 0) or 0)
                    top10_pct = float(token_info.get("holdersTop10Percent", 0) or 0)
                    if dev_pct > 30:
                        print(json.dumps({"verdict": "FAIL", "reason": f"Dev holds {dev_pct:.1f}% of supply — rug risk"}))
                        sys.exit(0)
                    if top10_pct > 95:
                        print(json.dumps({"verdict": "FAIL", "reason": f"Top 10 wallets hold {top10_pct:.1f}% — extreme concentration risk"}))
                        sys.exit(0)
            except Exception:
                pass  # skip dev check if API unavailable
            print(json.dumps({
                "verdict": "PASS",
                "risk_level": risk_level,
                "risk_enum": risk_enum,
                "buy_tax": audit.get("extraInfo", {}).get("buyTax"),
                "sell_tax": audit.get("extraInfo", {}).get("sellTax"),
                "dev_pct": dev_pct if token_info else None
            }))

    except Exception as e:
        print(json.dumps({"verdict": "FAIL", "reason": f"Parsing Error: {str(e)}", "output": out[:200]}))

if __name__ == "__main__":
    main()
