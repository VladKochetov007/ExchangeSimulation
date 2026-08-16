"""Report who trades with whom, by trade count and by volume.

Volume-weighted shares can be dominated by a handful of large crossings while
the trade population is diverse. Both views are needed: payoff flows follow
volume, while tape statistics such as sign autocorrelation follow counts.

Usage:
    python tools/counterparty_mix.py --run logs/<name> [--venue central]
"""

from __future__ import annotations

import argparse
import collections
import json
from pathlib import Path


def role_group(role: str) -> str:
    head, _, tail = role.rpartition("_")
    return head if head and tail.isdigit() else role


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True, type=Path)
    parser.add_argument("--venue", default="central")
    parser.add_argument("--symbol", default="ABC-USD")
    parser.add_argument("--top", type=int, default=10)
    args = parser.parse_args()

    report = json.loads((args.run / "greeks.json").read_text())
    roles = {
        row["client_id"]: role_group(row["role"])
        for row in report["terminal_accounts"]
        if row["venue_id"] == args.venue
    }

    legs: dict[int, list[tuple[str, int]]] = collections.defaultdict(list)
    book = args.run / "venues" / args.venue / "spot" / f"{args.symbol}.jsonl"
    for line in book.open():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event["event"] != "OrderFill":
            continue
        payload = event["data"]["payload"]
        trade_id = payload.get("trade_id")
        if trade_id is None:
            continue
        legs[trade_id].append((roles.get(event["client_id"], "?"), payload.get("qty", 0)))

    counts: collections.Counter[tuple[str, str]] = collections.Counter()
    volumes: collections.Counter[tuple[str, str]] = collections.Counter()
    for pair_legs in legs.values():
        if len(pair_legs) != 2:
            continue
        key = tuple(sorted(leg[0] for leg in pair_legs))
        counts[key] += 1
        volumes[key] += pair_legs[0][1]

    total_count = sum(counts.values()) or 1
    total_volume = sum(volumes.values()) or 1
    print(f"{'counterparties':40s} {'count':>8s} {'volume':>8s}")
    for key, count in counts.most_common(args.top):
        label = f"{key[0]} x {key[1]}"
        print(f"{label:40s} {count/total_count:7.1%} {volumes[key]/total_volume:7.1%}")


if __name__ == "__main__":
    main()
