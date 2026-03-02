#!/usr/bin/env python3
"""Plot price evolution and market statistics from a randomwalk simulation run.
Also generates a 10-second animated GIF of price evolution.
"""

import json
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import matplotlib.animation as animation
import numpy as np

LOG_DIR = Path("logs/randomwalk")
PERP_LOG = LOG_DIR / "perp" / "ABC-PERP.jsonl"
OUT_PNG = LOG_DIR / "price_evolution.png"
OUT_GIF = LOG_DIR / "price_evolution.gif"

USD_PRECISION = 100_000
GIF_DURATION_SEC = 10
GIF_FPS = 20


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


def rolling_count(times: np.ndarray, window_s: float = 10.0):
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


def create_price_gif(trade_times, trade_prices, bootstrap_price):
    if len(trade_times) < 2:
        print("Not enough trade data for GIF.")
        return

    total_frames = GIF_DURATION_SEC * GIF_FPS
    frame_indices = np.linspace(
        1, len(trade_times) - 1, total_frames
    ).astype(int)

    fig, ax = plt.subplots(figsize=(10, 6))

    ax.set_title("ABC-PERP Price Evolution")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(
        mticker.FuncFormatter(lambda x, _: f"${x:,.0f}")
    )

    line, = ax.plot([], [], color="steelblue", linewidth=1.2)

    price_text = ax.text(
        0.02, 0.95, "", transform=ax.transAxes,
        verticalalignment="top", fontsize=11,
        bbox=dict(facecolor="white", alpha=0.8, edgecolor="none")
    )

    ax.grid(True, alpha=0.3)

    def update(frame_idx):
        idx = frame_indices[frame_idx]

        x = trade_times[:idx]
        y = trade_prices[:idx]

        line.set_data(x, y)

        ax.set_xlim(0, x[-1])

        y_min = y.min()
        y_max = y.max()
        pad = (y_max - y_min) * 0.05 if y_max > y_min else 1
        ax.set_ylim(y_min - pad, y_max + pad)

        price_text.set_text(f"Price: ${y[-1]:,.2f}")

        return line, price_text

    ani = animation.FuncAnimation(
        fig,
        update,
        frames=len(frame_indices),
        interval=1000 / GIF_FPS,
        blit=False,
    )

    ani.save(OUT_GIF, writer="pillow", fps=GIF_FPS)
    plt.close(fig)

    print(f"Saved GIF: {OUT_GIF}")


def main():
    if not PERP_LOG.exists():
        print(f"Log not found: {PERP_LOG}", file=sys.stderr)
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
    fig.suptitle("ABC-PERP Random Walk Simulation",
                 fontsize=14, fontweight="bold")

    # Price evolution
    ax = axes[0, 0]
    ax.plot(trade_times, trade_prices,
            color="steelblue", linewidth=0.8)
    ax.axhline(bootstrap_price, color="red",
               linestyle="--", linewidth=1)
    ax.set_title("ABC-PERP Price Evolution")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(
        mticker.FuncFormatter(lambda x, _: f"${x:,.0f}")
    )
    ax.grid(True, alpha=0.3)

    # Spread
    ax = axes[0, 1]
    ax.plot(spread_times, spread_bps,
            color="green", linewidth=0.6)
    ax.set_title("Bid-Ask Spread Evolution")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Spread (bps)")
    ax.grid(True, alpha=0.3)

    # Trade volume
    ax = axes[1, 0]
    if len(trade_centers) > 0:
        ax.fill_between(trade_centers,
                        trade_counts,
                        alpha=0.6,
                        color="orange")
    ax.set_title("Trade Volume (rolling 10s)")
    ax.grid(True, alpha=0.3)

    # Returns
    ax = axes[1, 1]
    if len(returns) > 0:
        ax.hist(returns, bins=60,
                density=True,
                color="mediumpurple")
        ax.axvline(0, color="red",
                   linestyle="--")
    ax.set_title("Returns Distribution")
    ax.grid(True, alpha=0.3)

    # Rejections
    ax = axes[2, 0]
    if len(reject_centers) > 0:
        ax.fill_between(reject_centers,
                        reject_counts,
                        alpha=0.5,
                        color="red")
    ax.set_title("Order Rejection Rate")
    ax.grid(True, alpha=0.3)

    axes[2, 1].axis("off")

    fig.tight_layout()
    fig.savefig(OUT_PNG, dpi=150,
                bbox_inches="tight")
    print(f"Saved PNG: {OUT_PNG}")

    # Create animated GIF
    create_price_gif(trade_times,
                     trade_prices,
                     bootstrap_price)


if __name__ == "__main__":
    main()