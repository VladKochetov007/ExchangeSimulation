#!/usr/bin/env python3
"""Visualize the fee-aware multi-market simulation (feesim).

Layout: 3x2 grid
  Row 0: ABC prices (spot vs perp) + basis  |  Q prices (spot vs perp) + basis
  Row 1: Q/ABC cross price + implied price  |  Triangle arb spread (bps)
  Row 2: Funding rates (ABC-PERP, Q-PERP)   |  Cumulative realized PnL by actor type
"""

import json
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np

LOG_DIR = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("logs/feesim")
OUT_PNG = LOG_DIR / "feesim_analysis.png"

USD_PRECISION = 100_000
BTC_PRECISION = 100_000_000


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


def to_abc(price_units: int) -> float:
    return price_units / BTC_PRECISION


def parse_trades(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray]:
    times, prices = [], []
    for ev in events:
        if ev["event"] == "Trade":
            times.append(to_seconds(ev["sim_ts"], t0))
            prices.append(ev["data"]["price"])
    return np.array(times), np.array(prices)


def parse_spreads(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray]:
    times, spreads = [], []
    for ev in events:
        if ev["event"] == "BookSnapshot":
            bids = ev["data"].get("bids") or []
            asks = ev["data"].get("asks") or []
            if bids and asks:
                bid, ask = bids[0]["price"], asks[0]["price"]
                mid = (bid + ask) / 2
                if mid > 0:
                    times.append(to_seconds(ev["sim_ts"], t0))
                    spreads.append((ask - bid) / mid * 10_000)
    return np.array(times), np.array(spreads)


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
            settlement_times.append(t)
    return np.array(rate_times), np.array(rates), np.array(sorted(set(settlement_times)))


def parse_realized_pnl(events: list[dict], t0: int) -> tuple[np.ndarray, np.ndarray]:
    """Parse realized_pnl events from global log. Returns (times, pnl_amounts)."""
    times, pnls = [], []
    for ev in events:
        if ev["event"] == "realized_pnl":
            t = to_seconds(ev["sim_ts"], t0)
            pnl = ev["data"].get("pnl", 0)
            if pnl != 0:
                times.append(t)
                pnls.append(to_usd(pnl))
    return np.array(times), np.array(pnls)


def format_price(x, _):
    if abs(x) >= 1000:
        return f"${x:,.0f}"
    return f"${x:.2f}"


def main():
    # Find t0
    t0 = None
    for name in ["ABC-PERP", "Q-PERP"]:
        path = LOG_DIR / "perp" / f"{name}.jsonl"
        events = load_events(path)
        if events:
            t0 = events[0]["sim_ts"]
            break
    if t0 is None:
        print("No logs found. Run feesim first.", file=sys.stderr)
        sys.exit(1)

    # Load per-symbol data
    abc_perp_ev = load_events(LOG_DIR / "perp" / "ABC-PERP.jsonl")
    abc_spot_ev = load_events(LOG_DIR / "spot" / "ABC-USD.jsonl")
    q_perp_ev = load_events(LOG_DIR / "perp" / "Q-PERP.jsonl")
    q_spot_ev = load_events(LOG_DIR / "spot" / "Q-USD.jsonl")
    qabc_ev = load_events(LOG_DIR / "spot" / "Q-ABC.jsonl")
    global_ev = load_events(LOG_DIR / "general.jsonl")

    abc_perp_t, abc_perp_p = parse_trades(abc_perp_ev, t0)
    abc_spot_t, abc_spot_p = parse_trades(abc_spot_ev, t0)
    q_perp_t, q_perp_p = parse_trades(q_perp_ev, t0)
    q_spot_t, q_spot_p = parse_trades(q_spot_ev, t0)
    qabc_t, qabc_p = parse_trades(qabc_ev, t0)

    fig, axes = plt.subplots(3, 2, figsize=(16, 14))
    title = f"Fee-Aware Multi-Market Simulation ({LOG_DIR.name})"
    fig.suptitle(title, fontsize=14, fontweight="bold")

    # ---- Row 0 Left: ABC prices (spot vs perp) ----
    ax = axes[0, 0]
    if len(abc_perp_p) > 0:
        ax.plot(abc_perp_t, abc_perp_p / USD_PRECISION, color="steelblue", lw=0.8, label="ABC-PERP")
    if len(abc_spot_p) > 0:
        ax.plot(abc_spot_t, abc_spot_p / USD_PRECISION, color="darkorange", lw=0.8, alpha=0.85, label="ABC/USD")
    ax.set_title("ABC Prices (spot vs perp)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_price))
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # Stats annotation
    lines = []
    if len(abc_perp_p) > 0:
        p = abc_perp_p / USD_PRECISION
        lines.append(f"ABC-PERP: {len(p):,} trades")
        lines.append(f"  range ${p.min():,.0f}-${p.max():,.0f}")
    if len(abc_spot_p) > 0:
        s = abc_spot_p / USD_PRECISION
        lines.append(f"ABC/USD: {len(s):,} trades")
    if lines:
        ax.text(0.02, 0.97, "\n".join(lines), transform=ax.transAxes,
                va="top", fontfamily="monospace", fontsize=7,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

    # ---- Row 0 Right: Q prices (spot vs perp) ----
    ax = axes[0, 1]
    if len(q_perp_p) > 0:
        ax.plot(q_perp_t, q_perp_p / USD_PRECISION, color="mediumseagreen", lw=0.8, label="Q-PERP")
    if len(q_spot_p) > 0:
        ax.plot(q_spot_t, q_spot_p / USD_PRECISION, color="firebrick", lw=0.8, alpha=0.85, label="Q/USD")
    ax.set_title("Q Prices (spot vs perp)")
    ax.set_ylabel("Price (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_price))
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    lines = []
    if len(q_perp_p) > 0:
        p = q_perp_p / USD_PRECISION
        lines.append(f"Q-PERP: {len(p):,} trades, range ${p.min():,.0f}-${p.max():,.0f}")
    if len(q_spot_p) > 0:
        s = q_spot_p / USD_PRECISION
        lines.append(f"Q/USD: {len(s):,} trades")
    if lines:
        ax.text(0.02, 0.97, "\n".join(lines), transform=ax.transAxes,
                va="top", fontfamily="monospace", fontsize=7,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

    # ---- Row 1 Left: Q/ABC cross price + implied ----
    ax = axes[1, 0]
    if len(qabc_p) > 0:
        ax.plot(qabc_t, qabc_p / BTC_PRECISION, color="mediumpurple", lw=0.8, label="Q/ABC (direct)")

    # Calculate implied Q/ABC = Q_USD / ABC_USD at each Q/ABC trade time
    if len(qabc_t) > 0 and len(q_spot_p) > 0 and len(abc_spot_p) > 0:
        implied_times, implied_prices = [], []
        q_idx, abc_idx = 0, 0
        last_q, last_abc = 0.0, 0.0
        for i, t in enumerate(qabc_t):
            while q_idx < len(q_spot_t) and q_spot_t[q_idx] <= t:
                last_q = q_spot_p[q_idx]
                q_idx += 1
            while abc_idx < len(abc_spot_t) and abc_spot_t[abc_idx] <= t:
                last_abc = abc_spot_p[abc_idx]
                abc_idx += 1
            if last_q > 0 and last_abc > 0:
                implied_times.append(t)
                implied_prices.append(last_q / last_abc)
        if implied_times:
            ax.plot(implied_times, implied_prices, color="darkcyan", lw=0.6, alpha=0.7,
                    label="Q/ABC (implied from USD pairs)")

    ax.set_title("Q/ABC Cross Price")
    ax.set_ylabel("Price (ABC)")
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    if len(qabc_p) > 0:
        p = qabc_p / BTC_PRECISION
        ax.text(0.02, 0.97, f"Q/ABC: {len(p):,} trades\nrange {p.min():.6f}-{p.max():.6f}",
                transform=ax.transAxes, va="top", fontfamily="monospace", fontsize=7,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

    # ---- Row 1 Right: Triangle arb spread ----
    ax = axes[1, 1]
    if len(qabc_p) > 0 and len(q_spot_p) > 0 and len(abc_spot_p) > 0:
        spread_times, spread_bps = [], []
        q_idx, abc_idx = 0, 0
        last_q, last_abc = 0.0, 0.0
        for i, t in enumerate(qabc_t):
            while q_idx < len(q_spot_t) and q_spot_t[q_idx] <= t:
                last_q = q_spot_p[q_idx]
                q_idx += 1
            while abc_idx < len(abc_spot_t) and abc_spot_t[abc_idx] <= t:
                last_abc = abc_spot_p[abc_idx]
                abc_idx += 1
            if last_q > 0 and last_abc > 0:
                direct = qabc_p[i] / BTC_PRECISION  # Q/ABC direct
                implied = last_q / last_abc          # Q/ABC implied
                if implied > 0:
                    spread = (direct - implied) / implied * 10_000
                    spread_times.append(t)
                    spread_bps.append(spread)
        if spread_times:
            ax.plot(spread_times, spread_bps, color="teal", lw=0.5, alpha=0.6)
            ax.axhline(0, color="gray", ls="--", lw=0.8)
            # Fee threshold lines (3 legs × 7.5 bps = 22.5 bps)
            ax.axhline(22.5, color="red", ls=":", lw=0.8, alpha=0.5, label="Fee threshold (22.5 bps)")
            ax.axhline(-22.5, color="red", ls=":", lw=0.8, alpha=0.5)
            ax.text(0.02, 0.97,
                    f"Mean: {np.mean(spread_bps):.1f} bps\nStd: {np.std(spread_bps):.1f} bps",
                    transform=ax.transAxes, va="top", fontfamily="monospace", fontsize=7,
                    bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))
    ax.set_title("Triangle Arb Spread (direct vs implied, bps)")
    ax.set_ylabel("Spread (bps)")
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # ---- Row 2 Left: Funding rates ----
    ax = axes[2, 0]
    abc_rt, abc_rates, abc_settlements = parse_funding(abc_perp_ev, t0)
    q_rt, q_rates, q_settlements = parse_funding(q_perp_ev, t0)
    if len(abc_rt) > 0:
        ax.step(abc_rt, abc_rates, where="post", color="steelblue", lw=1.0,
                label=f"ABC-PERP ({len(abc_settlements)} settlements)")
    if len(q_rt) > 0:
        ax.step(q_rt, q_rates, where="post", color="mediumseagreen", lw=1.0,
                label=f"Q-PERP ({len(q_settlements)} settlements)")
    ax.axhline(0, color="gray", ls="--", lw=0.8)
    for st in abc_settlements:
        ax.axvline(st, color="purple", ls=":", lw=0.4, alpha=0.3)
    ax.set_title("Funding Rates (bps)")
    ax.set_xlabel("Time (s)")
    ax.set_ylabel("Rate (bps)")
    ax.legend(fontsize=8)
    ax.grid(True, alpha=0.3)

    # Stats
    lines = []
    if len(abc_rates) > 0:
        lines.append(f"ABC: avg {abc_rates.mean():.1f}, range [{abc_rates.min()}, {abc_rates.max()}]")
    if len(q_rates) > 0:
        lines.append(f"Q: avg {q_rates.mean():.1f}, range [{q_rates.min()}, {q_rates.max()}]")
    if lines:
        ax.text(0.02, 0.97, "\n".join(lines), transform=ax.transAxes,
                va="top", fontfamily="monospace", fontsize=7,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))

    # ---- Row 2 Right: Cumulative realized PnL ----
    ax = axes[2, 1]
    pnl_t, pnl_v = parse_realized_pnl(global_ev, t0)
    if len(pnl_t) > 0:
        cum_pnl = np.cumsum(pnl_v)
        ax.plot(pnl_t, cum_pnl, color="darkgreen", lw=1.0)
        ax.axhline(0, color="gray", ls="--", lw=0.8)
        ax.text(0.02, 0.97,
                f"Total realized PnL: ${cum_pnl[-1]:,.2f}\n{len(pnl_v):,} PnL events",
                transform=ax.transAxes, va="top", fontfamily="monospace", fontsize=7,
                bbox=dict(boxstyle="round,pad=0.3", facecolor="lightyellow", alpha=0.8))
    ax.set_title("Cumulative Realized PnL (all clients)")
    ax.set_xlabel("Time (s)")
    ax.set_ylabel("PnL (USD)")
    ax.yaxis.set_major_formatter(mticker.FuncFormatter(format_price))
    ax.grid(True, alpha=0.3)

    # Final layout
    for row in range(3):
        for col in range(2):
            axes[row, col].set_xlabel("Time (s)")

    fig.tight_layout()
    LOG_DIR.mkdir(parents=True, exist_ok=True)
    fig.savefig(OUT_PNG, dpi=150, bbox_inches="tight")
    print(f"Saved: {OUT_PNG}")


if __name__ == "__main__":
    main()
