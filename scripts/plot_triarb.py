#!/usr/bin/env python3
"""Plot triangular arbitrage activity: cross pairs and price convergence."""

import json
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np

LOG_DIR = Path("logs/randomwalk")
OUT_PNG = LOG_DIR / "triarb.png"

BTC_PRECISION = 100_000_000
USD_PRECISION = 100_000

CROSS_PAIRS = [
    {"cross": "DEF-ABC", "base_usd": "DEF-USD", "base": "DEF"},
    {"cross": "GHI-ABC", "base_usd": "GHI-USD", "base": "GHI"},
]


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


def extract_trades(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray]:
    times, prices = [], []
    for ev in events:
        if ev["event"] == "Trade":
            times.append(to_seconds(ev["sim_ts"], t0))
            prices.append(ev["data"]["price"])
    return np.array(times), np.array(prices, dtype=float)


def extract_spreads(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    """Returns (times, bids, asks) from BookSnapshot events."""
    times, bids, asks = [], [], []
    for ev in events:
        if ev["event"] == "BookSnapshot":
            d = ev["data"]
            bs, as_ = d.get("bids") or [], d.get("asks") or []
            if bs and as_:
                times.append(to_seconds(ev["sim_ts"], t0))
                bids.append(bs[0]["price"])
                asks.append(as_[0]["price"])
    return np.array(times), np.array(bids, dtype=float), np.array(asks, dtype=float)


def interp_price(query_times: np.ndarray, ref_times: np.ndarray, ref_prices: np.ndarray) -> np.ndarray:
    if len(ref_times) == 0:
        return np.zeros(len(query_times))
    return np.interp(query_times, ref_times, ref_prices)


def format_usd(x, _):
    if abs(x) >= 1000:
        return f"${x:,.0f}"
    return f"${x:.2f}"


def format_abc(x, _):
    return f"{x/BTC_PRECISION:.5f}"


def main():
    # Find t0.
    t0 = None
    for asset in ["ABC", "DEF", "GHI"]:
        path = LOG_DIR / "spot" / f"{asset}-USD.jsonl"
        evs = load_events(path)
        if evs:
            t0 = evs[0]["sim_ts"]
            break
    if t0 is None:
        print("No spot logs found. Run the simulation first.", file=sys.stderr)
        sys.exit(1)

    # Load ABC-USD price series (common quote in the triangles).
    abc_times, abc_prices_raw = extract_trades(load_events(LOG_DIR / "spot" / "ABC-USD.jsonl"), t0)
    abc_prices_usd = abc_prices_raw / USD_PRECISION  # dollars

    # Build figure: 3 rows × 2 columns.
    #   Row 0: ABC-USD price | DEF-ABC price in native satoshi
    #   Row 1: DEF: direct vs implied (USD)  | GHI: direct vs implied (USD)
    #   Row 2: DEF arb spread (bps)          | GHI arb spread (bps)
    fig, axes = plt.subplots(3, 2, figsize=(14, 13))
    fig.suptitle("Triangular Arbitrage — Cross Pair Price Discovery", fontsize=13, fontweight="bold")

    # ── Row 0 left: ABC-USD price ──────────────────────────────────────────────
    ax = axes[0, 0]
    if len(abc_times) > 0:
        ax.plot(abc_times, abc_prices_usd, color="darkorange", linewidth=0.9, label="ABC-USD")
    ax.set_title("ABC-USD (shared quote leg of all triangles)")
    ax.set_xlabel("Time (s)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_usd))
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # ── Row 0 right: cross pair native prices ──────────────────────────────────
    ax = axes[0, 1]
    colors = ["steelblue", "mediumseagreen"]
    for i, pair in enumerate(CROSS_PAIRS):
        ct, cp = extract_trades(load_events(LOG_DIR / "spot" / f"{pair['cross']}.jsonl"), t0)
        if len(ct) > 0:
            ax.plot(ct, cp / BTC_PRECISION, "o-", color=colors[i], linewidth=0.9,
                    markersize=4, label=f"{pair['cross']} ({len(ct)} trades)")
    ax.set_title("Cross pair prices (native ABC units per base)")
    ax.set_xlabel("Time (s)")
    ax.set_ylabel("Price (ABC per base)")
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # ── Rows 1 & 2: per-triangle convergence and spread ────────────────────────
    for col, pair in enumerate(CROSS_PAIRS):
        cross_sym = pair["cross"]
        base_usd_sym = pair["base_usd"]
        base = pair["base"]

        # Load data.
        cross_times, cross_prices = extract_trades(load_events(LOG_DIR / "spot" / f"{cross_sym}.jsonl"), t0)
        base_times, base_prices = extract_trades(load_events(LOG_DIR / "spot" / f"{base_usd_sym}.jsonl"), t0)
        base_st, base_bids, base_asks = extract_spreads(load_events(LOG_DIR / "spot" / f"{base_usd_sym}.jsonl"), t0)

        # Implied USD price for base asset via the triangle:
        #   implied = (cross_price [satoshi ABC/DEF]) × (ABC-USD [USD/ABC]) / BTC_PRECISION
        #           = cross_price_int / BTC_PRECISION × abc_price_usd
        if len(cross_times) > 0 and len(abc_times) > 0:
            abc_at_cross = interp_price(cross_times, abc_times, abc_prices_usd)
            implied_usd = (cross_prices / BTC_PRECISION) * abc_at_cross
        else:
            implied_usd = np.array([])

        # ── Row 1: direct vs implied price ────────────────────────────────────
        ax = axes[1, col]
        if len(base_times) > 0:
            ax.plot(base_times, base_prices / USD_PRECISION, color="firebrick",
                    linewidth=0.9, alpha=0.9, label=f"{base_usd_sym} direct")
        if len(base_st) > 0:
            ax.fill_between(base_st, base_bids / USD_PRECISION, base_asks / USD_PRECISION,
                            alpha=0.15, color="firebrick", label="bid-ask band")
        if len(cross_times) > 0:
            ax.scatter(cross_times, implied_usd, color="steelblue", s=20, zorder=5,
                       label=f"implied via {cross_sym} ({len(cross_times)} trades)")
        ax.set_title(f"{base} — Direct USD vs Implied via {cross_sym}")
        ax.set_xlabel("Time (s)")
        ax.set_ylabel("Price (USD)")
        ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_usd))
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

        # Summary stats in text box.
        lines = []
        if len(base_times) > 0:
            p = base_prices / USD_PRECISION
            lines.append(f"{base_usd_sym}: {len(p):,} trades, ${p[0]:,.0f}→${p[-1]:,.0f}")
        if len(cross_times) > 0:
            lines.append(f"{cross_sym}: {len(cross_times)} arb trades")
        ax.text(0.02, 0.97, "\n".join(lines), transform=ax.transAxes,
                verticalalignment="top", fontsize=7.5, fontfamily="monospace",
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

        # ── Row 2: spread between implied and direct (bps) ────────────────────
        ax = axes[2, col]
        if len(cross_times) > 0 and len(base_times) > 0:
            direct_at_cross = interp_price(cross_times, base_times, base_prices / USD_PRECISION)
            spread_bps = (implied_usd - direct_at_cross) / direct_at_cross * 10_000
            ax.scatter(cross_times, spread_bps, color="indigo", s=20, zorder=5,
                       label=f"implied − direct ({len(cross_times)} pts)")
            ax.axhline(0, color="gray", linestyle="--", linewidth=0.8)
            ax.axhline(1, color="green", linestyle=":", linewidth=0.8, alpha=0.6, label="MinProfit 1 bps")
            ax.axhline(-1, color="red", linestyle=":", linewidth=0.8, alpha=0.6)
            ax.set_ylim(-50, 50)

            # Annotate mean/std.
            valid = spread_bps[np.isfinite(spread_bps)]
            if len(valid) > 0:
                ax.text(0.02, 0.97,
                        f"mean {valid.mean():.1f} bps\nstd {valid.std():.1f} bps",
                        transform=ax.transAxes, verticalalignment="top",
                        fontsize=8, fontfamily="monospace",
                        bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))
        ax.set_title(f"{base} Arb Spread: implied − direct (bps)")
        ax.set_xlabel("Time (s)")
        ax.set_ylabel("Spread (bps)")
        ax.legend(fontsize=8)
        ax.grid(True, alpha=0.3)

    fig.tight_layout()
    fig.savefig(OUT_PNG, dpi=150, bbox_inches="tight")
    print(f"Saved: {OUT_PNG}")


if __name__ == "__main__":
    main()
