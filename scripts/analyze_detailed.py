#!/usr/bin/env python3
import polars as pl
import matplotlib.pyplot as plt
from pathlib import Path
import numpy as np

log_file = Path("logs/simulation.log")
df = pl.read_ndjson(log_file)

start_time = df["sim_time"].min()

trades = df.filter(pl.col("event") == "Trade").with_columns([
    ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
    (pl.col("price") / 1e8).alias("price_usd"),
    (pl.col("qty") / 1e8).alias("qty_btc")
])

fills = df.filter(pl.col("event") == "OrderFill").with_columns([
    ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
    (pl.col("price") / 1e8).alias("price_usd"),
    (pl.col("qty") / 1e8).alias("qty_btc"),
    pl.when(pl.col("side") == "BUY").then(pl.col("qty"))
      .otherwise(-pl.col("qty")).alias("signed_qty")
])

book_deltas = df.filter(pl.col("event") == "BookDelta").with_columns([
    ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
    (pl.col("price") / 1e8).alias("price_usd"),
    (pl.col("visible_qty") / 1e8).alias("qty_btc")
])

fig = plt.figure(figsize=(16, 12))
gs = fig.add_gridspec(4, 2, hspace=0.3, wspace=0.3)

ax1 = fig.add_subplot(gs[0, :])
trades_pd = trades.to_pandas()
colors = ['g' if side == 'BUY' else 'r' for side in trades_pd['side']]
ax1.scatter(trades_pd['time_sec'], trades_pd['price_usd'], c=colors, alpha=0.6, s=trades_pd['qty_btc']*500)
ax1.set_xlabel('Time (s)')
ax1.set_ylabel('Price (USD)')
ax1.set_title('Trade Prices Over Time (size = volume, green=buy, red=sell)')
ax1.grid(True, alpha=0.3)

ax2 = fig.add_subplot(gs[1, 0])
cumulative_volume = np.cumsum(trades_pd['qty_btc'])
ax2.plot(trades_pd['time_sec'], cumulative_volume, linewidth=2)
ax2.set_xlabel('Time (s)')
ax2.set_ylabel('Cumulative Volume (BTC)')
ax2.set_title('Cumulative Trading Volume')
ax2.grid(True, alpha=0.3)

ax3 = fig.add_subplot(gs[1, 1])
trade_intervals = np.diff(trades_pd['time_sec'])
if len(trade_intervals) > 0:
    ax3.hist(trade_intervals, bins=30, alpha=0.7, edgecolor='black')
    ax3.set_xlabel('Time Between Trades (s)')
    ax3.set_ylabel('Frequency')
    ax3.set_title(f'Trade Interval Distribution (mean={np.mean(trade_intervals):.2f}s)')
    ax3.grid(True, alpha=0.3)

ax4 = fig.add_subplot(gs[2, 0])
fills_pd = fills.to_pandas()
client_inventory = {}
for client_id in fills_pd['client_id'].unique():
    client_fills = fills_pd[fills_pd['client_id'] == client_id].sort_values('time_sec')
    inventory = np.cumsum(client_fills['signed_qty'] / 1e8)
    ax4.plot(client_fills['time_sec'], inventory, label=f'Client {client_id}', linewidth=2)
ax4.set_xlabel('Time (s)')
ax4.set_ylabel('Inventory (BTC)')
ax4.set_title('Actor Inventory Over Time')
ax4.legend()
ax4.grid(True, alpha=0.3)
ax4.axhline(y=0, color='k', linestyle='--', alpha=0.3)

ax5 = fig.add_subplot(gs[2, 1])
client_fills = fills_pd.groupby('client_id').agg({
    'qty_btc': 'sum',
    'role': lambda x: (x == 'maker').sum()
}).reset_index()
client_fills.columns = ['client_id', 'total_volume', 'maker_fills']
ax5.bar(client_fills['client_id'], client_fills['total_volume'], alpha=0.7, edgecolor='black')
ax5.set_xlabel('Client ID')
ax5.set_ylabel('Total Volume (BTC)')
ax5.set_title('Volume by Actor')
ax5.grid(True, alpha=0.3, axis='y')

ax6 = fig.add_subplot(gs[3, 0])
book_bids = book_deltas.filter(pl.col("side") == "BUY").to_pandas()
book_asks = book_deltas.filter(pl.col("side") == "SELL").to_pandas()
if len(book_bids) > 0:
    ax6.plot(book_bids['time_sec'], book_bids['price_usd'], 'g-', alpha=0.3, label='Best Bid')
if len(book_asks) > 0:
    ax6.plot(book_asks['time_sec'], book_asks['price_usd'], 'r-', alpha=0.3, label='Best Ask')
if len(book_bids) > 0 and len(book_asks) > 0:
    merged = book_bids.merge(book_asks, on='time_sec', how='outer', suffixes=('_bid', '_ask')).sort_values('time_sec')
    merged = merged.ffill()
    spread = merged['price_usd_ask'] - merged['price_usd_bid']
    ax6_twin = ax6.twinx()
    ax6_twin.plot(merged['time_sec'], spread, 'b-', alpha=0.5, linewidth=2, label='Spread')
    ax6_twin.set_ylabel('Spread (USD)', color='b')
    ax6_twin.tick_params(axis='y', labelcolor='b')
ax6.set_xlabel('Time (s)')
ax6.set_ylabel('Price (USD)')
ax6.set_title('Best Bid/Ask and Spread Over Time')
ax6.legend(loc='upper left')
ax6.grid(True, alpha=0.3)

ax7 = fig.add_subplot(gs[3, 1])
role_counts = fills_pd.groupby(['client_id', 'role']).size().reset_index(name='count')
role_pivot = role_counts.pivot(index='client_id', columns='role', values='count').fillna(0)
if 'maker' in role_pivot.columns and 'taker' in role_pivot.columns:
    x = np.arange(len(role_pivot))
    width = 0.35
    ax7.bar(x - width/2, role_pivot['maker'], width, label='Maker', alpha=0.7)
    ax7.bar(x + width/2, role_pivot['taker'], width, label='Taker', alpha=0.7)
    ax7.set_xlabel('Client ID')
    ax7.set_ylabel('Number of Fills')
    ax7.set_title('Maker vs Taker Fills by Actor')
    ax7.set_xticks(x)
    ax7.set_xticklabels(role_pivot.index)
    ax7.legend()
    ax7.grid(True, alpha=0.3, axis='y')

plt.savefig('logs/detailed_analysis.png', dpi=150, bbox_inches='tight')
print(f"\nDetailed analysis plot saved to logs/detailed_analysis.png")

print(f"\n=== Simulation Summary ===")
print(f"Duration: {trades_pd['time_sec'].max() - trades_pd['time_sec'].min():.1f} seconds")
print(f"Total trades: {len(trades)}")
print(f"Total volume: {trades_pd['qty_btc'].sum():.4f} BTC")
print(f"Average trade size: {trades_pd['qty_btc'].mean():.4f} BTC")
print(f"Price range: ${trades_pd['price_usd'].min():.2f} - ${trades_pd['price_usd'].max():.2f}")
print(f"Average spread: ${spread.mean():.2f}" if len(book_bids) > 0 and len(book_asks) > 0 else "")
print(f"\nActors:")
for _, row in client_fills.iterrows():
    print(f"  Client {int(row['client_id'])}: {row['total_volume']:.4f} BTC volume, {int(row['maker_fills'])} maker fills")
