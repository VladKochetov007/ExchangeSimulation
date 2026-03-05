#!/usr/bin/env python3
"""Plot price evolution and market statistics from a randomwalk simulation run."""

import json
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np

LOG_DIR = Path("logs/randomwalk")
PERP_LOG = LOG_DIR / "perp" / "ABC-PERP.jsonl"
SPOT_LOG = LOG_DIR / "spot" / "ABC-USD.jsonl"
OUT_PNG = LOG_DIR / "price_evolution.png"

USD_PRECISION = 100_000


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

    return {
        "trade_times": np.array(trade_times),
        "trade_prices": np.array(trade_prices),
        "spread_times": np.array(spread_times),
        "spread_bps": np.array(spread_bps),
        "reject_times": np.array(reject_times),
    }


def parse_funding(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    rate_times, rates = [], []
    settlement_times = []

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


def main():
    if not PERP_LOG.exists():
        print(f"Log not found: {PERP_LOG}\nRun 'make run-randomwalk' first.", file=sys.stderr)
        sys.exit(1)

    perp_events = load_events(PERP_LOG)
    if not perp_events:
        print("Perp log is empty.", file=sys.stderr)
        sys.exit(1)

    t0 = perp_events[0]["sim_ts"]

    spot_events = load_events(SPOT_LOG)

    perp = parse_instrument(perp_events, t0)
    spot = parse_instrument(spot_events, t0)
    rate_times, rates, settlement_times = parse_funding(perp_events, t0)

    bootstrap_price = to_usd(50_000 * USD_PRECISION)

    perp_centers, perp_counts = rolling_count(perp["trade_times"])
    spot_centers, spot_counts = rolling_count(spot["trade_times"])
    perp_returns = np.diff(perp["trade_prices"]) if len(perp["trade_prices"]) > 1 else np.array([])

    fig, axes = plt.subplots(3, 2, figsize=(14, 12))
    fig.suptitle("ABC Random Walk Simulation — Spot + Perp", fontsize=14, fontweight="bold")

    # 1. Price evolution: both instruments
    ax = axes[0, 0]
    ax.plot(perp["trade_times"], perp["trade_prices"], color="steelblue", linewidth=0.8, label="ABC-PERP")
    if len(spot["trade_prices"]) > 0:
        ax.plot(spot["trade_times"], spot["trade_prices"], color="darkorange", linewidth=1.2,
                marker="o", markersize=4, label="ABC-USD")
    ax.axhline(bootstrap_price, color="gray", linestyle="--", linewidth=0.8, label=f"Bootstrap (${bootstrap_price:,.0f})")
    for st in settlement_times:
        ax.axvline(st, color="purple", linestyle=":", linewidth=0.7, alpha=0.5)
    ax.set_title("Price Evolution (purple lines = funding settlements)")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(lambda x, _: f"${x:,.0f}"))
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # 2. Basis: perp mid – spot mid (interpolated to perp trade times)
    ax = axes[0, 1]
    if len(perp["trade_times"]) > 0 and len(spot["trade_times"]) > 0:
        spot_at_perp = np.interp(perp["trade_times"], spot["trade_times"], spot["trade_prices"],
                                 left=spot["trade_prices"][0], right=spot["trade_prices"][-1])
        basis = perp["trade_prices"] - spot_at_perp
        basis_bps = basis / spot_at_perp * 10_000
        ax.plot(perp["trade_times"], basis_bps, color="teal", linewidth=0.8)
        ax.axhline(0, color="gray", linestyle="--", linewidth=0.8)
        for st in settlement_times:
            ax.axvline(st, color="purple", linestyle=":", linewidth=0.7, alpha=0.5)
        ax.set_title("Basis: Perp − Spot (bps)")
        ax.set_ylabel("Basis (bps)")
    else:
        ax.set_title("Basis: insufficient spot data")
    ax.set_xlabel("Time (seconds)")
    ax.grid(True, alpha=0.3)

    # 3. Funding rate over time
    ax = axes[1, 0]
    if len(rate_times) > 0:
        ax.step(rate_times, rates, where="post", color="indigo", linewidth=1.2)
        ax.axhline(0, color="gray", linestyle="--", linewidth=0.8)
        for st in settlement_times:
            ax.axvline(st, color="purple", linestyle=":", linewidth=0.7, alpha=0.5, label="_nolegend_")
        ax.set_title(f"Funding Rate (bps) — {len(settlement_times)} settlements")
    else:
        ax.set_title("Funding Rate — no data")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Rate (bps)")
    ax.grid(True, alpha=0.3)

    # 4. Trade volume by instrument
    ax = axes[1, 1]
    if len(perp_centers) > 0:
        ax.fill_between(perp_centers, perp_counts, alpha=0.5, color="steelblue", label="ABC-PERP")
        ax.plot(perp_centers, perp_counts, color="steelblue", linewidth=0.8)
    if len(spot_centers) > 0:
        ax.fill_between(spot_centers, spot_counts, alpha=0.5, color="darkorange", label="ABC-USD")
        ax.plot(spot_centers, spot_counts, color="darkorange", linewidth=0.8)
    ax.set_title("Trade Volume (rolling 10s window)")
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Trades per 10s")
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # 5. Perp returns distribution
    ax = axes[2, 0]
    if len(perp_returns) > 0:
        ax.hist(perp_returns, bins=60, density=True, color="mediumpurple", edgecolor="none", alpha=0.85)
        ax.axvline(0, color="red", linestyle="--", linewidth=1)
        ax.set_title("ABC-PERP Returns Distribution")
        ax.set_xlabel("Price Change (USD)")
        ax.set_ylabel("Density")
        ax.grid(True, alpha=0.3)

    # 6. Summary stats
    ax = axes[2, 1]
    ax.axis("off")
    lines = []
    if len(perp["trade_prices"]) > 0:
        p = perp["trade_prices"]
        lines += [
            "ABC-PERP",
            f"  Trades:    {len(p):,}",
            f"  Start:     ${p[0]:,.2f}",
            f"  End:       ${p[-1]:,.2f}",
            f"  Drift:     ${p[-1] - p[0]:+,.2f}",
            f"  Range:     ${p.min():,.2f} – ${p.max():,.2f}",
        ]
        if len(perp["spread_bps"]) > 0:
            lines.append(f"  Avg spread:{perp['spread_bps'].mean():.1f} bps")
    if len(spot["trade_prices"]) > 0:
        s = spot["trade_prices"]
        lines += [
            "",
            "ABC-USD",
            f"  Trades:    {len(s):,}",
            f"  End:       ${s[-1]:,.2f}",
        ]
    if len(rates) > 0:
        lines += [
            "",
            "Funding",
            f"  Settlements: {len(settlement_times)}",
            f"  Rate range:  {rates.min()} – {rates.max()} bps",
            f"  Avg rate:    {rates.mean():.1f} bps",
        ]
    if lines:
        ax.text(0.05, 0.95, "\n".join(lines), transform=ax.transAxes,
                verticalalignment="top", fontfamily="monospace", fontsize=9)

    fig.tight_layout()
    fig.savefig(OUT_PNG, dpi=150, bbox_inches="tight")
    print(f"Saved: {OUT_PNG}")


if __name__ == "__main__":
    main()
