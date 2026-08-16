"""Measure the order-flow and price-response statistics of Bouchaud et al.

arXiv:cond-mat/0307332 poses the tension this measures: the signs of market
orders are strongly persistent, yet prices are close to diffusive. The
resolution is that impact is transient, so three quantities have to be read
together rather than one at a time:

  * the autocorrelation of trade signs, C(l), which should decay slowly;
  * the response function R(l), the average signed price move l trades after a
    trade, which should rise and then flatten rather than grow linearly;
  * the variance ratio of returns in trade time, which should stay near one.

Everything is measured in *trade time*: the clock here is the trade counter,
not the wall clock, because that is the frame the persistence lives in.

Usage:
    python tools/analyze_response.py --log logs/research/<run>/venues/north/spot/ABC-USD.jsonl
"""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path


def load_trade_series(path: Path) -> tuple[list[int], list[float]]:
    """Return trade signs and the midpoint prevailing at each trade."""
    signs: list[int] = []
    mids: list[float] = []
    mid = 0.0
    with path.open() as handle:
        for line in handle:
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            event = record.get("event")
            payload = (record.get("data") or {}).get("payload") or {}
            if event == "BookSnapshot" and record.get("client_id") == 0:
                bids, asks = payload.get("bids") or [], payload.get("asks") or []
                if bids and asks:
                    mid = (bids[0]["price"] + asks[0]["price"]) / 2
            elif event == "Trade" and mid > 0:
                side = payload.get("side")
                if side not in ("BUY", "SELL"):
                    continue
                signs.append(1 if side == "BUY" else -1)
                mids.append(mid)
    return signs, mids


def sign_autocorrelation(signs: list[int], lags: list[int]) -> dict[int, float]:
    mean = sum(signs) / len(signs)
    variance = sum((s - mean) ** 2 for s in signs) / len(signs)
    result: dict[int, float] = {}
    for lag in lags:
        if lag >= len(signs) or variance <= 0:
            continue
        total = sum((signs[i] - mean) * (signs[i + lag] - mean) for i in range(len(signs) - lag))
        result[lag] = total / ((len(signs) - lag) * variance)
    return result


def response(signs: list[int], mids: list[float], lags: list[int]) -> dict[int, float]:
    """Average signed log return l trades after a trade, in basis points."""
    result: dict[int, float] = {}
    for lag in lags:
        if lag >= len(mids):
            continue
        total, count = 0.0, 0
        for i in range(len(mids) - lag):
            if mids[i] <= 0 or mids[i + lag] <= 0:
                continue
            total += signs[i] * math.log(mids[i + lag] / mids[i])
            count += 1
        if count:
            result[lag] = 1e4 * total / count
    return result


def variance_ratio(mids: list[float], scales: list[int]) -> dict[int, float]:
    """Variance of k-trade returns over k times the variance of one-trade returns."""
    single = [math.log(mids[i + 1] / mids[i]) for i in range(len(mids) - 1) if mids[i] > 0 and mids[i + 1] > 0]
    if len(single) < 10:
        return {}
    base = sum(r * r for r in single) / len(single)
    result: dict[int, float] = {}
    for scale in scales:
        sampled = [math.log(mids[i + scale] / mids[i]) for i in range(0, len(mids) - scale, scale) if mids[i] > 0]
        if len(sampled) < 10 or base <= 0:
            continue
        result[scale] = (sum(r * r for r in sampled) / len(sampled)) / (scale * base)
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True, type=Path)
    parser.add_argument("--lags", type=int, nargs="+", default=[1, 2, 5, 10, 20, 50, 100, 200, 500, 1000])
    args = parser.parse_args()

    signs, mids = load_trade_series(args.log)
    print(f"trades: {len(signs)}")
    if len(signs) < 100:
        raise SystemExit("not enough trades to measure anything")
    print(f"buy fraction: {sum(1 for s in signs if s > 0) / len(signs):.3f}")

    correlation = sign_autocorrelation(signs, args.lags)
    print("\nsign autocorrelation C(l)")
    for lag, value in correlation.items():
        print(f"  l={lag:<5d} {value:+.4f}")

    curve = response(signs, mids, args.lags)
    print("\nresponse R(l), basis points")
    for lag, value in curve.items():
        print(f"  l={lag:<5d} {value:+.3f}")

    ratios = variance_ratio(mids, [2, 5, 10, 50, 100])
    print("\nvariance ratio in trade time")
    for scale, value in ratios.items():
        print(f"  k={scale:<5d} {value:.3f}")


if __name__ == "__main__":
    main()
