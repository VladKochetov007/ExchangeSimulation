#!/usr/bin/env python3
"""
Visualize orderbook data from exchange simulation.

Shows best bid/ask spread over time with trades overlaid as colored dots.
Trade size (volume) is represented by dot size.
"""

import argparse
import sys
from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import polars as pl


def parse_args():
    parser = argparse.ArgumentParser(
        description="Visualize orderbook and trade data"
    )
    parser.add_argument(
        "--trades",
        type=Path,
        required=True,
        help="Path to trades CSV file",
    )
    parser.add_argument(
        "--orderbook",
        type=Path,
        required=True,
        help="Path to orderbook observable CSV file",
    )
    parser.add_argument(
        "--symbol",
        type=str,
        default="BTCUSDT",
        help="Trading symbol to visualize (default: BTCUSDT)",
    )
    parser.add_argument(
        "--time-unit",
        type=str,
        choices=["ns", "us", "ms", "s"],
        default="ns",
        help="Time unit in input data (default: ns)",
    )
    parser.add_argument(
        "--bin-width",
        type=float,
        default=1.0,
        help="Time bin width in seconds for discretization (default: 1.0)",
    )
    parser.add_argument(
        "--price-decimals",
        type=int,
        default=2,
        help="Number of decimal places for price display (default: 2)",
    )
    parser.add_argument(
        "--output",
        type=Path,
        help="Save plot to file instead of showing",
    )
    return parser.parse_args()


def load_trades(trades_path: Path, symbol: str, time_unit: str) -> pl.DataFrame:
    """Load and process trades data."""
    df = pl.read_csv(trades_path)

    df = df.filter(pl.col("symbol") == symbol)

    time_col = pl.col("timestamp")
    if time_unit == "ns":
        time_col = time_col / 1e9
    elif time_unit == "us":
        time_col = time_col / 1e6
    elif time_unit == "ms":
        time_col = time_col / 1e3

    df = df.with_columns(time_col.alias("time_seconds"))

    return df


def load_orderbook(
    orderbook_path: Path, symbol: str, time_unit: str
) -> pl.DataFrame:
    """Load and process orderbook data."""
    df = pl.read_csv(orderbook_path)

    df = df.filter(pl.col("symbol") == symbol)

    time_col = pl.col("timestamp")
    if time_unit == "ns":
        time_col = time_col / 1e9
    elif time_unit == "us":
        time_col = time_col / 1e6
    elif time_unit == "ms":
        time_col = time_col / 1e3

    df = df.with_columns(time_col.alias("time_seconds"))

    return df


def compute_best_bid_ask(ob_df: pl.DataFrame, bin_width: float) -> tuple:
    """
    Compute best bid and ask prices over time bins.

    Returns (time_bins, best_bids, best_asks)
    """
    if ob_df.height == 0:
        return np.array([]), np.array([]), np.array([])

    min_time = ob_df["time_seconds"].min()
    max_time = ob_df["time_seconds"].max()

    if min_time is None or max_time is None:
        return np.array([]), np.array([]), np.array([])

    n_bins = int(np.ceil((max_time - min_time) / bin_width)) + 1
    time_bins = np.linspace(min_time, min_time + n_bins * bin_width, n_bins)

    ob_df = ob_df.with_columns(
        ((pl.col("time_seconds") - min_time) / bin_width)
        .floor()
        .cast(pl.Int64)
        .alias("bin")
    )

    bids = (
        ob_df.filter(pl.col("side") == "bid")
        .group_by("bin")
        .agg(pl.col("price").max().alias("best_bid"))
        .sort("bin")
    )

    asks = (
        ob_df.filter(pl.col("side") == "ask")
        .group_by("bin")
        .agg(pl.col("price").min().alias("best_ask"))
        .sort("bin")
    )

    best_bids = np.full(n_bins, np.nan)
    best_asks = np.full(n_bins, np.nan)

    for row in bids.iter_rows(named=True):
        bin_idx = row["bin"]
        if 0 <= bin_idx < n_bins:
            best_bids[bin_idx] = row["best_bid"]

    for row in asks.iter_rows(named=True):
        bin_idx = row["bin"]
        if 0 <= bin_idx < n_bins:
            best_asks[bin_idx] = row["best_ask"]

    for i in range(1, n_bins):
        if np.isnan(best_bids[i]) and not np.isnan(best_bids[i - 1]):
            best_bids[i] = best_bids[i - 1]
        if np.isnan(best_asks[i]) and not np.isnan(best_asks[i - 1]):
            best_asks[i] = best_asks[i - 1]

    return time_bins, best_bids, best_asks


def visualize(
    trades_df: pl.DataFrame,
    ob_df: pl.DataFrame,
    bin_width: float,
    price_decimals: int,
    output_path: Path | None,
):
    """Create visualization of orderbook and trades."""
    fig, ax = plt.subplots(figsize=(14, 8))

    time_bins, best_bids, best_asks = compute_best_bid_ask(ob_df, bin_width)

    if len(time_bins) > 0:
        ax.plot(
            time_bins,
            best_bids,
            label="Best Bid",
            color="#2ecc71",
            linewidth=2,
            alpha=0.8,
        )
        ax.plot(
            time_bins,
            best_asks,
            label="Best Ask",
            color="#e74c3c",
            linewidth=2,
            alpha=0.8,
        )

        ax.fill_between(
            time_bins,
            best_bids,
            best_asks,
            alpha=0.2,
            color="gray",
            label="Spread",
        )

    if trades_df.height > 0:
        buys = trades_df.filter(pl.col("side") == "buy")
        sells = trades_df.filter(pl.col("side") == "sell")

        if buys.height > 0:
            buy_sizes = (buys["qty"].to_numpy() / buys["qty"].max()) * 200
            ax.scatter(
                buys["time_seconds"],
                buys["price"],
                s=buy_sizes,
                c="#27ae60",
                alpha=0.6,
                marker="o",
                edgecolors="darkgreen",
                linewidths=0.5,
                label="Buy Trades",
            )

        if sells.height > 0:
            sell_sizes = (sells["qty"].to_numpy() / sells["qty"].max()) * 200
            ax.scatter(
                sells["time_seconds"],
                sells["price"],
                s=sell_sizes,
                c="#c0392b",
                alpha=0.6,
                marker="o",
                edgecolors="darkred",
                linewidths=0.5,
                label="Sell Trades",
            )

    ax.set_xlabel("Time (seconds)", fontsize=12)
    ax.set_ylabel(f"Price (decimals: {price_decimals})", fontsize=12)
    ax.set_title("Order Book Dynamics and Trade Execution", fontsize=14, fontweight="bold")
    ax.legend(loc="best", framealpha=0.9)
    ax.grid(True, alpha=0.3, linestyle="--")

    plt.tight_layout()

    if output_path:
        plt.savefig(output_path, dpi=300, bbox_inches="tight")
        print(f"Plot saved to {output_path}")
    else:
        plt.show()


def main():
    args = parse_args()

    if not args.trades.exists():
        print(f"Error: trades file not found: {args.trades}", file=sys.stderr)
        sys.exit(1)

    if not args.orderbook.exists():
        print(f"Error: orderbook file not found: {args.orderbook}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading trades from {args.trades}...")
    trades_df = load_trades(args.trades, args.symbol, args.time_unit)
    print(f"  Loaded {trades_df.height} trades")

    print(f"Loading orderbook from {args.orderbook}...")
    ob_df = load_orderbook(args.orderbook, args.symbol, args.time_unit)
    print(f"  Loaded {ob_df.height} orderbook updates")

    print("Creating visualization...")
    visualize(trades_df, ob_df, args.bin_width, args.price_decimals, args.output)

    print("Done!")


if __name__ == "__main__":
    main()
