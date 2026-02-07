import json
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
import sys
import random
import os
from datetime import datetime

def parse_logs(filepath):
    events = []
    print(f"Parsing logs from {filepath}...")
    with open(filepath, 'r') as f:
        for line in f:
            try:
                data = json.loads(line)
                events.append(data)
            except json.JSONDecodeError:
                continue
    return events

def get_book_at_time(events, target_time_ns=None):
    bids = {} # order_id -> (price, qty)
    asks = {} # order_id -> (price, qty)
    
    last_time = 0
    timestamps = []

    # Helper to get order ID
    def get_id(evt):
        return evt.get('order_id') or evt.get('id') or evt.get('ID')

    # If target_time_ns is None, we need to collect all timestamps first to pick random
    # But that requires two passes or storing everything.
    # Storing everything is memory intensive.
    # Let's do one pass to build book state history? No, too big.
    # Let's do pass to find max time if random needed.
    
    if target_time_ns is None:
        # Find range
        start_time = 0
        end_time = 0
        for evt in events:
            t = evt.get('sim_time', 0)
            if start_time == 0: start_time = t
            end_time = t
        
        if end_time == 0:
            return None, None, 0
            
        target_time_ns = random.uniform(start_time, end_time)
        print(f"Selected random time: {target_time_ns} ns ({target_time_ns/1e9:.2f}s)")

    print(f"Reconstructing book at {target_time_ns} ns...")
    
    current_time = 0
    for evt in events:
        ts = evt.get('sim_time', 0)
        if ts > target_time_ns:
            break
        current_time = ts
        
        etype = evt.get('event')
        
        if etype == 'OrderAccepted':
            oid = get_id(evt)
            side = evt.get('side')
            price = evt.get('price')
            qty = evt.get('qty')
            otype = evt.get('type')
            
            if otype == 'LIMIT' and oid is not None:
                if side == 'BUY':
                    bids[oid] = {'price': price, 'qty': qty}
                elif side == 'SELL':
                    asks[oid] = {'price': price, 'qty': qty}
                    
        elif etype in ('OrderFill', 'OrderPartialFill'):
             oid = get_id(evt)
             remaining = evt.get('remaining_qty', 0)
             if remaining == 0:
                 if oid in bids: del bids[oid]
                 if oid in asks: del asks[oid]
             else:
                 # Update qty
                 if oid in bids: bids[oid]['qty'] = remaining
                 if oid in asks: asks[oid]['qty'] = remaining

        elif etype == 'OrderCancelled':
            oid = get_id(evt)
            if oid in bids: del bids[oid]
            if oid in asks: del asks[oid]
            
    return bids, asks, current_time

def plot_depth(bids, asks, timestamp, filename):
    bid_data = [{'price': v['price'], 'quantity': v['qty'], 'side': 'bid'} for v in bids.values()]
    ask_data = [{'price': v['price'], 'quantity': v['qty'], 'side': 'ask'} for v in asks.values()]
    
    bid_df = pd.DataFrame(bid_data)
    ask_df = pd.DataFrame(ask_data)
    
    plt.figure(figsize=(12, 6))
    ax = plt.gca()
    
    # Filter out outliers for better visualization?
    # Maybe limit to +/- 5% of mid price
    if not bid_df.empty and not ask_df.empty:
        best_bid = bid_df['price'].max()
        best_ask = ask_df['price'].min()
        mid = (best_bid + best_ask) / 2
        lower_bound = mid * 0.95
        upper_bound = mid * 1.05
        
        bid_df = bid_df[bid_df['price'] >= lower_bound]
        ask_df = ask_df[ask_df['price'] <= upper_bound]
    
    if not ask_df.empty:
        sns.ecdfplot(x="price", weights="quantity", stat="count",
                     data=ask_df, ax=ax, color="red", label="Asks")
    
    if not bid_df.empty:
        # complementary=True makes it cumulative from right to left (like depth chart)
        sns.ecdfplot(x="price", weights="quantity", stat="count",
                     complementary=True, data=bid_df, ax=ax, color="green", label="Bids")
                     
    ax.set_title(f"Market Depth at {timestamp/1e9:.2f}s - {os.path.basename(filename)}")
    ax.set_xlabel("Price")
    ax.set_ylabel("Cumulative Volume")
    plt.legend()
    plt.grid(True, alpha=0.3)
    
    output_file = 'depth_plot.png'
    plt.savefig(output_file)
    print(f"Depth plot saved to {output_file}")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python plot_depth.py <logfile> [timestamp_ns]")
        sys.exit(1)
        
    filepath = sys.argv[1]
    target_time = float(sys.argv[2]) if len(sys.argv) > 2 else None
    
    events = parse_logs(filepath)
    bids, asks, ts = get_book_at_time(events, target_time)
    
    if not bids and not asks:
        print("Empty book at selected time.")
        sys.exit(0)
        
    plot_depth(bids, asks, ts, filepath)
