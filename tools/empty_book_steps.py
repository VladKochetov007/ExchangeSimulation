"""Count simulation steps that contain an instant with an empty book side.

Quote replacement that cancels before submitting leaves the book empty for the
remainder of the phase. Every actor scheduled behind the maker meets that state,
so the fraction of steps containing it bounds how often a taker can be denied
liquidity that exists on average.

Usage:
    python tools/empty_book_steps.py <run>/venues/<venue>/spot/ABC-USD.jsonl
"""

from __future__ import annotations

import argparse
import collections
import json
from pathlib import Path


def empty_side_steps(path: Path) -> tuple[int, int, int, int]:
    asks: dict[int, int] = {}
    bids: dict[int, int] = {}
    steps: dict[int, list[bool]] = collections.defaultdict(lambda: [False, False])
    for line in path.open():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            # An interrupted run leaves a partial final line; the steps already
            # counted are still complete observations.
            continue
        if event["event"] != "BookDelta":
            continue
        payload = event["data"]["payload"]
        side = bids if payload["side"] == "BUY" else asks
        if payload["total_qty"] == 0:
            side.pop(payload["price"], None)
        else:
            side[payload["price"]] = payload["total_qty"]
        state = steps[event["sim_ts"]]
        state[0] |= not asks
        state[1] |= not bids
    total = len(steps)
    no_ask = sum(1 for s in steps.values() if s[0])
    no_bid = sum(1 for s in steps.values() if s[1])
    both = sum(1 for s in steps.values() if s[0] and s[1])
    return total, no_ask, no_bid, both


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("books", nargs="+", type=Path)
    args = parser.parse_args()
    for book in args.books:
        total, no_ask, no_bid, both = empty_side_steps(book)
        if total == 0:
            print(f"{book}: no book deltas")
            continue
        print(
            f"{book}: steps={total} no_ask={no_ask/total:.1%} "
            f"no_bid={no_bid/total:.1%} both_empty={both/total:.1%}"
        )


if __name__ == "__main__":
    main()
