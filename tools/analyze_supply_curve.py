"""Measure how much volume it takes to move the price a given distance.

The square-root impact law is derived from a latent supply density that
vanishes linearly at the transaction price. That assumption has a direct
consequence which can be measured without metaorders at all: if the density
grows linearly with distance from the price, then the volume V needed to move
the price by x grows as x squared, and inverting gives impact proportional to
the square root of volume.

So this measures V(x) directly by first passage. From each trade, walk forward
accumulating signed volume until the midpoint has moved by at least x in either
direction, and record the absolute signed volume consumed. The median over many
starting points is V(x), and the slope of log V against log x is the exponent
to compare against two.

Usage:
    python tools/analyze_supply_curve.py --log <run>/venues/north/spot/ABC-USD.jsonl
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
from pathlib import Path


def load_tape(path: Path) -> tuple[list[float], list[float]]:
    """Midpoint and signed volume at each trade."""
    mids: list[float] = []
    signed: list[float] = []
    mid = 0.0
    with path.open() as handle:
        for line in handle:
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            payload = (record.get("data") or {}).get("payload") or {}
            if record.get("event") == "BookSnapshot" and record.get("client_id") == 0:
                bids, asks = payload.get("bids") or [], payload.get("asks") or []
                if bids and asks:
                    mid = (bids[0]["price"] + asks[0]["price"]) / 2
            elif record.get("event") == "Trade" and mid > 0:
                side = payload.get("side")
                qty = payload.get("qty", 0)
                if side not in ("BUY", "SELL") or qty <= 0:
                    continue
                mids.append(mid)
                signed.append(qty if side == "BUY" else -qty)
    return mids, signed


def volume_to_move(mids: list[float], signed: list[float], move_bps: float, starts: int, horizon: int) -> float | None:
    """Median absolute signed volume consumed before the price moves move_bps."""
    consumed: list[float] = []
    step = max(1, len(mids) // starts)
    for begin in range(0, len(mids) - 1, step):
        start_mid = mids[begin]
        if start_mid <= 0:
            continue
        total = 0.0
        for i in range(begin, min(begin + horizon, len(mids))):
            total += signed[i]
            move = 1e4 * abs(mids[i] - start_mid) / start_mid
            if move >= move_bps:
                consumed.append(abs(total))
                break
    if len(consumed) < 20:
        return None
    return statistics.median(consumed)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True, type=Path)
    parser.add_argument("--moves", type=float, nargs="+", default=[1, 2, 5, 10, 20, 50])
    parser.add_argument("--starts", type=int, default=4000)
    parser.add_argument("--horizon", type=int, default=20000)
    args = parser.parse_args()

    mids, signed = load_tape(args.log)
    print(f"trades: {len(mids)}")
    if len(mids) < 1000:
        raise SystemExit("not enough trades")

    points: list[tuple[float, float]] = []
    print(f"\n{'move (bps)':>11} {'median |signed volume| (units)':>32}")
    for move in args.moves:
        volume = volume_to_move(mids, signed, move, args.starts, args.horizon)
        if volume is None or volume <= 0:
            print(f"{move:>11.1f} {'insufficient':>32}")
            continue
        points.append((move, volume / 1e8))
        print(f"{move:>11.1f} {volume / 1e8:>32.4f}")

    if len(points) >= 3:
        xs = [math.log(p[0]) for p in points]
        ys = [math.log(p[1]) for p in points]
        mean_x, mean_y = sum(xs) / len(xs), sum(ys) / len(ys)
        slope = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys)) / sum((x - mean_x) ** 2 for x in xs)
        print(f"\nV(x) ~ x^{slope:.3f}   (square-root impact requires 2; linear impact requires 1)")
        print(f"implied impact exponent: {1 / slope:.3f}   (square-root law is 0.5)")


if __name__ == "__main__":
    main()
