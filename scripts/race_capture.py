#!/usr/bin/env python3
"""Measure latency-race capture from per-client fills and positions.

Three metrics, because the first two answer different questions and the third
is the one that is easy to get wrong:

  volume — filled base quantity: how many opportunities the entrant won
  net    — realized P&L minus fees: the economics of completed round trips
  equity — cash + spot inventory + perp unrealized: total economic P&L

`equity` exists because counting only the spot leg of a delta-neutral basis
arb turns pure inventory accumulation into apparent profit — the short perp
position that offsets it is a position, not a balance, so it never appears in
a wallet-only view. Any capture claim has to value both legs.

Usage: race_capture.py <log-dir-prefix> <seed> [<seed> ...]
"""
import json
import sys
from pathlib import Path

TIERS = {12: 1.0, 13: 0.5, 14: 0.2, 15: 0.05}
RACE_CLIENTS = list(TIERS)
BTC, USD = 10**8, 10**5


def collect(run_dir):
    vol = {c: 0 for c in RACE_CLIENTS}
    pnl = {c: 0 for c in RACE_CLIENTS}
    fees = {c: 0 for c in RACE_CLIENTS}
    # Last position per (client, symbol) and the last mark per symbol.
    pos = {}
    marks = {}
    spot_qty = {c: 0 for c in RACE_CLIENTS}
    spot_cash = {c: 0 for c in RACE_CLIENTS}

    for leg in ("spot", "perp"):
        for path in (run_dir / leg).glob("*.jsonl"):
            for line in path.open():
                d = json.loads(line)
                event = d.get("event")
                data = d.get("data") or {}
                if event == "mark_price_update":
                    marks[data["symbol"]] = data["mark_price"]
                    continue
                cid = d.get("client_id")
                if cid not in vol:
                    continue
                if event == "OrderFill":
                    vol[cid] += data.get("qty", 0)
                    pnl[cid] += data.get("realized_pnl", 0)
                    if data.get("fee_asset") == "USD":
                        fees[cid] += data.get("fee_amount", 0)
                    # Spot legs move inventory and cash; perp legs do not.
                    if leg == "spot":
                        signed = data["qty"] if data["side"] == "BUY" else -data["qty"]
                        spot_qty[cid] += signed
                        spot_cash[cid] -= signed * data["price"] // BTC
                elif event == "position_update":
                    pos[(cid, data["symbol"])] = (data["new_size"], data["new_entry_price"])

    unrealized = {c: 0 for c in RACE_CLIENTS}
    for (cid, symbol), (size, entry) in pos.items():
        mark = marks.get(symbol)
        if mark is None or size == 0:
            continue
        unrealized[cid] += size * (mark - entry) // BTC

    spot_mark = marks.get("ABC-PERP", 50_000 * USD)
    equity = {
        c: spot_cash[c] + spot_qty[c] * spot_mark // BTC + unrealized[c] + pnl[c] - fees[c]
        for c in RACE_CLIENTS
    }
    return vol, pnl, fees, equity


def spearman(xs, ys):
    def rank(vals):
        order = sorted(range(len(vals)), key=lambda i: vals[i])
        r = [0.0] * len(vals)
        for pos_, i in enumerate(order):
            r[i] = float(pos_)
        return r
    rx, ry = rank(xs), rank(ys)
    n = len(xs)
    return 1 - 6 * sum((rx[i] - ry[i]) ** 2 for i in range(n)) / (n * (n * n - 1))


def main():
    prefix, seeds = sys.argv[1], sys.argv[2:]
    speeds = [1.0 / TIERS[c] for c in RACE_CLIENTS]
    for metric in ("volume", "net", "equity"):
        print(f"\n=== {prefix}  metric={metric} ===")
        unit = "ABC" if metric == "volume" else "USD"
        print(f"{'seed':>6} " + " ".join(f"{TIERS[c]:>9}x" for c in RACE_CLIENTS) +
              f"   rank-corr   ({unit}, absolute)")
        corrs = []
        for seed in seeds:
            run_dir = Path(f"logs/{prefix}_{seed}")
            if not run_dir.exists():
                continue
            vol, pnl, fees, equity = collect(run_dir)
            if metric == "volume":
                vals = [vol[c] / BTC for c in RACE_CLIENTS]
            elif metric == "net":
                vals = [(pnl[c] - fees[c]) / USD for c in RACE_CLIENTS]
            else:
                vals = [equity[c] / USD for c in RACE_CLIENTS]
            c_ = spearman(speeds, vals)
            corrs.append(c_)
            print(f"{seed:>6} " + " ".join(f"{v:10.1f}" for v in vals) + f"   {c_:+.2f}")
        if corrs:
            print(f"{'':>6} " + " " * (11 * len(RACE_CLIENTS)) +
                  f"   {sum(corrs) / len(corrs):+.2f}  mean (n={len(corrs)})")


if __name__ == "__main__":
    main()
