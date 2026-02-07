#!/usr/bin/env python3
"""
L3 orderbook reconstruction from exchange logs using snapshots and deltas.

This script provides accurate orderbook state reconstruction by:
1. Starting from the first BookSnapshot event
2. Applying BookDelta events sequentially
3. Handling hidden quantities (iceberg orders)
4. Validating book integrity

Usage:
    python reconstruct_book_l3.py <logfile> [timestamp_ns]
"""

import json
import sys
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass


@dataclass
class Level:
    """Represents a price level in the L3 orderbook."""
    price: int
    visible_qty: int
    hidden_qty: int
    total_qty: int

    def is_empty(self) -> bool:
        return self.total_qty == 0


class OrderbookL3:
    """L3 orderbook state with full depth and hidden quantities."""

    def __init__(self):
        self.bids: Dict[int, Level] = {}  # price -> Level
        self.asks: Dict[int, Level] = {}  # price -> Level
        self.last_update_time: int = 0

    def apply_snapshot(self, snapshot: dict, timestamp: int):
        """Apply a full book snapshot."""
        self.bids.clear()
        self.asks.clear()

        # Parse bids
        for level in snapshot.get('bids', []):
            if isinstance(level, dict):
                price = level.get('Price', level.get('price', 0))
                visible = level.get('VisibleQty', level.get('visible_qty', 0))
                hidden = level.get('HiddenQty', level.get('hidden_qty', 0))
            else:
                # Handle array format [price, visible, hidden]
                price, visible, hidden = level[0], level[1], level[2] if len(level) > 2 else 0

            if price > 0:
                self.bids[price] = Level(
                    price=price,
                    visible_qty=visible,
                    hidden_qty=hidden,
                    total_qty=visible + hidden
                )

        # Parse asks
        for level in snapshot.get('asks', []):
            if isinstance(level, dict):
                price = level.get('Price', level.get('price', 0))
                visible = level.get('VisibleQty', level.get('visible_qty', 0))
                hidden = level.get('HiddenQty', level.get('hidden_qty', 0))
            else:
                # Handle array format
                price, visible, hidden = level[0], level[1], level[2] if len(level) > 2 else 0

            if price > 0:
                self.asks[price] = Level(
                    price=price,
                    visible_qty=visible,
                    hidden_qty=hidden,
                    total_qty=visible + hidden
                )

        self.last_update_time = timestamp

    def apply_delta(self, delta: dict, timestamp: int):
        """Apply a book delta update."""
        side = delta.get('side', '').upper()
        price = delta.get('price', 0)
        visible = delta.get('visible_qty', 0)
        hidden = delta.get('hidden_qty', 0)
        total = delta.get('total_qty', visible + hidden)

        if price == 0:
            return

        book = self.bids if side == 'BUY' else self.asks

        if total == 0 or visible == 0:
            # Delete level
            book.pop(price, None)
        else:
            # Update or insert level
            book[price] = Level(
                price=price,
                visible_qty=visible,
                hidden_qty=hidden,
                total_qty=total
            )

        self.last_update_time = timestamp

    def get_best_bid(self) -> Optional[Level]:
        """Get best bid price level."""
        if not self.bids:
            return None
        best_price = max(self.bids.keys())
        return self.bids[best_price]

    def get_best_ask(self) -> Optional[Level]:
        """Get best ask price level."""
        if not self.asks:
            return None
        best_price = min(self.asks.keys())
        return self.asks[best_price]

    def get_mid_price(self) -> Optional[float]:
        """Get mid price."""
        best_bid = self.get_best_bid()
        best_ask = self.get_best_ask()

        if best_bid and best_ask:
            return (best_bid.price + best_ask.price) / 2.0
        return None

    def get_spread_bps(self) -> Optional[float]:
        """Get spread in basis points."""
        best_bid = self.get_best_bid()
        best_ask = self.get_best_ask()

        if best_bid and best_ask and best_bid.price > 0:
            spread = best_ask.price - best_bid.price
            return (spread / best_bid.price) * 10000
        return None

    def get_depth_summary(self, levels: int = 5) -> dict:
        """Get depth summary for top N levels."""
        sorted_bids = sorted(self.bids.values(), key=lambda x: x.price, reverse=True)[:levels]
        sorted_asks = sorted(self.asks.values(), key=lambda x: x.price)[:levels]

        return {
            'bids': [
                {
                    'price': level.price,
                    'visible_qty': level.visible_qty,
                    'hidden_qty': level.hidden_qty,
                    'total_qty': level.total_qty
                }
                for level in sorted_bids
            ],
            'asks': [
                {
                    'price': level.price,
                    'visible_qty': level.visible_qty,
                    'hidden_qty': level.hidden_qty,
                    'total_qty': level.total_qty
                }
                for level in sorted_asks
            ],
            'mid_price': self.get_mid_price(),
            'spread_bps': self.get_spread_bps(),
            'bid_levels': len(self.bids),
            'ask_levels': len(self.asks),
            'timestamp': self.last_update_time
        }


def reconstruct_book_at_time(logfile: str, target_time: Optional[int] = None) -> OrderbookL3:
    """
    Reconstruct orderbook state at a specific timestamp.

    Args:
        logfile: Path to the log file
        target_time: Target timestamp in nanoseconds (None = end of log)

    Returns:
        OrderbookL3 instance with reconstructed state
    """
    book = OrderbookL3()
    snapshot_found = False
    events_processed = 0

    with open(logfile, 'r') as f:
        for line in f:
            try:
                event = json.loads(line)
                sim_time = event.get('sim_time', 0)

                # Stop if we've reached target time
                if target_time and sim_time > target_time:
                    break

                event_type = event.get('event')

                if event_type == 'BookSnapshot':
                    # Full snapshot - rebuild entire book
                    book.apply_snapshot(event, sim_time)
                    snapshot_found = True
                    events_processed += 1

                elif event_type == 'BookDelta':
                    # Delta update
                    if snapshot_found:
                        book.apply_delta(event, sim_time)
                        events_processed += 1

            except (json.JSONDecodeError, KeyError) as e:
                continue

    if not snapshot_found:
        print("WARNING: No BookSnapshot found in log. Book reconstruction may be incomplete.")

    print(f"Processed {events_processed} book events")
    return book


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    logfile = sys.argv[1]
    target_time = int(sys.argv[2]) if len(sys.argv) > 2 else None

    print(f"Reconstructing orderbook from: {logfile}")
    if target_time:
        print(f"Target time: {target_time} ns ({target_time/1e9:.2f}s)")

    book = reconstruct_book_at_time(logfile, target_time)

    # Print summary
    summary = book.get_depth_summary(levels=10)

    print(f"\n{'='*80}")
    print(f"ORDERBOOK STATE")
    print(f"{'='*80}")
    print(f"Timestamp: {summary['timestamp']/1e9:.2f}s")
    print(f"Mid Price: ${summary['mid_price']/1e8:,.2f}" if summary['mid_price'] else "Mid Price: N/A")
    print(f"Spread: {summary['spread_bps']:.2f} bps" if summary['spread_bps'] else "Spread: N/A")
    print(f"Bid Levels: {summary['bid_levels']}")
    print(f"Ask Levels: {summary['ask_levels']}")

    print(f"\n{'='*80}")
    print(f"TOP 10 BIDS")
    print(f"{'='*80}")
    print(f"{'Price':>15} {'Visible':>12} {'Hidden':>12} {'Total':>12}")
    print("-" * 80)
    for level in summary['bids']:
        price = level['price'] / 1e8
        visible = level['visible_qty'] / 1e8
        hidden = level['hidden_qty'] / 1e8
        total = level['total_qty'] / 1e8
        print(f"${price:>14,.2f} {visible:>12.4f} {hidden:>12.4f} {total:>12.4f}")

    print(f"\n{'='*80}")
    print(f"TOP 10 ASKS")
    print(f"{'='*80}")
    print(f"{'Price':>15} {'Visible':>12} {'Hidden':>12} {'Total':>12}")
    print("-" * 80)
    for level in summary['asks']:
        price = level['price'] / 1e8
        visible = level['visible_qty'] / 1e8
        hidden = level['hidden_qty'] / 1e8
        total = level['total_qty'] / 1e8
        print(f"${price:>14,.2f} {visible:>12.4f} {hidden:>12.4f} {total:>12.4f}")

    # Check for issues
    if summary['bid_levels'] == 0:
        print("\n⚠️  WARNING: No bids in orderbook")
    if summary['ask_levels'] == 0:
        print("\n⚠️  WARNING: No asks in orderbook")

    return book


if __name__ == "__main__":
    main()
