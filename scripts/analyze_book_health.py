#!/usr/bin/env python3
"""
Analyze orderbook health and identify liquidity issues.
Shows depth imbalance, mid-price, and spread at key moments.
"""

import json
import sys
import matplotlib.pyplot as plt
import numpy as np
from reconstruct_book_l3 import reconstruct_book_at_time, OrderbookL3


def sample_book_states(logfile, num_samples=20):
    """Sample book state at regular intervals."""
    # Get time range
    timestamps = []
    with open(logfile, 'r') as f:
        for line in f:
            try:
                event = json.loads(line)
                ts = event.get('sim_time', 0)
                if ts > 0:
                    timestamps.append(ts)
            except:
                continue

    if not timestamps:
        return []

    start, end = min(timestamps), max(timestamps)
    duration = (end - start) / 1e9

    print(f"Time range: {start/1e9:.2f}s - {end/1e9:.2f}s")
    print(f"Duration: {duration:.2f}s")
    print(f"Sampling {num_samples} points...")

    # Sample evenly
    sample_times = [int(start + (end - start) * i / (num_samples - 1))
                    for i in range(num_samples)]

    results = []
    for i, ts in enumerate(sample_times):
        print(f"  [{i+1}/{num_samples}] t={ts/1e9:.2f}s", end='')
        book = reconstruct_book_at_time(logfile, ts)

        best_bid = book.get_best_bid()
        best_ask = book.get_best_ask()

        # Calculate total liquidity on each side (top 10 levels)
        bid_liquidity = sum(level.total_qty for level in sorted(book.bids.values(), key=lambda x: x.price, reverse=True)[:10])
        ask_liquidity = sum(level.total_qty for level in sorted(book.asks.values(), key=lambda x: x.price)[:10])

        result = {
            'timestamp': ts,
            'time_sec': ts / 1e9,
            'best_bid': best_bid.price / 1e8 if best_bid else None,
            'best_ask': best_ask.price / 1e8 if best_ask else None,
            'mid_price': book.get_mid_price() / 1e8 if book.get_mid_price() else None,
            'spread_bps': book.get_spread_bps(),
            'bid_levels': len(book.bids),
            'ask_levels': len(book.asks),
            'bid_qty': bid_liquidity / 1e8,
            'ask_qty': ask_liquidity / 1e8,
            'imbalance': (bid_liquidity - ask_liquidity) / (bid_liquidity + ask_liquidity + 1) if (bid_liquidity + ask_liquidity) > 0 else 0
        }

        print(f" | Bid: {result['bid_levels']} lvls, {result['bid_qty']:.2f} BTC | Ask: {result['ask_levels']} lvls, {result['ask_qty']:.2f} BTC")
        results.append(result)

    return results


def plot_analysis(results, logfile):
    """Create diagnostic plots."""
    fig, axes = plt.subplots(3, 2, figsize=(16, 12))
    fig.suptitle(f'Orderbook Health Analysis - {logfile}', fontsize=16, fontweight='bold')

    times = [r['time_sec'] - results[0]['time_sec'] for r in results]

    # 1. Mid-price
    ax = axes[0, 0]
    mid_prices = [r['mid_price'] for r in results if r['mid_price']]
    mid_times = [t for t, r in zip(times, results) if r['mid_price']]
    if mid_prices:
        ax.plot(mid_times, mid_prices, 'b-o', linewidth=2, markersize=4)
        ax.set_ylabel('Mid Price ($)')
        ax.set_title('Mid-Price Evolution', fontweight='bold')
        ax.grid(True, alpha=0.3)
    else:
        ax.text(0.5, 0.5, 'No Mid-Price Data', ha='center', va='center', transform=ax.transAxes)

    # 2. Spread
    ax = axes[0, 1]
    spreads = [r['spread_bps'] for r in results if r['spread_bps'] is not None]
    spread_times = [t for t, r in zip(times, results) if r['spread_bps'] is not None]
    if spreads:
        ax.plot(spread_times, spreads, 'purple', linewidth=2, marker='o', markersize=4)
        ax.set_ylabel('Spread (bps)')
        ax.set_title('Bid-Ask Spread', fontweight='bold')
        ax.grid(True, alpha=0.3)
    else:
        ax.text(0.5, 0.5, 'No Spread Data', ha='center', va='center', transform=ax.transAxes)

    # 3. Liquidity on each side
    ax = axes[1, 0]
    bid_qtys = [r['bid_qty'] for r in results]
    ask_qtys = [r['ask_qty'] for r in results]
    ax.plot(times, bid_qtys, 'g-o', linewidth=2, label='Bid Liquidity', markersize=4)
    ax.plot(times, ask_qtys, 'r-o', linewidth=2, label='Ask Liquidity', markersize=4)
    ax.set_ylabel('Liquidity (BTC)')
    ax.set_title('Total Liquidity (Top 10 Levels)', fontweight='bold')
    ax.legend()
    ax.grid(True, alpha=0.3)

    # 4. Liquidity imbalance
    ax = axes[1, 1]
    imbalances = [r['imbalance'] * 100 for r in results]
    colors = ['green' if x > 0 else 'red' for x in imbalances]
    ax.bar(times, imbalances, width=(times[1]-times[0])*0.8 if len(times) > 1 else 1, color=colors, alpha=0.6)
    ax.axhline(y=0, color='black', linestyle='--', linewidth=1)
    ax.set_ylabel('Imbalance (%)')
    ax.set_title('Liquidity Imbalance (+ = More Bids)', fontweight='bold')
    ax.grid(True, alpha=0.3)

    # 5. Number of levels
    ax = axes[2, 0]
    bid_levels = [r['bid_levels'] for r in results]
    ask_levels = [r['ask_levels'] for r in results]
    ax.plot(times, bid_levels, 'g-o', linewidth=2, label='Bid Levels', markersize=4)
    ax.plot(times, ask_levels, 'r-o', linewidth=2, label='Ask Levels', markersize=4)
    ax.set_xlabel('Time (seconds)')
    ax.set_ylabel('Number of Levels')
    ax.set_title('Orderbook Depth (Levels)', fontweight='bold')
    ax.legend()
    ax.grid(True, alpha=0.3)

    # 6. Summary statistics
    ax = axes[2, 1]
    ax.axis('off')

    # Calculate stats
    avg_bid_qty = np.mean([r['bid_qty'] for r in results])
    avg_ask_qty = np.mean([r['ask_qty'] for r in results])
    avg_spread = np.mean([r['spread_bps'] for r in results if r['spread_bps'] is not None]) if any(r['spread_bps'] is not None for r in results) else 0
    avg_bid_levels = np.mean([r['bid_levels'] for r in results])
    avg_ask_levels = np.mean([r['ask_levels'] for r in results])

    stats_text = f"""
SUMMARY STATISTICS
{'='*50}

Average Bid Liquidity:  {avg_bid_qty:.2f} BTC
Average Ask Liquidity:  {avg_ask_qty:.2f} BTC
Liquidity Ratio (Bid/Ask): {avg_bid_qty/avg_ask_qty:.2f}x

Average Spread:         {avg_spread:.2f} bps
Average Bid Levels:     {avg_bid_levels:.1f}
Average Ask Levels:     {avg_ask_levels:.1f}

Samples with Mid-Price: {sum(1 for r in results if r['mid_price'])} / {len(results)}

DIAGNOSIS:
{diagnose_issues(results)}
    """

    ax.text(0.1, 0.9, stats_text, transform=ax.transAxes, fontsize=10,
            verticalalignment='top', fontfamily='monospace',
            bbox=dict(boxstyle='round', facecolor='wheat', alpha=0.5))

    plt.tight_layout()

    output_file = 'orderbook_health_analysis.png'
    plt.savefig(output_file, dpi=150, bbox_inches='tight')
    print(f"\n✅ Analysis saved to: {output_file}")


def diagnose_issues(results):
    """Diagnose orderbook health issues."""
    issues = []

    avg_bid_qty = np.mean([r['bid_qty'] for r in results])
    avg_ask_qty = np.mean([r['ask_qty'] for r in results])

    if avg_bid_qty == 0 and avg_ask_qty == 0:
        issues.append("⚠️  CRITICAL: Book is empty!")
    elif avg_bid_qty < 0.1 and avg_ask_qty > 10:
        issues.append("⚠️  CRITICAL: Severe imbalance (no bids)")
    elif avg_ask_qty < 0.1 and avg_bid_qty > 10:
        issues.append("⚠️  CRITICAL: Severe imbalance (no asks)")
    elif abs(avg_bid_qty - avg_ask_qty) / max(avg_bid_qty, avg_ask_qty) > 0.8:
        issues.append("⚠️  WARNING: Significant imbalance")
    else:
        issues.append("✅ Balanced liquidity")

    samples_with_price = sum(1 for r in results if r['mid_price'])
    if samples_with_price < len(results) * 0.5:
        issues.append("⚠️  Book empty >50% of the time")

    return "\n".join(issues) if issues else "✅ Orderbook healthy"


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    logfile = sys.argv[1]
    num_samples = int(sys.argv[2]) if len(sys.argv) > 2 else 20

    print(f"Analyzing orderbook health: {logfile}\n")

    results = sample_book_states(logfile, num_samples)

    if not results:
        print("❌ No data found!")
        sys.exit(1)

    plot_analysis(results, logfile)


if __name__ == "__main__":
    main()
