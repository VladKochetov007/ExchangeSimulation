#!/usr/bin/env python3
import polars as pl
import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from pathlib import Path
from collections import defaultdict

log_file = Path("logs/simulation.log")
df = pl.read_ndjson(log_file)

start_time = df["sim_time"].min()

book_deltas = df.filter(pl.col("event") == "BookDelta").with_columns([
    ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
    (pl.col("price") / 1e8).alias("price_usd"),
    (pl.col("visible_qty") / 1e8).alias("qty_btc")
])

trades = df.filter(pl.col("event") == "Trade").with_columns([
    ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
    (pl.col("price") / 1e8).alias("price_usd"),
    (pl.col("qty") / 1e8).alias("qty_btc")
])

book_deltas_pd = book_deltas.to_pandas()
trades_pd = trades.to_pandas()

bids = book_deltas_pd[book_deltas_pd['side'] == 'BUY'].copy()
asks = book_deltas_pd[book_deltas_pd['side'] == 'SELL'].copy()

fig = plt.figure(figsize=(18, 12))
gs = fig.add_gridspec(3, 2, hspace=0.35, wspace=0.25)

# Panel 1: BBO (Best Bid/Offer) over time
ax1 = fig.add_subplot(gs[0, :])
if len(bids) > 0:
    ax1.plot(bids['time_sec'], bids['price_usd'], 'g-', alpha=0.6, linewidth=1.5, label='Best Bid')
if len(asks) > 0:
    ax1.plot(asks['time_sec'], asks['price_usd'], 'r-', alpha=0.6, linewidth=1.5, label='Best Ask')
if len(trades_pd) > 0:
    trade_colors = ['g' if side == 'BUY' else 'r' for side in trades_pd['side']]
    ax1.scatter(trades_pd['time_sec'], trades_pd['price_usd'],
               c=trade_colors, alpha=0.4, s=30, marker='x', linewidths=1.5, label='Trades')

# Calculate and plot mid-price
if len(bids) > 0 and len(asks) > 0:
    merged = bids[['time_sec', 'price_usd']].merge(
        asks[['time_sec', 'price_usd']], on='time_sec', how='outer', suffixes=('_bid', '_ask')
    ).sort_values('time_sec').ffill()
    merged['mid'] = (merged['price_usd_bid'] + merged['price_usd_ask']) / 2
    ax1.plot(merged['time_sec'], merged['mid'], 'b--', alpha=0.4, linewidth=1, label='Mid Price')

ax1.set_xlabel('Time (seconds)', fontsize=11)
ax1.set_ylabel('Price (USD)', fontsize=11)
ax1.set_title('Best Bid/Offer and Trades Over Time', fontsize=13, fontweight='bold')
ax1.legend(loc='best', fontsize=9)
ax1.grid(True, alpha=0.3, linestyle='--')

# Panel 2: Spread over time
ax2 = fig.add_subplot(gs[1, 0])
if len(bids) > 0 and len(asks) > 0:
    merged = bids[['time_sec', 'price_usd']].merge(
        asks[['time_sec', 'price_usd']], on='time_sec', how='outer', suffixes=('_bid', '_ask')
    ).sort_values('time_sec').ffill()
    merged['mid'] = (merged['price_usd_bid'] + merged['price_usd_ask']) / 2
    merged['spread'] = merged['price_usd_ask'] - merged['price_usd_bid']
    merged['spread_bps'] = (merged['spread'] / merged['mid']) * 10000

    ax2.plot(merged['time_sec'], merged['spread'], 'b-', linewidth=2, alpha=0.7)
    ax2.fill_between(merged['time_sec'], 0, merged['spread'], alpha=0.3)
    ax2.set_xlabel('Time (seconds)', fontsize=11)
    ax2.set_ylabel('Spread (USD)', fontsize=11)
    ax2.set_title('Bid-Ask Spread Over Time', fontsize=12, fontweight='bold')
    ax2.grid(True, alpha=0.3, linestyle='--')

    ax2_twin = ax2.twinx()
    ax2_twin.set_ylabel('Spread (bps)', color='orange', fontsize=11)
    ax2_twin.tick_params(axis='y', labelcolor='orange')
    mean_spread_bps = merged['spread_bps'].mean()
    ax2_twin.axhline(y=mean_spread_bps, color='orange', linestyle='--',
                     alpha=0.5, label=f'Mean: {mean_spread_bps:.1f} bps')
    ax2_twin.legend(loc='upper right', fontsize=9)

# Panel 3: Liquidity at BBO over time
ax3 = fig.add_subplot(gs[1, 1])
if len(bids) > 0:
    ax3.fill_between(bids['time_sec'], 0, bids['qty_btc'],
                     color='green', alpha=0.4, label='Bid Size', step='post')
if len(asks) > 0:
    ax3.fill_between(asks['time_sec'], 0, asks['qty_btc'],
                     color='red', alpha=0.4, label='Ask Size', step='post')
ax3.set_xlabel('Time (seconds)', fontsize=11)
ax3.set_ylabel('Quantity at BBO (BTC)', fontsize=11)
ax3.set_title('Liquidity at Best Bid/Offer', fontsize=12, fontweight='bold')
ax3.legend(loc='best', fontsize=9)
ax3.grid(True, alpha=0.3, linestyle='--')

# Panel 4: Order book snapshots at various times
ax4 = fig.add_subplot(gs[2, 0])
snapshot_times = np.linspace(book_deltas_pd['time_sec'].min(),
                             book_deltas_pd['time_sec'].max(), 6)

for i, snap_time in enumerate(snapshot_times):
    # Find book state just before this time
    bids_at_time = bids[bids['time_sec'] <= snap_time]
    asks_at_time = asks[asks['time_sec'] <= snap_time]

    if len(bids_at_time) > 0 and len(asks_at_time) > 0:
        # Get most recent state for each price level
        latest_bid = bids_at_time.sort_values('time_sec').iloc[-1]
        latest_ask = asks_at_time.sort_values('time_sec').iloc[-1]

        color = plt.cm.viridis(i / len(snapshot_times))
        ax4.barh(latest_bid['price_usd'], latest_bid['qty_btc'],
                height=0.1, color=color, alpha=0.6, label=f't={snap_time:.0f}s')
        ax4.barh(latest_ask['price_usd'], -latest_ask['qty_btc'],
                height=0.1, color=color, alpha=0.6)

ax4.axvline(x=0, color='black', linestyle='-', linewidth=0.5)
ax4.set_xlabel('Quantity (BTC, positive=bid, negative=ask)', fontsize=11)
ax4.set_ylabel('Price (USD)', fontsize=11)
ax4.set_title('Order Book Snapshots at Different Times', fontsize=12, fontweight='bold')
ax4.legend(loc='best', fontsize=8, ncol=2)
ax4.grid(True, alpha=0.3, axis='x', linestyle='--')

# Panel 5: Liquidity heatmap
ax5 = fig.add_subplot(gs[2, 1])

# Create time bins
time_bins = np.linspace(book_deltas_pd['time_sec'].min(),
                        book_deltas_pd['time_sec'].max(), 50)

# Aggregate liquidity by time bin and side
bids_copy = bids.copy()
asks_copy = asks.copy()
bids_copy['time_bin'] = pd.cut(bids_copy['time_sec'], bins=time_bins)
asks_copy['time_bin'] = pd.cut(asks_copy['time_sec'], bins=time_bins)

bid_liquidity = bids_copy.groupby('time_bin')['qty_btc'].sum().reset_index()
ask_liquidity = asks_copy.groupby('time_bin')['qty_btc'].sum().reset_index()

if len(bid_liquidity) > 0 and len(ask_liquidity) > 0:
    bid_liquidity['bin_center'] = bid_liquidity['time_bin'].apply(lambda x: x.mid)
    ask_liquidity['bin_center'] = ask_liquidity['time_bin'].apply(lambda x: x.mid)

    ax5.fill_between(bid_liquidity['bin_center'], 0, bid_liquidity['qty_btc'],
                     color='green', alpha=0.5, label='Bid Liquidity')
    ax5.fill_between(ask_liquidity['bin_center'], 0, ask_liquidity['qty_btc'],
                     color='red', alpha=0.5, label='Ask Liquidity')

    ax5.set_xlabel('Time (seconds)', fontsize=11)
    ax5.set_ylabel('Total Liquidity (BTC)', fontsize=11)
    ax5.set_title('Aggregated Liquidity Over Time', fontsize=12, fontweight='bold')
    ax5.legend(loc='best', fontsize=9)
    ax5.grid(True, alpha=0.3, linestyle='--')

plt.savefig('logs/liquidity_analysis.png', dpi=150, bbox_inches='tight')
print(f"Liquidity analysis plot saved to logs/liquidity_analysis.png")

# Print statistics
print(f"\n=== Liquidity Statistics ===")
if len(merged) > 0:
    print(f"Average spread: ${merged['spread'].mean():.2f} ({merged['spread_bps'].mean():.1f} bps)")
    print(f"Min spread: ${merged['spread'].min():.2f} ({merged['spread_bps'].min():.1f} bps)")
    print(f"Max spread: ${merged['spread'].max():.2f} ({merged['spread_bps'].max():.1f} bps)")

if len(bids) > 0:
    print(f"\nBid side:")
    print(f"  Average size at best: {bids['qty_btc'].mean():.4f} BTC")
    print(f"  Min size: {bids['qty_btc'].min():.4f} BTC")
    print(f"  Max size: {bids['qty_btc'].max():.4f} BTC")

if len(asks) > 0:
    print(f"\nAsk side:")
    print(f"  Average size at best: {asks['qty_btc'].mean():.4f} BTC")
    print(f"  Min size: {asks['qty_btc'].min():.4f} BTC")
    print(f"  Max size: {asks['qty_btc'].max():.4f} BTC")

print(f"\nBook update frequency:")
print(f"  Total updates: {len(book_deltas)}")
print(f"  Bid updates: {len(bids)}")
print(f"  Ask updates: {len(asks)}")
print(f"  Updates per second: {len(book_deltas) / (book_deltas_pd['time_sec'].max() - book_deltas_pd['time_sec'].min()):.2f}")
