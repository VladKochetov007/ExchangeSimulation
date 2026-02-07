#!/usr/bin/env python3
"""
Plot market depth and mid-price evolution over time.

Uses L3 orderbook reconstruction to show:
1. Top-of-book (best bid/ask) over time
2. Mid-price evolution
3. Spread dynamics
4. Market depth snapshots at multiple timestamps
"""

import json
import sys
import matplotlib.pyplot as plt
import matplotlib.gridspec as gridspec
import numpy as np
from pathlib import Path

# Import L3 reconstruction
import reconstruct_book_l3 as l3


def extract_timestamps(logfile, num_samples=20):
    """Extract evenly-spaced timestamps from log for sampling."""
    timestamps = []

    with open(logfile, 'r') as f:
        for line in f:
            try:
                event = json.loads(line)
                sim_time = event.get('sim_time', 0)
                if sim_time > 0:
                    timestamps.append(sim_time)
            except:
                continue

    if not timestamps:
        return []

    timestamps.sort()
    start = timestamps[0]
    end = timestamps[-1]

    # Sample evenly across time range
    step = (end - start) / (num_samples - 1)
    return [int(start + i * step) for i in range(num_samples)]


def reconstruct_timeline(logfile, timestamps):
    """Reconstruct orderbook state at each timestamp."""
    timeline = []

    for ts in timestamps:
        book = l3.reconstruct_book_at_time(logfile, ts)

        best_bid = book.get_best_bid()
        best_ask = book.get_best_ask()
        mid = book.get_mid_price()
        spread_bps = book.get_spread_bps()

        timeline.append({
            'timestamp': ts,
            'time_sec': ts / 1e9,
            'best_bid': best_bid.price / 1e8 if best_bid else None,
            'best_ask': best_ask.price / 1e8 if best_ask else None,
            'mid_price': mid / 1e8 if mid else None,
            'spread_bps': spread_bps,
            'bid_qty': best_bid.total_qty / 1e8 if best_bid else 0,
            'ask_qty': best_ask.total_qty / 1e8 if best_ask else 0,
            'bid_levels': len(book.bids),
            'ask_levels': len(book.asks),
            'book': book
        })

    return timeline


def plot_timeline(timeline, logfile):
    """Create comprehensive market visualization."""
    fig = plt.figure(figsize=(16, 12))
    gs = gridspec.GridSpec(4, 2, figure=fig, hspace=0.3, wspace=0.3)

    times = [t['time_sec'] for t in timeline]
    times_rel = [t - times[0] for t in times]  # Relative to start

    # 1. Best Bid/Ask over time
    ax1 = fig.add_subplot(gs[0, :])
    best_bids = [t['best_bid'] for t in timeline if t['best_bid']]
    best_asks = [t['best_ask'] for t in timeline if t['best_ask']]
    times_bid = [t - times[0] for t, data in zip(times_rel, timeline) if data['best_bid']]
    times_ask = [t - times[0] for t, data in zip(times_rel, timeline) if data['best_ask']]

    if best_bids:
        ax1.plot(times_bid, best_bids, 'g-', linewidth=2, label='Best Bid', marker='o', markersize=4)
    if best_asks:
        ax1.plot(times_ask, best_asks, 'r-', linewidth=2, label='Best Ask', marker='o', markersize=4)

    ax1.fill_between(times_rel[:min(len(best_bids), len(best_asks))],
                      best_bids[:min(len(best_bids), len(best_asks))],
                      best_asks[:min(len(best_bids), len(best_asks))],
                      alpha=0.2, color='gray', label='Spread')

    ax1.set_ylabel('Price ($)')
    ax1.set_title(f'Best Bid/Ask Over Time - {Path(logfile).name}', fontsize=14, fontweight='bold')
    ax1.legend(loc='best')
    ax1.grid(True, alpha=0.3)

    # 2. Mid-price evolution
    ax2 = fig.add_subplot(gs[1, :], sharex=ax1)
    mid_prices = [t['mid_price'] for t in timeline if t['mid_price']]
    times_mid = [t - times[0] for t, data in zip(times_rel, timeline) if data['mid_price']]

    if mid_prices:
        ax2.plot(times_mid, mid_prices, 'b-', linewidth=2, label='Mid Price')
        ax2.fill_between(times_mid, mid_prices, alpha=0.3, color='blue')

    ax2.set_ylabel('Mid Price ($)')
    ax2.set_title('Mid-Price Evolution', fontsize=12, fontweight='bold')
    ax2.legend(loc='best')
    ax2.grid(True, alpha=0.3)

    # 3. Spread in basis points
    ax3 = fig.add_subplot(gs[2, 0], sharex=ax1)
    spreads = [t['spread_bps'] for t in timeline if t['spread_bps'] is not None]
    times_spread = [t - times[0] for t, data in zip(times_rel, timeline) if data['spread_bps'] is not None]

    if spreads:
        ax3.plot(times_spread, spreads, 'purple', linewidth=2, marker='o', markersize=3)
        ax3.fill_between(times_spread, spreads, alpha=0.3, color='purple')

    ax3.set_xlabel('Time (seconds)')
    ax3.set_ylabel('Spread (bps)')
    ax3.set_title('Bid-Ask Spread', fontsize=12, fontweight='bold')
    ax3.grid(True, alpha=0.3)

    # 4. Book depth (levels)
    ax4 = fig.add_subplot(gs[2, 1], sharex=ax1)
    bid_levels = [t['bid_levels'] for t in timeline]
    ask_levels = [t['ask_levels'] for t in timeline]

    ax4.plot(times_rel, bid_levels, 'g-', linewidth=2, label='Bid Levels', marker='o', markersize=3)
    ax4.plot(times_rel, ask_levels, 'r-', linewidth=2, label='Ask Levels', marker='o', markersize=3)

    ax4.set_xlabel('Time (seconds)')
    ax4.set_ylabel('Number of Levels')
    ax4.set_title('Orderbook Depth (Levels)', fontsize=12, fontweight='bold')
    ax4.legend(loc='best')
    ax4.grid(True, alpha=0.3)

    # 5. Sample depth chart (first timestamp)
    ax5 = fig.add_subplot(gs[3, 0])
    plot_depth_snapshot(ax5, timeline[0]['book'], "Start", times_rel[0])

    # 6. Sample depth chart (last timestamp)
    ax6 = fig.add_subplot(gs[3, 1])
    plot_depth_snapshot(ax6, timeline[-1]['book'], "End", times_rel[-1])

    plt.suptitle(f'Market Microstructure Analysis', fontsize=16, fontweight='bold', y=0.995)

    output_file = 'market_depth_timeline.png'
    plt.savefig(output_file, dpi=150, bbox_inches='tight')
    print(f"\n✅ Market timeline saved to: {output_file}")

    return fig


def plot_depth_snapshot(ax, book, label, time_sec):
    """Plot orderbook depth snapshot."""
    summary = book.get_depth_summary(levels=10)

    if not summary['bids'] and not summary['asks']:
        ax.text(0.5, 0.5, f'Empty Book\n@ t={time_sec:.1f}s',
                ha='center', va='center', fontsize=12, transform=ax.transAxes)
        ax.set_title(f'{label} - Empty', fontsize=11, fontweight='bold')
        return

    # Extract data
    bid_prices = [level['price'] / 1e8 for level in summary['bids']]
    bid_qtys = [level['total_qty'] / 1e8 for level in summary['bids']]
    ask_prices = [level['price'] / 1e8 for level in summary['asks']]
    ask_qtys = [level['total_qty'] / 1e8 for level in summary['asks']]

    # Cumulative sums for depth chart
    if bid_prices:
        bid_cumsum = np.cumsum(bid_qtys[::-1])[::-1]  # Reverse for display
        ax.barh(bid_prices, bid_cumsum, height=(max(bid_prices) - min(bid_prices))/20 if len(bid_prices) > 1 else 1,
                color='green', alpha=0.6, label='Bids')

    if ask_prices:
        ask_cumsum = np.cumsum(ask_qtys)
        ax.barh(ask_prices, ask_cumsum, height=(max(ask_prices) - min(ask_prices))/20 if len(ask_prices) > 1 else 1,
                color='red', alpha=0.6, label='Asks')

    mid = summary.get('mid_price')
    if mid:
        ax.axhline(y=mid/1e8, color='blue', linestyle='--', linewidth=1, alpha=0.7, label=f'Mid: ${mid/1e8:,.2f}')

    ax.set_xlabel('Cumulative Volume')
    ax.set_ylabel('Price ($)')
    ax.set_title(f'{label} @ t={time_sec:.1f}s', fontsize=11, fontweight='bold')
    ax.legend(loc='best', fontsize=8)
    ax.grid(True, alpha=0.3)


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    logfile = sys.argv[1]
    num_samples = int(sys.argv[2]) if len(sys.argv) > 2 else 20

    print(f"Analyzing market depth timeline from: {logfile}")
    print(f"Sampling {num_samples} timestamps...")

    # Extract sample timestamps
    timestamps = extract_timestamps(logfile, num_samples)

    if not timestamps:
        print("❌ No timestamps found in log!")
        sys.exit(1)

    print(f"Time range: {timestamps[0]/1e9:.2f}s - {timestamps[-1]/1e9:.2f}s")
    print(f"Duration: {(timestamps[-1] - timestamps[0])/1e9:.2f}s")

    # Reconstruct orderbook at each timestamp
    print("\nReconstructing orderbook timeline...")
    timeline = reconstruct_timeline(logfile, timestamps)

    # Print summary statistics
    print(f"\n{'='*80}")
    print("TIMELINE SUMMARY")
    print(f"{'='*80}")

    non_empty = [t for t in timeline if t['mid_price'] is not None]
    if non_empty:
        mid_prices = [t['mid_price'] for t in non_empty]
        spreads = [t['spread_bps'] for t in non_empty if t['spread_bps'] is not None]

        print(f"Non-empty samples: {len(non_empty)}/{len(timeline)}")
        print(f"Mid-price range: ${min(mid_prices):,.2f} - ${max(mid_prices):,.2f}")
        print(f"Average mid-price: ${np.mean(mid_prices):,.2f}")

        if spreads:
            print(f"Average spread: {np.mean(spreads):.2f} bps")
            print(f"Spread range: {min(spreads):.2f} - {max(spreads):.2f} bps")
    else:
        print("⚠️  All samples have empty orderbooks!")

    # Create visualizations
    plot_timeline(timeline, logfile)

    return timeline


if __name__ == "__main__":
    main()
