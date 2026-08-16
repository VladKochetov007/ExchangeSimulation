"""Draw what a basic-actor market does, from the run's own event logs.

Nothing here knows a "correct" price. Every series is reconstructed from trades
and quotes the participants produced, which is the only thing that exists.

Usage:
    python tools/visualize_market.py --run logs/<name> --out research/visual
"""

from __future__ import annotations

import argparse
import collections
import json
import math
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

PRECISION = 100_000  # USD units per dollar, matching the venue's quote precision



def derivative_series(path: Path, bucket_ns: int, kinds: tuple[str, ...]) -> dict[str, tuple[list[float], list[float]]]:
    """Last trade price per bucket for each derivative symbol matching a kind."""
    prices: dict[str, dict[int, int]] = collections.defaultdict(dict)
    for line in path.open():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("event") != "Trade":
            continue
        outer = event["data"]["payload"]
        symbol = outer.get("symbol") or ""
        # Derivative trades nest the trade body inside the symbol envelope.
        payload = outer.get("payload") or outer
        price = payload.get("price")
        if price and any(k in symbol for k in kinds):
            prices[symbol][event["sim_ts"] // bucket_ns] = price
    out = {}
    for symbol, by_bucket in prices.items():
        buckets = sorted(by_bucket)
        if len(buckets) < 5:
            continue
        out[symbol] = (buckets, [by_bucket[b] / PRECISION for b in buckets])
    return out


def black_scholes_call(spot: float, strike: float, years: float, vol: float) -> float:
    """Textbook Black-Scholes with zero rates, for comparison against traded premia."""
    if years <= 0 or vol <= 0 or spot <= 0 or strike <= 0:
        return max(0.0, spot - strike)
    d1 = (math.log(spot / strike) + 0.5 * vol * vol * years) / (vol * math.sqrt(years))
    d2 = d1 - vol * math.sqrt(years)
    ncdf = lambda x: 0.5 * (1 + math.erf(x / math.sqrt(2)))
    return spot * ncdf(d1) - strike * ncdf(d2)


def trade_series(book: Path, bucket_ns: int) -> tuple[list[float], list[float]]:
    """Last trade price per time bucket, in whole quote units."""
    prices: dict[int, int] = {}
    for line in book.open():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("event") != "Trade":
            continue
        payload = event["data"]["payload"]
        price = payload.get("price")
        if price:
            prices[event["sim_ts"] // bucket_ns] = price
    if not prices:
        return [], []
    buckets = sorted(prices)
    start = buckets[0]
    hours = [(b - start) * bucket_ns / 3.6e12 for b in buckets]
    return hours, [prices[b] / PRECISION for b in buckets]


def align(a_x: list[float], a_y: list[float], b_x: list[float], b_y: list[float]):
    """Pair two series on their shared time buckets."""
    b = dict(zip(b_x, b_y))
    xs, ys, zs = [], [], []
    for x, y in zip(a_x, a_y):
        if x in b:
            xs.append(x)
            ys.append(y)
            zs.append(b[x])
    return xs, ys, zs


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--bucket-seconds", type=int, default=60)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    bucket = args.bucket_seconds * 1_000_000_000

    venues = sorted(p.name for p in (args.run / "venues").iterdir() if p.is_dir())
    series: dict[tuple[str, str], tuple[list[float], list[float]]] = {}
    for venue in venues:
        spot_dir = args.run / "venues" / venue / "spot"
        if spot_dir.exists():
            for book in sorted(spot_dir.glob("*.jsonl")):
                series[(venue, book.stem)] = trade_series(book, bucket)
        deriv = args.run / "venues" / venue / "derivatives.jsonl"
        if deriv.exists():
            series[(venue, "derivatives")] = ([], [])

    # 1. Emergent price path per venue, and the dispersion between venues.
    fig, axes = plt.subplots(2, 1, figsize=(11, 7), sharex=True,
                             gridspec_kw={"height_ratios": [3, 1]})
    for venue in venues:
        x, y = series.get((venue, "ABC-USD"), ([], []))
        if x:
            axes[0].plot(x, y, linewidth=0.9, label=f"{venue}")
    axes[0].set_title("ABC/USD: price formed only by participants' orders")
    axes[0].set_ylabel("price (USD)")
    axes[0].legend(loc="upper left", fontsize=8)
    axes[0].grid(alpha=0.3)

    ref = series.get((venues[0], "ABC-USD"), ([], []))
    for venue in venues[1:]:
        x, a, b = align(*ref, *series.get((venue, "ABC-USD"), ([], [])))
        if x:
            axes[1].plot(x, [10000 * (bb - aa) / aa for aa, bb in zip(a, b)],
                         linewidth=0.8, label=f"{venue} vs {venues[0]}")
    axes[1].axhline(0, color="black", linewidth=0.6)
    axes[1].set_title("cross-venue dispersion, held together by arbitrage")
    axes[1].set_ylabel("bps")
    axes[1].set_xlabel("simulated hours")
    axes[1].legend(loc="upper left", fontsize=8)
    axes[1].grid(alpha=0.3)
    fig.tight_layout()
    fig.savefig(args.out / "01-price-and-venue-dispersion.png", dpi=130)
    plt.close(fig)

    # 2. The triangular loop: ABC/USD against ABC/CDF x CDF/USD.
    fig, ax = plt.subplots(figsize=(11, 4))
    for venue in venues:
        direct = series.get((venue, "ABC-USD"), ([], []))
        cross = series.get((venue, "ABC-CDF"), ([], []))
        quote = series.get((venue, "CDF-USD"), ([], []))
        if not (direct[0] and cross[0] and quote[0]):
            continue
        x1, d, c = align(*direct, *cross)
        x2, c2, q = align(x1, c, *quote)
        d2 = dict(zip(x1, d))
        loop = [10000 * ((cc * qq) / d2[x] - 1) for x, cc, qq in zip(x2, c2, q) if d2.get(x)]
        ax.plot(x2[: len(loop)], loop, linewidth=0.8, label=venue)
    ax.axhline(0, color="black", linewidth=0.6)
    ax.set_title("triangular loop residual: ABC/CDF x CDF/USD against ABC/USD")
    ax.set_ylabel("bps")
    ax.set_xlabel("simulated hours")
    ax.legend(loc="upper left", fontsize=8)
    ax.grid(alpha=0.3)
    fig.tight_layout()
    fig.savefig(args.out / "03-triangular-loop.png", dpi=130)
    plt.close(fig)


    # 2. Spot against the perpetual and the dated futures, which arbitrage ties together.
    fig, axes = plt.subplots(2, 1, figsize=(11, 7), sharex=True)
    venue = venues[0]
    deriv_path = args.run / "venues" / venue / "derivatives.jsonl"
    spot_x, spot_y = series.get((venue, "ABC-USD"), ([], []))
    spot_book = args.run / "venues" / venue / "spot" / "ABC-USD.jsonl"
    spot_by_bucket: dict[int, float] = {}
    if spot_book.exists():
        for line in spot_book.open():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if event.get("event") != "Trade":
                continue
            body = event["data"]["payload"]
            body = body.get("payload") or body
            if body.get("price"):
                spot_by_bucket[event["sim_ts"] // bucket] = body["price"] / PRECISION
    if deriv_path.exists() and spot_x:
        perp = derivative_series(deriv_path, bucket, ("PERP",))
        futures = derivative_series(deriv_path, bucket, ("-FUT-", "FUT"))
        start = min(spot_by_bucket) if spot_by_bucket else 0
        for symbol, (buckets, prices) in sorted(perp.items()):
            hours = [(b - start) * bucket / 3.6e12 for b in buckets]
            basis = [10000 * (p / spot_by_bucket[b] - 1) for b, p in zip(buckets, prices) if b in spot_by_bucket]
            axes[0].plot(hours[: len(basis)], basis, linewidth=0.8, label=symbol)
        for symbol, (buckets, prices) in sorted(futures.items()):
            hours = [(b - start) * bucket / 3.6e12 for b in buckets]
            basis = [10000 * (p / spot_by_bucket[b] - 1) for b, p in zip(buckets, prices) if b in spot_by_bucket]
            axes[1].plot(hours[: len(basis)], basis, linewidth=0.8, label=symbol)
    axes[0].axhline(0, color="black", linewidth=0.6)
    axes[0].set_title(f"{venue}: perpetual against spot, held by carry desks and funding")
    axes[0].set_ylabel("basis (bps)")
    axes[0].legend(loc="upper left", fontsize=7)
    axes[0].grid(alpha=0.3)
    axes[1].axhline(0, color="black", linewidth=0.6)
    axes[1].set_title("dated futures against spot, converging into settlement")
    axes[1].set_ylabel("basis (bps)")
    axes[1].set_xlabel("simulated hours")
    axes[1].legend(loc="upper left", fontsize=7)
    axes[1].grid(alpha=0.3)
    fig.tight_layout()
    fig.savefig(args.out / "02-spot-perp-futures-basis.png", dpi=130)
    plt.close(fig)

    # 4. Traded option premia against a textbook Black-Scholes value.
    if deriv_path.exists() and spot_by_bucket:
        options = derivative_series(deriv_path, bucket, ("-C", "-P"))
        rows = []
        for symbol, (buckets, prices) in options.items():
            parts = symbol.split("-")
            if len(parts) < 4:
                continue
            try:
                strike = float(parts[-2])
                expiry_ns = float(parts[-3]) * 1e9  # symbols carry epoch seconds
            except ValueError:
                continue
            is_call = parts[-1].startswith("C")
            for b, premium in zip(buckets, prices):
                spot = spot_by_bucket.get(b)
                if not spot:
                    continue
                years = max(0.0, (expiry_ns - b * bucket) / (365 * 24 * 3.6e12))
                theo = black_scholes_call(spot, strike, years, 0.8)
                if not is_call:
                    theo = max(0.0, theo - spot + strike)
                rows.append((theo, premium))
        if rows:
            fig, ax = plt.subplots(figsize=(6.5, 6.5))
            ax.scatter([r[0] for r in rows], [r[1] for r in rows], s=6, alpha=0.35)
            top = max(max(r[0] for r in rows), max(r[1] for r in rows))
            ax.plot([0, top], [0, top], color="black", linewidth=0.8)
            ax.set_title("traded option premium against Black-Scholes")
            ax.set_xlabel("Black-Scholes value (USD)")
            ax.set_ylabel("traded premium (USD)")
            ax.grid(alpha=0.3)
            fig.tight_layout()
            fig.savefig(args.out / "04-option-premium-vs-black-scholes.png", dpi=130)
            plt.close(fig)

    print(f"wrote plots to {args.out}")


if __name__ == "__main__":
    main()
