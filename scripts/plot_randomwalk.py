#!/usr/bin/env python3
"""Plot price evolution and market statistics from a randomwalk simulation run."""

import json
import sys
from collections import deque
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np

LOG_DIR = Path("logs/randomwalk")
PERP_LOG = LOG_DIR / "perp" / "BTC-PERP.jsonl"
OUT_PNG = LOG_DIR / "price_evolution.png"

USD_PRECISION = 100_000


def load_events(path: Path) -> list[dict]:
    events = []
    with path.open() as f:
        for line in f:
            line = line.strip()
            if line:
                events.append(json.loads(line))
    return events


def to_seconds(ns: int, t0: int) -> float:
    return (ns - t0) / 1e9


def to_usd(price_units: int) -> float:
    return price_units / USD_PRECISION


def rolling_count(times: np.ndarray, window_s: float = 10.0) -> tuple[np.ndarray, np.ndarray]:
    """Return (time_centers, counts_per_window) for a rolling count over `times`."""
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


def main():
    if not PERP_LOG.exists():
        print(f"Log not found: {PERP_LOG}\nRun 'make run-randomwalk' first.", file=sys.stderr)
        sys.exit(1)

    events = load_events(PERP_LOG)
    if not events:
        print("Log is empty.", file=sys.stderr)
        sys.exit(1)

    t0 = events[0]["sim_ts"]

    trade_times, trade_prices = [], []
    spread_times, spread_bps = [], []
    reject_times = []

    for ev in events:
        t = to_seconds(ev["sim_ts"], t0)
        name = ev["event"]
        data = ev["data"]

        if name == "Trade":
            price = to_usd(data["price"])
            trade_times.append(t)
            trade_prices.append(price)

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

        elif name == "OrderRejected":
            reject_times.append(t)

    trade_times = np.array(trade_times)
    trade_prices = np.array(trade_prices)
    spread_times = np.array(spread_times)
    spread_bps = np.array(spread_bps)
    reject_times = np.array(reject_times)

    returns = np.diff(trade_prices) if len(trade_prices) > 1 else np.array([])

    trade_centers, trade_counts = rolling_count(trade_times)
    reject_centers, reject_counts = rolling_count(reject_times)

    bootstrap_price = to_usd(50_000 * USD_PRECISION)

    fig, axes = plt.subplots(3, 2, figsize=(14, 12))
    fig.suptitle("BTC-PERP Random Walk Simulation", fontsize=14, fontweight="bold")

    # 1. Price evolution
    ax = axes[0, 0]
    ax.plot(trade_times, trade_prices, color="steelblue", linewidth=0.8)
    ax.axhline(bootstrap_price, color="red", linestyle="--", linewidth=1, label=f"Bootstrap (${bootstrap_price:,.0f})")
    ax.set_title("BTC-PERP Price Evolution")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(lambda x, _: f"${x:,.0f}"))
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # 2. Bid-ask spread
    ax = axes[0, 1]
    ax.plot(spread_times, spread_bps, color="green", linewidth=0.6, alpha=0.8)
    ax.set_title("Bid-Ask Spread Evolution")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Spread (basis points)")
    ax.grid(True, alpha=0.3)

    # 3. Trade volume rolling 10s
    ax = axes[1, 0]
    if len(trade_centers) > 0:
        ax.fill_between(trade_centers, trade_counts, alpha=0.6, color="orange")
        ax.plot(trade_centers, trade_counts, color="darkorange", linewidth=0.8)
    ax.set_title("Trade Volume (rolling 10s window)")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Trades per 10s")
    ax.grid(True, alpha=0.3)

    # 4. Returns distribution
    ax = axes[1, 1]
    if len(returns) > 0:
        ax.hist(returns, bins=60, density=True, color="mediumpurple", edgecolor="none", alpha=0.85)
        ax.axvline(0, color="red", linestyle="--", linewidth=1, label="Zero return")
        ax.set_title("Returns Distribution")
        ax.set_xlabel("Price Change (USD)")
        ax.set_ylabel("Density")
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

    # 5. Order rejection rate rolling 10s
    ax = axes[2, 0]
    if len(reject_centers) > 0:
        ax.fill_between(reject_centers, reject_counts, alpha=0.5, color="red")
        ax.plot(reject_centers, reject_counts, color="darkred", linewidth=0.8)
    ax.set_title("Order Rejection Rate (rolling 10s window)")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Rejections per 10s")
    ax.grid(True, alpha=0.3)

    # 6. Summary stats
    ax = axes[2, 1]
    ax.axis("off")
    if len(trade_prices) > 0:
        stats = [
            f"Total trades:      {len(trade_prices):,}",
            f"Start price:       ${trade_prices[0]:,.2f}",
            f"End price:         ${trade_prices[-1]:,.2f}",
            f"Price drift:       ${trade_prices[-1] - trade_prices[0]:+,.2f}",
            f"Price range:       ${trade_prices.min():,.2f} – ${trade_prices.max():,.2f}",
            f"Sim duration:      {trade_times[-1]:.1f}s",
            f"Total rejections:  {len(reject_times):,}",
        ]
        if len(spread_bps) > 0:
            stats.insert(3, f"Avg spread:        {spread_bps.mean():.1f} bps")
        ax.text(0.05, 0.95, "\n".join(stats), transform=ax.transAxes,
                verticalalignment="top", fontfamily="monospace", fontsize=10)

    fig.tight_layout()
    fig.savefig(OUT_PNG, dpi=150, bbox_inches="tight")
    print(f"Saved: {OUT_PNG}")


if __name__ == "__main__":
    main()
