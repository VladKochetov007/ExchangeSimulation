#!/usr/bin/env python3
import polars as pl
import matplotlib.pyplot as plt
from pathlib import Path
import sys

# Auto-detect log file
if len(sys.argv) > 1:
    log_file = Path(sys.argv[1])
else:
    # Try to find the log file in structured format
    log_files = list(Path("logs").rglob("*.log"))
    if not log_files:
        print("No log files found in logs/")
        sys.exit(1)
    log_file = log_files[0]
    print(f"Using log file: {log_file}")

if not log_file.exists():
    print(f"Log file {log_file} not found")
    exit(1)

df = pl.read_ndjson(log_file)

print(f"Total events: {len(df)}")
print(f"\nEvent types:")
print(df.group_by("event").agg(pl.len()).sort("event"))

print(f"\nEvent counts by client:")
print(df.group_by(["client_id", "event"]).agg(pl.len()).sort(["client_id", "event"]))

trades = df.filter(pl.col("event") == "Trade")
print(f"\nTotal trades: {len(trades)}")

if len(trades) > 0:
    start_time = df["sim_time"].min()
    trades = trades.with_columns([
        ((pl.col("sim_time") - start_time) / 1e9).alias("time_sec"),
        (pl.col("price") / 1e8).alias("price_btc")
    ])

    print("\nTrade statistics:")
    print(f"  Price range: ${trades['price_btc'].min():.2f} - ${trades['price_btc'].max():.2f}")
    print(f"  Total volume: {(trades['qty'].sum() / 1e8):.4f} BTC")
    print(f"  Average trade size: {(trades['qty'].mean() / 1e8):.4f} BTC")

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 8))

    trades_pd = trades.to_pandas()
    ax1.plot(trades_pd["time_sec"], trades_pd["price_btc"], marker='o', linestyle='-', markersize=4)
    ax1.set_xlabel("Simulation Time (seconds)")
    ax1.set_ylabel("Price (BTC/USD)")
    ax1.set_title("Trade Prices Over Time")
    ax1.grid(True, alpha=0.3)

    ax2.bar(range(len(trades_pd)), trades_pd["qty"] / 1e8)
    ax2.set_xlabel("Trade Number")
    ax2.set_ylabel("Size (BTC)")
    ax2.set_title("Trade Sizes")
    ax2.grid(True, alpha=0.3)

    plt.tight_layout()
    plt.savefig("logs/trades_analysis.png", dpi=150)
    print(f"\nPlot saved to logs/trades_analysis.png")

book_deltas = df.filter(pl.col("event") == "BookDelta")
if len(book_deltas) > 0:
    print(f"\nBook delta events: {len(book_deltas)}")
    print(book_deltas.group_by("side").agg(pl.len()).sort("side"))

fills = df.filter(pl.col("event") == "OrderFill")
if len(fills) > 0:
    fills_by_client = fills.group_by("client_id").agg([
        pl.len().alias("num_fills"),
        pl.col("qty").sum().alias("total_qty"),
        pl.col("role").value_counts()
    ])
    print(f"\nFills by client:")
    print(fills_by_client)

rejections = df.filter(pl.col("event") == "OrderRejected")
if len(rejections) > 0:
    print(f"\nOrder rejections: {len(rejections)}")
    print(rejections.group_by("error").agg(pl.len()).sort("len", descending=True))
