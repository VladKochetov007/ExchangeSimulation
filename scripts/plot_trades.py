#!/usr/bin/env python3
"""Plot trades over time to verify execution."""

import json
import sys
import matplotlib.pyplot as plt
import pandas as pd

def parse_trades(filepath):
    """Extract trade events from log file."""
    trades = []
    with open(filepath, 'r') as f:
        for line in f:
            try:
                event = json.loads(line)
                if event.get('event') == 'Trade':
                    trades.append({
                        'time': event['sim_time'] / 1e9,  # Convert to seconds
                        'price': event['price'] / 1e8,  # Assuming SATOSHI precision
                        'qty': event['qty'] / 1e8,
                        'side': event['side']
                    })
            except (json.JSONDecodeError, KeyError):
                continue
    return pd.DataFrame(trades)

def plot_trades(df, output_file='trades.png'):
    """Plot trades over time."""
    if df.empty:
        print("No trades found!")
        return

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(14, 10), sharex=True)

    # Price over time with trades
    buys = df[df['side'] == 'BUY']
    sells = df[df['side'] == 'SELL']

    ax1.scatter(buys['time'], buys['price'], c='green', s=20, alpha=0.6, label='Buy')
    ax1.scatter(sells['time'], sells['price'], c='red', s=20, alpha=0.6, label='Sell')
    ax1.set_ylabel('Price')
    ax1.set_title(f'Trade Execution Over Time ({len(df)} trades)')
    ax1.legend()
    ax1.grid(True, alpha=0.3)

    # Volume over time
    ax2.bar(df['time'], df['qty'], width=0.1, alpha=0.6, color='blue')
    ax2.set_xlabel('Time (seconds)')
    ax2.set_ylabel('Trade Volume')
    ax2.set_title('Trade Volume Over Time')
    ax2.grid(True, alpha=0.3)

    plt.tight_layout()
    plt.savefig(output_file)
    print(f"Trade plot saved to {output_file}")

    # Print statistics
    print(f"\n=== Trade Statistics ===")
    print(f"Total trades: {len(df)}")
    print(f"Buy trades: {len(buys)} ({len(buys)/len(df)*100:.1f}%)")
    print(f"Sell trades: {len(sells)} ({len(sells)/len(df)*100:.1f}%)")
    print(f"Average price: ${df['price'].mean():,.2f}")
    print(f"Price range: ${df['price'].min():,.2f} - ${df['price'].max():,.2f}")
    print(f"Total volume: {df['qty'].sum():,.4f}")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python plot_trades.py <logfile>")
        sys.exit(1)

    filepath = sys.argv[1]
    df = parse_trades(filepath)
    plot_trades(df)
