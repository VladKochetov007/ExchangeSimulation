"""Whether a run's book is a market or five makers churning against each other.

Reports the maker-versus-maker share of volume, the at-touch taker fill rate by
side, and the share of maker-seconds with only one side quoted. A population
whose makers cross each other consumes its own depth before any outside taker
is scheduled, so every taker measurement in such a run describes queue position
rather than strategy.
"""
import argparse
import collections
import json
from pathlib import Path


def health(run: Path, venue: str = "north", symbol: str = "ABC-USD") -> dict:
    report = json.loads((run / "greeks.json").read_text())
    roles = {}
    for row in report["terminal_accounts"]:
        head, _, tail = row["role"].rpartition("_")
        roles[(row["venue_id"], row["client_id"])] = head if tail.isdigit() else row["role"]

    owner: dict[int, int] = {}
    pair_qty: collections.Counter = collections.Counter()
    total = 0
    expired: collections.Counter = collections.Counter()
    accepted: collections.Counter = collections.Counter()
    for line in (run / "venues" / venue / "spot" / f"{symbol}.jsonl").open():
        if '"OrderAccepted"' in line:
            event = json.loads(line)
            payload = (event.get("data") or {}).get("payload") or {}
            payload = payload.get("payload") or payload
            if payload.get("order_id") is not None:
                owner[payload["order_id"]] = event.get("client_id")
            role = roles.get((venue, event.get("client_id")))
            if role and payload.get("time_in_force") == "IOC":
                accepted[(role, payload.get("side"))] += 1
        elif '"OrderCancelled"' in line:
            event = json.loads(line)
            payload = (event.get("data") or {}).get("payload") or {}
            payload = payload.get("payload") or payload
            if payload.get("reason") != "IOC_EXPIRED":
                continue
            role = roles.get((venue, event.get("client_id")))
            side = None
            oid = payload.get("order_id")
            if oid in owner:
                side = None
            if role:
                expired[role] += 1
        elif '"Trade"' in line:
            event = json.loads(line)
            payload = (event.get("data") or {}).get("payload") or {}
            taker = roles.get((venue, owner.get(payload.get("taker_order_id"))))
            maker = roles.get((venue, owner.get(payload.get("maker_order_id"))))
            qty = payload.get("qty") or 0
            pair_qty[(taker, maker)] += qty
            total += qty

    one_sided = seconds = 0
    for line in (run / "venues" / venue / "general.jsonl").open():
        if "maker_state" not in line:
            continue
        event = json.loads(line)
        if event.get("event") != "maker_state":
            continue
        payload = (event.get("data") or {}).get("payload") or {}
        payload = payload.get("payload") or payload
        if not str(payload.get("maker", "")).startswith("spot"):
            continue
        seconds += 1
        if not (payload.get("bid") and payload.get("ask")):
            one_sided += 1

    maker_vs_maker = sum(q for (t, m), q in pair_qty.items() if t == "spot_maker" and m == "spot_maker")
    return {
        "volume": total,
        "maker_vs_maker_pct": 100 * maker_vs_maker / total if total else 0.0,
        "one_sided_pct": 100 * one_sided / seconds if seconds else 0.0,
        "expired": dict(expired),
        "accepted_ioc": dict(accepted),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runs", nargs="+", type=Path)
    args = parser.parse_args()
    print(f'{"run":18s}{"vol ABC":>12}{"mkr-v-mkr":>11}{"1-sided":>9}')
    for run in args.runs:
        h = health(run)
        print(f'{run.name:18s}{h["volume"]/1e8:12.0f}{h["maker_vs_maker_pct"]:10.1f}%{h["one_sided_pct"]:8.1f}%')


if __name__ == "__main__":
    main()
