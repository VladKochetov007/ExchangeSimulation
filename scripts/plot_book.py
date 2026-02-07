import json
import pandas as pd
import matplotlib.pyplot as plt
import sys
import os

def parse_logs(filepath):
    events = []
    with open(filepath, 'r') as f:
        for line in f:
            try:
                data = json.loads(line)
                events.append(data)
            except json.JSONDecodeError:
                continue
    return events

def reconstruct_book(events):
    bids = {} # order_id -> (price, qty)
    asks = {} # order_id -> (price, qty)
    
    updates = [] # (time, best_bid, best_ask)

    # Helper to get order ID
    def get_id(evt):
        return evt.get('order_id') or evt.get('id') or evt.get('ID')

    for evt in events:
        etype = evt.get('event')
        # Use sim_time (ns)
        ts = evt.get('sim_time', 0)
        
        # Order Accepted (Limit)
        if etype == 'OrderAccepted':
            oid = get_id(evt)
            side = evt.get('side')
            price = evt.get('price')
            qty = evt.get('qty')
            otype = evt.get('type') # LIMIT or MARKET
            
            # Assume LIMIT orders stay in book until filled/cancelled
            if otype == 'LIMIT' and oid is not None:
                if side == 'BUY':
                    bids[oid] = (price, qty)
                elif side == 'SELL':
                    asks[oid] = (price, qty)
        
        # Order Filled
        elif etype in ('OrderFill', 'OrderPartialFill'):
             oid = get_id(evt)
             # Fills reduce qty, removing if fully filled
             # But simplistic view: if we track ID, we can remove it on Full Fill?
             # Or just ignore fills if we only care about price?
             # Actually, if an order is fully filled, it's removed.
             # Logs usually say 'remaining_qty' or similar?
             # Or 'fully_filled': true?
             # Step 296 view of composite showed 'EventOrderFilled'.
             # Let's assume 'remaining_qty' = 0 means removal.
             remaining = evt.get('remaining_qty', 0)
             if remaining == 0:
                 if oid in bids: del bids[oid]
                 if oid in asks: del asks[oid]
        
        # Order Cancelled
        elif etype == 'OrderCancelled':
            oid = get_id(evt)
            if oid in bids: del bids[oid]
            if oid in asks: del asks[oid]
            
        # Snapshot BBO
        best_bid = max([p for p, q in bids.values()], default=None)
        best_ask = min([p for p, q in asks.values()], default=None)
        
        if best_bid is not None or best_ask is not None:
            updates.append({'time': ts, 'bid': best_bid, 'ask': best_ask})

    return pd.DataFrame(updates)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python plot_book.py <logfile>")
        sys.exit(1)
        
    filepath = sys.argv[1]
    events = parse_logs(filepath)
    df = reconstruct_book(events)
    
    if df.empty:
        print("No BBO data found.")
        sys.exit(0)
        
    # Convert time to datetime (ns)
    df['datetime'] = pd.to_datetime(df['time'], unit='ns')
    df = df.set_index('datetime')
    
    # Resample to 1ms
    df_resampled = df.resample('1ms').last().ffill()
    
    plt.figure(figsize=(12, 6))
    plt.plot(df_resampled.index, df_resampled['bid'], label='Best Bid', color='green')
    plt.plot(df_resampled.index, df_resampled['ask'], label='Best Ask', color='red')
    plt.title(f'BBO for {os.path.basename(filepath)}')
    plt.xlabel('Time')
    plt.ylabel('Price')
    plt.legend()
    plt.grid(True)
    
    output_file = 'bbo_plot.png'
    plt.savefig(output_file)
    print(f"Plot saved to {output_file}")
