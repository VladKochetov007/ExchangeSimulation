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

# Create comprehensive plot
fig, axes = plt.subplots(2, 2, figsize=(16, 10))

# Panel 1: Price over time
ax1 = axes[0, 0]
ax1.plot(trades["time_sec"], trades["price_usd"], linewidth=1, alpha=0.8, color='blue')
ax1.axhline(y=50000, color='red', linestyle='--', alpha=0.5, label='Bootstrap ($50,000)')
ax1.set_xlabel('Time (seconds)', fontsize=12)
ax1.set_ylabel('Price (USD)', fontsize=12)
ax1.set_title('BTC-PERP Price Evolution', fontsize=14, fontweight='bold')
ax1.legend()
ax1.grid(True, alpha=0.3)

# Panel 2: Price changes (cumulative drift)
ax2 = axes[0, 1]
price_change = trades["price_usd"] - trades["price_usd"].iloc[0]
ax2.plot(trades["time_sec"], price_change, linewidth=1, alpha=0.8, color='green')
ax2.axhline(y=0, color='black', linestyle='-', alpha=0.3)
ax2.set_xlabel('Time (seconds)', fontsize=12)
ax2.set_ylabel('Price Change from Start ($)', fontsize=12)
ax2.set_title('Cumulative Price Drift', fontsize=14, fontweight='bold')
ax2.grid(True, alpha=0.3)

# Panel 3: Trade frequency over time
ax3 = axes[1, 0]
time_bins = np.arange(0, trades["time_sec"].max() + 10, 10)
trade_counts, _ = np.histogram(trades["time_sec"], bins=time_bins)
ax3.bar(time_bins[:-1], trade_counts, width=10, alpha=0.7, color='orange', edgecolor='black')
ax3.set_xlabel('Time (seconds)', fontsize=12)
ax3.set_ylabel('Trades per 10s', fontsize=12)
ax3.set_title('Trade Frequency', fontsize=14, fontweight='bold')
ax3.grid(True, alpha=0.3, axis='y')

# Panel 4: Returns distribution
ax4 = axes[1, 1]
ax4.hist(returns, bins=50, alpha=0.7, color='purple', edgecolor='black', density=True)
ax4.axvline(x=0, color='red', linestyle='--', alpha=0.5, label='Zero return')
ax4.set_xlabel('Price Change ($)', fontsize=12)
ax4.set_ylabel('Density', fontsize=12)
ax4.set_title('Returns Distribution', fontsize=14, fontweight='bold')
ax4.legend()
ax4.grid(True, alpha=0.3)

plt.tight_layout()
fig.savefig(OUTPUT_FILE, dpi=150)
print(f"\nPlot saved to {OUTPUT_FILE}")
