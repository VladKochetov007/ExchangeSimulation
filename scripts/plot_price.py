#!/usr/bin/env python3
import pandas as pd
import matplotlib.pyplot as plt
from pathlib import Path
import json

LOG_FILE = Path("logs/randomwalk/_global.log")
OUTPUT_FILE = Path("logs/randomwalk/price_plot.png")

# Load trade data
with open(LOG_FILE) as f:
    data = [json.loads(line) for line in f]

df = pd.DataFrame(data)
trades = df[df["event"] == "Trade"].copy()
start_time = df["sim_time"].min()
trades["time_sec"] = (trades["sim_time"] - start_time) / 1e9
trades["price_usd"] = trades["price"] / 1e5  # USD_PRECISION = 100,000
trades = trades.sort_values("time_sec")

print(f"Total trades: {len(trades)}")
print(f"Duration: {trades['time_sec'].max():.1f} seconds")
print(f"Price range: ${trades['price_usd'].min():.2f} - ${trades['price_usd'].max():.2f}")

# Create plot
fig, ax = plt.subplots(figsize=(14, 6))

# Plot price over time
time_series = trades["time_sec"].to_numpy()
price_series = trades["price_usd"].to_numpy()

ax.plot(time_series, price_series, linewidth=1, alpha=0.8, color='blue', marker='o', markersize=3)
ax.set_xlabel('Time (seconds)', fontsize=12)
ax.set_ylabel('Price (USD)', fontsize=12)
ax.set_title('BTC-PERP Price Over Time', fontsize=14, fontweight='bold')
ax.grid(True, alpha=0.3)

plt.tight_layout()
fig.savefig(OUTPUT_FILE, dpi=150)
print(f"\nPlot saved to {OUTPUT_FILE}")
