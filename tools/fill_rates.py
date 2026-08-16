"""Report, per participant class, how often its submitted orders actually fill.

An order that is accepted and never filled is invisible in payoff tables: the
participant simply appears not to have traded. Separating acceptance from
execution is what distinguishes a strategy that declines to trade from one that
tries and cannot.

Usage:
    python tools/fill_rates.py --run logs/<name> [--symbol ABC-USD]
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
    parser.add_argument("--symbol", default="ABC-USD")
    args = parser.parse_args()

    report = json.loads((args.run / "greeks.json").read_text())
    roles = {
        (row["venue_id"], row["client_id"]): role_group(row["role"])
        for row in report["terminal_accounts"]
    }

    accepted: collections.Counter[str] = collections.Counter()
    filled: collections.Counter[str] = collections.Counter()
    volume: collections.Counter[str] = collections.Counter()
    for book in sorted(args.run.glob(f"venues/*/spot/{args.symbol}.jsonl")):
        venue = book.parts[-3]
        for line in book.open():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            role = roles.get((venue, event.get("client_id")))
            if role is None:
                continue
            if event["event"] == "OrderAccepted":
                accepted[role] += 1
            elif event["event"] == "OrderFill":
                filled[role] += 1
                payload = event["data"]["payload"]
                volume[role] += payload.get("qty") or payload.get("filled_qty") or 0

    total_volume = sum(volume.values()) or 1
    print(f"{'class':24s} {'accepted':>9s} {'fills':>8s} {'fill/acc':>9s} {'vol share':>10s}")
    for role in sorted(set(accepted) | set(filled), key=lambda r: -volume[r]):
        ratio = filled[role] / accepted[role] if accepted[role] else float("nan")
        print(f"{role:24s} {accepted[role]:9d} {filled[role]:8d} {ratio:8.1%} {volume[role]/total_volume:9.1%}")


if __name__ == "__main__":
    main()
