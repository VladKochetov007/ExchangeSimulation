#!/usr/bin/env python3
import pandas as pd
import matplotlib.pyplot as plt
from pathlib import Path
import json
import numpy as np

# New logging structure: trades are in symbol-specific log files
LOG_FILE = Path("logs/randomwalk_v2/perp/BTC-PERP.log")
OUTPUT_FILE = Path("logs/randomwalk_v2/price_evolution.png")

# Load trade data from symbol log
with open(LOG_FILE) as f:
    data = [json.loads(line) for line in f]

df = pd.DataFrame(data)
trades = df[df["event"] == "Trade"].copy()

if len(trades) == 0:
    print("No trades found in log file")
    exit(1)

start_time = df["sim_time"].min()
trades["time_sec"] = (trades["sim_time"] - start_time) / 1e9
trades["price_usd"] = trades["price"] / 1e5  # USD_PRECISION = 100,000
trades = trades.sort_values("time_sec")

# Extract order rejections for rejection analysis
rejections = df[df["event"] == "OrderRejected"].copy()
if len(rejections) > 0:
    rejections["time_sec"] = (rejections["sim_time"] - start_time) / 1e9
    rejections = rejections.sort_values("time_sec")
    print(f"\nTotal order rejections: {len(rejections)}")
    if "error" in rejections.columns:
        print(f"Rejection reasons:")
        for reason, count in rejections["error"].value_counts().items():
            print(f"  {reason}: {count}")

# Extract book snapshots for spread analysis
snapshots = df[df["event"] == "BookSnapshot"].copy()
if len(snapshots) > 0:
    snapshots["time_sec"] = (snapshots["sim_time"] - start_time) / 1e9

    # Extract best bid and ask from snapshot - they're already parsed as lists
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

    # Filter for valid snapshots (both bid and ask present)
    snapshots = snapshots[(snapshots["best_bid"] > 0) & (snapshots["best_ask"] > 0)].copy()

    if len(snapshots) > 0:
        snapshots["spread_usd"] = (snapshots["best_ask"] - snapshots["best_bid"]) / 1e5
        snapshots["spread_bps"] = ((snapshots["best_ask"] - snapshots["best_bid"]) / snapshots["best_bid"]) * 10000
        snapshots = snapshots.sort_values("time_sec")

print(f"Total trades: {len(trades)}")
print(f"Duration: {trades['time_sec'].max():.1f} seconds")
print(f"Price range: ${trades['price_usd'].min():.2f} - ${trades['price_usd'].max():.2f}")
print(f"Price change: ${trades['price_usd'].iloc[-1] - trades['price_usd'].iloc[0]:.2f}")
print(f"Mean price: ${trades['price_usd'].mean():.2f}")
print(f"Std dev: ${trades['price_usd'].std():.2f}")

# Calculate returns
returns = trades['price_usd'].diff().dropna()
print(f"\nReturns mean: ${returns.mean():.4f} (should be ~0 for random walk)")
print(f"Returns std: ${returns.std():.4f}")

# Create comprehensive plot (5 panels: 3 rows x 2 columns)
fig, axes = plt.subplots(3, 2, figsize=(16, 14))

# Panel 1: Price over time
ax1 = axes[0, 0]
ax1.plot(trades["time_sec"], trades["price_usd"], linewidth=1, alpha=0.8, color='blue')
ax1.axhline(y=50000, color='red', linestyle='--', alpha=0.5, label='Bootstrap ($50,000)')
ax1.set_xlabel('Time (seconds)', fontsize=12)
ax1.set_ylabel('Price (USD)', fontsize=12)
ax1.set_title('BTC-PERP Price Evolution', fontsize=14, fontweight='bold')
ax1.legend()
ax1.grid(True, alpha=0.3)

# Panel 2: Spread changes over time
ax2 = axes[0, 1]
if len(snapshots) > 0:
    ax2.plot(snapshots["time_sec"], snapshots["spread_bps"], linewidth=1, alpha=0.8, color='green')
    ax2.set_xlabel('Time (seconds)', fontsize=12)
    ax2.set_ylabel('Spread (basis points)', fontsize=12)
    ax2.set_title('Bid-Ask Spread Evolution', fontsize=14, fontweight='bold')
    ax2.grid(True, alpha=0.3)
    # Set y-axis lower limit to 0 for clarity (no negative spreads possible)
    ax2.set_ylim(bottom=0)
    print(f"\nSpread statistics:")
    print(f"Mean spread: {snapshots['spread_bps'].mean():.2f} bps (${snapshots['spread_usd'].mean():.4f})")
    print(f"Min spread: {snapshots['spread_bps'].min():.2f} bps (${snapshots['spread_usd'].min():.4f})")
    print(f"Max spread: {snapshots['spread_bps'].max():.2f} bps (${snapshots['spread_usd'].max():.4f})")
    print(f"Tick size: $0.01 (minimum possible spread)")
else:
    ax2.text(0.5, 0.5, 'No snapshot data available', ha='center', va='center', transform=ax2.transAxes)
    ax2.set_title('Bid-Ask Spread Evolution', fontsize=14, fontweight='bold')

# Panel 3: Trade volume (rolling 10-second window)
ax3 = axes[1, 0]
rolling_window = 10  # seconds
trades_sorted = trades.sort_values("time_sec").copy()

# Calculate rolling trade count using efficient method
rolling_counts = []
times_for_rolling = []

# Sample every second for better performance
for t in np.arange(rolling_window, trades_sorted["time_sec"].max(), 1):
    # Count trades in the last 10 seconds
    count = len(trades_sorted[(trades_sorted["time_sec"] >= t - rolling_window) &
                               (trades_sorted["time_sec"] < t)])
    rolling_counts.append(count)
    times_for_rolling.append(t)

# Plot as filled area chart
ax3.plot(times_for_rolling, rolling_counts, linewidth=1.5, alpha=0.9, color='orange')
ax3.fill_between(times_for_rolling, rolling_counts, alpha=0.3, color='orange')
ax3.set_xlabel('Time (seconds)', fontsize=12)
ax3.set_ylabel('Trades per 10s', fontsize=12)
ax3.set_title('Trade Volume (rolling 10s window)', fontsize=14, fontweight='bold')
ax3.grid(True, alpha=0.3)

# Print statistics
print(f"\nTrade volume statistics:")
print(f"Mean trades/10s: {np.mean(rolling_counts):.1f}")
print(f"Max trades/10s: {np.max(rolling_counts):.0f}")
print(f"Min trades/10s: {np.min(rolling_counts):.0f}")

# Panel 4: Returns distribution
ax4 = axes[1, 1]
ax4.hist(returns, bins=50, alpha=0.7, color='purple', edgecolor='black', density=True)
ax4.axvline(x=0, color='red', linestyle='--', alpha=0.5, label='Zero return')
ax4.set_xlabel('Price Change ($)', fontsize=12)
ax4.set_ylabel('Density', fontsize=12)
ax4.set_title('Returns Distribution', fontsize=14, fontweight='bold')
ax4.legend()
ax4.grid(True, alpha=0.3)

# Panel 5: Order rejection rate (rolling 10-second window)
ax5 = axes[2, 0]
if len(rejections) > 0:
    rolling_window = 10  # seconds
    rejection_counts = []
    times_for_rolling = []

    # Calculate rolling rejection count
    max_time = rejections["time_sec"].max()
    for t in np.arange(rolling_window, max_time, 1):
        count = len(rejections[(rejections["time_sec"] >= t - rolling_window) &
                                (rejections["time_sec"] < t)])
        rejection_counts.append(count)
        times_for_rolling.append(t)

    # Plot as filled area chart
    ax5.plot(times_for_rolling, rejection_counts, linewidth=1.5, alpha=0.9, color='red')
    ax5.fill_between(times_for_rolling, rejection_counts, alpha=0.3, color='red')
    ax5.set_xlabel('Time (seconds)', fontsize=12)
    ax5.set_ylabel('Rejections per 10s', fontsize=12)
    ax5.set_title('Order Rejection Rate (rolling 10s window)', fontsize=14, fontweight='bold')
    ax5.grid(True, alpha=0.3)

    # Print statistics
    if len(rejection_counts) > 0:
        print(f"\nOrder rejection statistics:")
        print(f"Mean rejections/10s: {np.mean(rejection_counts):.1f}")
        print(f"Max rejections/10s: {np.max(rejection_counts):.0f}")
        print(f"Min rejections/10s: {np.min(rejection_counts):.0f}")
else:
    ax5.text(0.5, 0.5, 'No order rejections', ha='center', va='center', transform=ax5.transAxes)
    ax5.set_title('Order Rejection Rate', fontsize=14, fontweight='bold')

# Panel 6: Empty for now (could be used for additional metrics)
ax6 = axes[2, 1]
ax6.axis('off')  # Hide the empty panel

plt.tight_layout()
fig.savefig(OUTPUT_FILE, dpi=150)
print(f"\nPlot saved to {OUTPUT_FILE}")
