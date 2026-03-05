#!/usr/bin/env python3
"""Plot price evolution and market statistics from a multi-asset randomwalk simulation run."""

import json
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np

LOG_DIR = Path("logs/randomwalk")
OUT_PNG = LOG_DIR / "price_evolution.png"
USD_PRECISION = 100_000

ASSETS = ["ABC", "DEF", "GHI"]
COLORS = {
    "perp": ["steelblue", "mediumseagreen", "mediumpurple"],
    "spot": ["darkorange", "firebrick", "darkcyan"],
}


def load_events(path: Path) -> list[dict]:
    if not path.exists():
        return []
    events = []
    with path.open() as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    events.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
    return events


def to_seconds(ns: int, t0: int) -> float:
    return (ns - t0) / 1e9


def to_usd(price_units: int) -> float:
    return price_units / USD_PRECISION


def rolling_count(times: np.ndarray, window_s: float = 10.0) -> tuple[np.ndarray, np.ndarray]:
    if len(times) == 0:
        return np.array([]), np.array([])
    t_min, t_max = times[0], times[-1]
    step = window_s / 2
    centers = np.arange(t_min + window_s / 2, t_max, step)
    counts = np.empty(len(centers))
    lo = 0
    for i, c in enumerate(centers):
        lo_bound, hi_bound = c - window_s / 2, c + window_s / 2
        while lo < len(times) and times[lo] < lo_bound:
            lo += 1
        hi = lo
        while hi < len(times) and times[hi] < hi_bound:
            hi += 1
        counts[i] = hi - lo
    return centers, counts


def parse_instrument(events: list[dict], t0: int) -> dict:
    trade_times, trade_prices = [], []
    spread_times, spread_bps = [], []

    for ev in events:
        t = to_seconds(ev["sim_ts"], t0)
        name = ev["event"]
        data = ev["data"]

        if name == "Trade":
            trade_times.append(t)
            trade_prices.append(to_usd(data["price"]))

        elif name == "BookSnapshot":
            bids = data.get("bids") or []
            asks = data.get("asks") or []
            if bids and asks:
                bid = to_usd(bids[0]["price"])
                ask = to_usd(asks[0]["price"])
                mid = (bid + ask) / 2
                if mid > 0:
                    spread_times.append(t)
                    spread_bps.append((ask - bid) / mid * 10_000)

    return {
        "trade_times": np.array(trade_times),
        "trade_prices": np.array(trade_prices),
        "spread_times": np.array(spread_times),
        "spread_bps": np.array(spread_bps),
    }


def parse_funding(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    rate_times, rates, settlement_times = [], [], []
    for ev in events:
        t = to_seconds(ev["sim_ts"], t0)
        name = ev["event"]
        data = ev["data"]
        if name == "funding_rate_update":
            rate_times.append(t)
            rates.append(data["rate"])
        elif name == "balance_change" and data.get("reason") == "funding_settlement":
            if t not in settlement_times:
                settlement_times.append(t)
    return np.array(rate_times), np.array(rates), np.array(sorted(set(settlement_times)))


def format_price(x, _):
    if x >= 1000:
        return f"${x:,.0f}"
    return f"${x:.2f}"


def main():
    # Find t0 from the first available perp log.
    t0 = None
    for asset in ASSETS:
        path = LOG_DIR / "perp" / f"{asset}-PERP.jsonl"
        if path.exists():
            events = load_events(path)
            if events:
                t0 = events[0]["sim_ts"]
                break

    if t0 is None:
        print("No perp logs found. Run 'make run-randomwalk' first.", file=sys.stderr)
        sys.exit(1)

    # Load all per-asset data.
    asset_data = {}
    for asset in ASSETS:
        perp_events = load_events(LOG_DIR / "perp" / f"{asset}-PERP.jsonl")
        spot_events = load_events(LOG_DIR / "spot" / f"{asset}-USD.jsonl")
        if not perp_events:
            continue
        perp = parse_instrument(perp_events, t0)
        spot = parse_instrument(spot_events, t0)
        rate_times, rates, settlements = parse_funding(perp_events, t0)
        asset_data[asset] = {
            "perp": perp,
            "spot": spot,
            "rate_times": rate_times,
            "rates": rates,
            "settlements": settlements,
        }

    n = len(asset_data)
    if n == 0:
        print("All perp logs empty.", file=sys.stderr)
        sys.exit(1)

    # Layout: one row per asset, 2 columns (price+basis | funding+volume).
    fig, axes = plt.subplots(n, 2, figsize=(14, 5 * n))
    if n == 1:
        axes = axes.reshape(1, 2)
    fig.suptitle(f"Random Walk Simulation — {n}-Asset ({', '.join(asset_data.keys())})",
                 fontsize=13, fontweight="bold")

    for row, (asset, d) in enumerate(asset_data.items()):
        perp = d["perp"]
        spot = d["spot"]
        settlements = d["settlements"]
        rate_times = d["rate_times"]
        rates = d["rates"]
        perp_sym = f"{asset}-PERP"
        spot_sym = f"{asset}-USD"

        # Left column: price evolution + basis as inset or secondary plot
        ax = axes[row, 0]
        if len(perp["trade_prices"]) > 0:
            ax.plot(perp["trade_times"], perp["trade_prices"],
                    color="steelblue", linewidth=0.8, label=perp_sym)
        if len(spot["trade_prices"]) > 0:
            ax.plot(spot["trade_times"], spot["trade_prices"],
                    color="darkorange", linewidth=1.0, alpha=0.85, label=spot_sym)
        for st in settlements:
            ax.axvline(st, color="purple", linestyle=":", linewidth=0.6, alpha=0.4)
        ax.set_title(f"{asset} — Price (purple = funding settlements)")
        ax.set_xlabel("Time (s)")
        ax.set_ylabel("Price (USD)")
        ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_price))
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

        # Right column: funding rate + summary stats
        ax = axes[row, 1]
        if len(rate_times) > 0:
            ax.step(rate_times, rates, where="post", color="indigo", linewidth=1.2,
                    label=f"Funding ({len(settlements)} settlements)")
            ax.axhline(0, color="gray", linestyle="--", linewidth=0.8)
            for st in settlements:
                ax.axvline(st, color="purple", linestyle=":", linewidth=0.6, alpha=0.4)
        ax.set_title(f"{asset} — Funding Rate (bps)")
        ax.set_xlabel("Time (s)")
        ax.set_ylabel("Rate (bps)")
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

        # Annotate with summary stats as text on the right plot.
        lines = []
        if len(perp["trade_prices"]) > 0:
            p = perp["trade_prices"]
            lines += [
                f"{perp_sym}: {len(p):,} trades",
                f"  ${p[0]:,.2f} → ${p[-1]:,.2f}  (drift {p[-1]-p[0]:+,.2f})",
                f"  range ${p.min():,.2f}–${p.max():,.2f}",
            ]
            if len(perp["spread_bps"]) > 0:
                lines.append(f"  avg spread {perp['spread_bps'].mean():.1f} bps")
        if len(spot["trade_prices"]) > 0:
            s = spot["trade_prices"]
            lines.append(f"{spot_sym}: {len(s):,} trades, end ${s[-1]:,.2f}")
        if len(rates) > 0:
            lines.append(f"Funding avg {rates.mean():.1f} bps, range {rates.min()}–{rates.max()}")
        ax.text(0.02, 0.97, "\n".join(lines), transform=ax.transAxes,
                verticalalignment="top", fontfamily="monospace", fontsize=7.5,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

    fig.tight_layout()
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    fig.savefig(OUT_PNG, dpi=150, bbox_inches="tight")
    print(f"Saved: {OUT_PNG}")


if __name__ == "__main__":
    main()
