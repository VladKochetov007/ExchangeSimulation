#!/usr/bin/env python3
"""
Comprehensive analysis script for microstructure_v1 simulation.
Analyzes all 14 instruments and generates summary plots.

Usage:
    python scripts/analyze_microstructure_v1.py
"""

import pandas as pd
import matplotlib.pyplot as plt
from pathlib import Path
import json
import numpy as np
from collections import defaultdict

import sys as _sys

LOG_DIR = Path(_sys.argv[1]) if len(_sys.argv) > 1 else Path("logs/microstructure_v1")
OUTPUT_DIR = LOG_DIR / "plots"
USD_PRECISION = 100_000
ASSET_PRECISION = 100_000_000

# Instrument groups
USD_SPOT = ["ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"]
ABC_SPOT = ["BCD/ABC", "CDE/ABC", "DEF/ABC", "EFG/ABC"]
PERPS = ["ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"]
ALL_INSTRUMENTS = USD_SPOT + ABC_SPOT + PERPS

# Bootstrap prices for reference
BOOTSTRAP_PRICES = {
    "ABC/USD": 50000, "BCD/USD": 25000, "CDE/USD": 10000,
    "DEF/USD": 5000, "EFG/USD": 1000,
    "ABC-PERP": 50000, "BCD-PERP": 25000, "CDE-PERP": 10000,
    "DEF-PERP": 5000, "EFG-PERP": 1000,
    "BCD/ABC": 0.5, "CDE/ABC": 0.2, "DEF/ABC": 0.1, "EFG/ABC": 0.02
}


def get_log_path(symbol):
    """Get the log file path for a given symbol."""
    safe_symbol = symbol.replace("/", "-")
    if symbol in PERPS:
        return LOG_DIR / "perp" / f"{safe_symbol}.log"
    else:
        return LOG_DIR / "spot" / f"{safe_symbol}.log"


def load_instrument_data(symbol):
    """Load and parse log data for an instrument."""
    log_path = get_log_path(symbol)
    if not log_path.exists():
        print(f"⚠️  Log file not found: {log_path}")
        return None

    with open(log_path) as f:
        data = [json.loads(line) for line in f]

    return pd.DataFrame(data)


def extract_metrics(df, symbol):
    """Extract key metrics from instrument data."""
    if df is None or len(df) == 0:
        return None

    start_time = df["sim_time"].min()

    # Extract trades
    trades = df[df["event"] == "Trade"].copy()
    if len(trades) == 0:
        return None

    trades["time_sec"] = (trades["sim_time"] - start_time) / 1e9

    # Determine precision based on symbol type
    if "/" in symbol and "USD" in symbol:
        precision = USD_PRECISION
    elif "/" in symbol and "ABC" in symbol:
        precision = ASSET_PRECISION  # ABC is the quote
    else:  # PERP
        precision = USD_PRECISION

    trades["price_normalized"] = trades["price"] / precision
    trades = trades.sort_values("time_sec")

    # Extract rejections
    rejections = df[df["event"] == "OrderRejected"].copy()
    if len(rejections) > 0:
        rejections["time_sec"] = (rejections["sim_time"] - start_time) / 1e9

    # Extract book snapshots
    snapshots = df[df["event"] == "BookSnapshot"].copy()
    if len(snapshots) > 0:
        snapshots["time_sec"] = (snapshots["sim_time"] - start_time) / 1e9

        def get_best_bid(bids):
            if isinstance(bids, list) and len(bids) > 0:
                return bids[0].get("price", 0)
            return 0

        def get_best_ask(asks):
            if isinstance(asks, list) and len(asks) > 0:
                return asks[0].get("price", 0)
            return 0

        snapshots["best_bid"] = snapshots["bids"].apply(get_best_bid)
        snapshots["best_ask"] = snapshots["asks"].apply(get_best_ask)
        snapshots = snapshots[(snapshots["best_bid"] > 0) & (snapshots["best_ask"] > 0)].copy()

        if len(snapshots) > 0:
            snapshots["spread"] = (snapshots["best_ask"] - snapshots["best_bid"]) / precision
            snapshots["spread_bps"] = ((snapshots["best_ask"] - snapshots["best_bid"]) / snapshots["best_bid"]) * 10000
            snapshots = snapshots.sort_values("time_sec")

    return {
        "symbol": symbol,
        "trades": trades,
        "rejections": rejections,
        "snapshots": snapshots,
        "start_time": start_time,
        "precision": precision
    }


def plot_instrument(metrics, output_path):
    """Create comprehensive plot for a single instrument."""
    if metrics is None:
        return

    trades = metrics["trades"]
    rejections = metrics["rejections"]
    snapshots = metrics["snapshots"]
    symbol = metrics["symbol"]

    fig, axes = plt.subplots(3, 2, figsize=(16, 14))
    fig.suptitle(f'{symbol} - Microstructure Analysis', fontsize=16, fontweight='bold')

    # Panel 1: Price evolution
    ax1 = axes[0, 0]
    ax1.plot(trades["time_sec"], trades["price_normalized"], linewidth=1, alpha=0.8, color='blue')
    if symbol in BOOTSTRAP_PRICES:
        ax1.axhline(y=BOOTSTRAP_PRICES[symbol], color='red', linestyle='--',
                   alpha=0.5, label=f'Bootstrap (${BOOTSTRAP_PRICES[symbol]:.2f})')
    ax1.set_xlabel('Time (seconds)', fontsize=11)
    ax1.set_ylabel('Price', fontsize=11)
    ax1.set_title('Price Evolution', fontsize=13, fontweight='bold')
    ax1.legend()
    ax1.grid(True, alpha=0.3)

    # Panel 2: Spread evolution
    ax2 = axes[0, 1]
    if len(snapshots) > 0:
        ax2.plot(snapshots["time_sec"], snapshots["spread_bps"], linewidth=1, alpha=0.8, color='green')
        ax2.set_xlabel('Time (seconds)', fontsize=11)
        ax2.set_ylabel('Spread (bps)', fontsize=11)
        ax2.set_title('Bid-Ask Spread Evolution', fontsize=13, fontweight='bold')
        ax2.grid(True, alpha=0.3)
        ax2.set_ylim(bottom=0)
    else:
        ax2.text(0.5, 0.5, 'No snapshot data', ha='center', va='center', transform=ax2.transAxes)
        ax2.set_title('Bid-Ask Spread Evolution', fontsize=13, fontweight='bold')

    # Panel 3: Trade volume (rolling 10s)
    ax3 = axes[1, 0]
    rolling_window = 10
    trades_sorted = trades.sort_values("time_sec")
    rolling_counts = []
    times_for_rolling = []

    max_time = trades_sorted["time_sec"].max()
    if max_time > rolling_window:
        for t in np.arange(rolling_window, max_time, 1):
            count = len(trades_sorted[(trades_sorted["time_sec"] >= t - rolling_window) &
                                     (trades_sorted["time_sec"] < t)])
            rolling_counts.append(count)
            times_for_rolling.append(t)

        ax3.plot(times_for_rolling, rolling_counts, linewidth=1.5, alpha=0.9, color='orange')
        ax3.fill_between(times_for_rolling, rolling_counts, alpha=0.3, color='orange')

    ax3.set_xlabel('Time (seconds)', fontsize=11)
    ax3.set_ylabel('Trades per 10s', fontsize=11)
    ax3.set_title('Trade Volume (rolling 10s)', fontsize=13, fontweight='bold')
    ax3.grid(True, alpha=0.3)

    # Panel 4: Returns distribution
    ax4 = axes[1, 1]
    returns = trades['price_normalized'].diff().dropna()
    if len(returns) > 0:
        ax4.hist(returns, bins=50, alpha=0.7, color='purple', edgecolor='black', density=True)
        ax4.axvline(x=0, color='red', linestyle='--', alpha=0.5, label='Zero return')
        ax4.set_xlabel('Price Change', fontsize=11)
        ax4.set_ylabel('Density', fontsize=11)
        ax4.set_title('Returns Distribution', fontsize=13, fontweight='bold')
        ax4.legend()
        ax4.grid(True, alpha=0.3)

    # Panel 5: Order rejections
    ax5 = axes[2, 0]
    if len(rejections) > 0:
        rejection_counts = []
        times_for_rolling = []
        max_time = rejections["time_sec"].max()

        for t in np.arange(rolling_window, max_time, 1):
            count = len(rejections[(rejections["time_sec"] >= t - rolling_window) &
                                  (rejections["time_sec"] < t)])
            rejection_counts.append(count)
            times_for_rolling.append(t)

        if len(rejection_counts) > 0:
            ax5.plot(times_for_rolling, rejection_counts, linewidth=1.5, alpha=0.9, color='red')
            ax5.fill_between(times_for_rolling, rejection_counts, alpha=0.3, color='red')

    ax5.set_xlabel('Time (seconds)', fontsize=11)
    ax5.set_ylabel('Rejections per 10s', fontsize=11)
    ax5.set_title('Order Rejection Rate (rolling 10s)', fontsize=13, fontweight='bold')
    ax5.grid(True, alpha=0.3)

    # Panel 6: Summary statistics
    ax6 = axes[2, 1]
    ax6.axis('off')

    stats_text = f"""
    SUMMARY STATISTICS

    Total Trades: {len(trades):,}
    Duration: {trades['time_sec'].max():.1f}s

    Price Range: {trades['price_normalized'].min():.4f} - {trades['price_normalized'].max():.4f}
    Mean Price: {trades['price_normalized'].mean():.4f}
    Std Dev: {trades['price_normalized'].std():.4f}
    """

    if len(snapshots) > 0:
        stats_text += f"""
    Mean Spread: {snapshots['spread_bps'].mean():.2f} bps
    Min Spread: {snapshots['spread_bps'].min():.2f} bps
    Max Spread: {snapshots['spread_bps'].max():.2f} bps
    """

    if len(rejections) > 0:
        stats_text += f"""
    Total Rejections: {len(rejections):,}
    """

    ax6.text(0.1, 0.5, stats_text, transform=ax6.transAxes,
            fontsize=11, verticalalignment='center', fontfamily='monospace',
            bbox=dict(boxstyle='round', facecolor='wheat', alpha=0.3))

    plt.tight_layout()
    fig.savefig(output_path, dpi=150, bbox_inches='tight')
    plt.close(fig)
    print(f"✓ Saved: {output_path}")


def create_summary_plot(all_metrics, output_path):
    """Create cross-instrument summary plot."""
    fig, axes = plt.subplots(2, 2, figsize=(16, 12))
    fig.suptitle('Microstructure V1 - Cross-Instrument Summary', fontsize=16, fontweight='bold')

    # Panel 1: Trade counts by instrument
    ax1 = axes[0, 0]
    instruments = []
    trade_counts = []
    for m in all_metrics:
        if m is not None:
            instruments.append(m["symbol"])
            trade_counts.append(len(m["trades"]))

    colors = ['blue']*len(USD_SPOT) + ['green']*len(ABC_SPOT) + ['red']*len(PERPS)
    ax1.barh(instruments, trade_counts, color=colors[:len(instruments)], alpha=0.7)
    ax1.set_xlabel('Total Trades', fontsize=12)
    ax1.set_title('Trade Volume by Instrument', fontsize=13, fontweight='bold')
    ax1.grid(True, alpha=0.3, axis='x')

    # Panel 2: Mean spreads by instrument
    ax2 = axes[0, 1]
    instruments = []
    mean_spreads = []
    for m in all_metrics:
        if m is not None and len(m["snapshots"]) > 0:
            instruments.append(m["symbol"])
            mean_spreads.append(m["snapshots"]["spread_bps"].mean())

    colors = []
    for inst in instruments:
        if inst in USD_SPOT:
            colors.append('blue')
        elif inst in ABC_SPOT:
            colors.append('green')
        else:
            colors.append('red')

    ax2.barh(instruments, mean_spreads, color=colors, alpha=0.7)
    ax2.set_xlabel('Mean Spread (bps)', fontsize=12)
    ax2.set_title('Average Bid-Ask Spread by Instrument', fontsize=13, fontweight='bold')
    ax2.grid(True, alpha=0.3, axis='x')

    # Panel 3: Price volatility (std dev of returns)
    ax3 = axes[1, 0]
    instruments = []
    volatilities = []
    for m in all_metrics:
        if m is not None:
            returns = m["trades"]["price_normalized"].diff().dropna()
            if len(returns) > 0:
                instruments.append(m["symbol"])
                volatilities.append(returns.std())

    colors = []
    for inst in instruments:
        if inst in USD_SPOT:
            colors.append('blue')
        elif inst in ABC_SPOT:
            colors.append('green')
        else:
            colors.append('red')

    ax3.barh(instruments, volatilities, color=colors, alpha=0.7)
    ax3.set_xlabel('Std Dev of Returns', fontsize=12)
    ax3.set_title('Price Volatility by Instrument', fontsize=13, fontweight='bold')
    ax3.grid(True, alpha=0.3, axis='x')

    # Panel 4: Legend and summary
    ax4 = axes[1, 1]
    ax4.axis('off')

    legend_elements = [
        plt.Rectangle((0,0),1,1, fc='blue', alpha=0.7, label='USD-quoted Spot'),
        plt.Rectangle((0,0),1,1, fc='green', alpha=0.7, label='ABC-quoted Spot'),
        plt.Rectangle((0,0),1,1, fc='red', alpha=0.7, label='Perpetuals')
    ]
    ax4.legend(handles=legend_elements, loc='center', fontsize=12)

    total_trades = sum(len(m["trades"]) for m in all_metrics if m is not None)
    total_instruments = len([m for m in all_metrics if m is not None])

    summary = f"""
    SIMULATION SUMMARY

    Total Instruments: {total_instruments}
    Total Trades: {total_trades:,}

    Instrument Groups:
      • USD-quoted Spot: {len(USD_SPOT)}
      • ABC-quoted Spot: {len(ABC_SPOT)}
      • Perpetuals: {len(PERPS)}
    """

    ax4.text(0.5, 0.3, summary, transform=ax4.transAxes,
            fontsize=12, verticalalignment='center', ha='center',
            fontfamily='monospace',
            bbox=dict(boxstyle='round', facecolor='lightblue', alpha=0.3))

    plt.tight_layout()
    fig.savefig(output_path, dpi=150, bbox_inches='tight')
    plt.close(fig)
    print(f"✓ Saved summary: {output_path}")


def main():
    """Main analysis function."""
    print("=" * 60)
    print("Microstructure V1 Analysis")
    print("=" * 60)
    print(f"\nRun directory: {LOG_DIR}")

    # Create output directory
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    print(f"Output directory: {OUTPUT_DIR}")

    # Process each instrument
    all_metrics = []

    print("\nProcessing instruments:")
    for symbol in ALL_INSTRUMENTS:
        print(f"\n{symbol}:")
        df = load_instrument_data(symbol)
        metrics = extract_metrics(df, symbol)

        if metrics is not None:
            all_metrics.append(metrics)

            # Print quick stats
            trades = metrics["trades"]
            print(f"  Trades: {len(trades):,}")
            print(f"  Duration: {trades['time_sec'].max():.1f}s")
            print(f"  Price range: {trades['price_normalized'].min():.4f} - {trades['price_normalized'].max():.4f}")

            if len(metrics["snapshots"]) > 0:
                print(f"  Mean spread: {metrics['snapshots']['spread_bps'].mean():.2f} bps")

            # Create individual plot
            output_file = OUTPUT_DIR / f"{symbol.replace('/', '-')}.png"
            plot_instrument(metrics, output_file)
        else:
            print(f"  No data available")

    # Create summary plot
    if all_metrics:
        print("\nCreating summary plot...")
        summary_path = OUTPUT_DIR / "00_SUMMARY.png"
        create_summary_plot(all_metrics, summary_path)

    print("\n" + "=" * 60)
    print(f"Analysis complete! {len(all_metrics)} instruments processed.")
    print(f"Plots saved to: {OUTPUT_DIR}")
    print("=" * 60)


if __name__ == "__main__":
    main()
