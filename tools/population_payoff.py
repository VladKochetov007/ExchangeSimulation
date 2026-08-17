"""Report what each participant class earned, net of inventory revaluation.

The campaign's question is which strategies survive against which opponents, so
the quantity that matters is trading result rather than marked wealth. Every
participant here is net long the base asset, so a rising price lifts all of
them at once; comparing raw equity changes would rank participants by how much
inventory they happened to hold.

Active result is therefore measured against a passive benchmark: the
participant's own initial holdings valued at the same terminal marks. What
remains is what its trading did.

Usage:
    python tools/population_payoff.py --report <run>/greeks.json
"""

from __future__ import annotations

import argparse
import collections
import json
from pathlib import Path

PRECISION = {"USD": 100_000, "ABC": 100_000_000, "CDF": 100_000_000}


def role_group(role: str) -> str:
    """Collapse numbered participants of the same class."""
    head, _, tail = role.rpartition("_")
    return head if head and tail.isdigit() else role


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()

    report = json.loads(args.report.read_text())
    initial = {(row["venue_id"], row["role"]): row for row in report["initial_accounts"]}

    active: dict[str, float] = collections.defaultdict(float)
    members: dict[str, int] = collections.defaultdict(int)
    for row in report["terminal_accounts"]:
        start = initial.get((row["venue_id"], row["role"]))
        if start is None:
            continue
        marks = row.get("marks") or {}
        benchmark = 0
        for wallet in ("spot_balances", "perp_balances"):
            for balance in start["account"].get(wallet) or []:
                asset = balance["asset"]
                precision = PRECISION.get(asset)
                if precision is None:
                    continue
                benchmark += balance["net_asset"] * marks.get(asset, 0) // precision
        group = role_group(row["role"])
        active[group] += (row["account"]["equity"] - benchmark) / PRECISION["USD"]
        members[group] += 1

    total = sum(active.values())

    # A bankrupt participant's negative balance is zeroed and charged to the
    # insurance fund, which writes off their debt. The population's summed
    # result therefore equals the fees it paid only when no one goes bankrupt;
    # otherwise it is raised by whatever the fund absorbed.
    fund_absorbed = 0.0
    fee_revenue = 0.0
    for ledger in report.get("venue_ledgers") or []:
        for asset, amount in (ledger.get("insurance_fund") or {}).items():
            if asset in PRECISION:
                # The fund goes negative as it absorbs bankrupt debt, so what it
                # paid out is the negation of its balance.
                fund_absorbed -= amount / PRECISION[asset]
        for asset, amount in (ledger.get("fee_revenue") or {}).items():
            if asset in PRECISION:
                fee_revenue += amount / PRECISION[asset]
    print(f"{'participant class':<26}{'active result (USD)':>24}{'members':>9}{'per member':>20}")
    for group, value in sorted(active.items(), key=lambda item: -item[1]):
        print(f"{group:<26}{value:>24,.2f}{members[group]:>9}{value / members[group]:>20,.2f}")
    print(f"{'':<26}{'-' * 24:>24}")
    print(f"{'population sum':<26}{total:>22,.2f}")
    if report.get("venue_ledgers"):
        print(f"{'venue fee revenue':<26}{fee_revenue:>22,.2f}")
        print(f"{'insurance fund paid out':<26}{fund_absorbed:>22,.2f}")
        print(f"{'residual (want ~0)':<26}{total + fee_revenue - fund_absorbed:>22,.2f}")
    else:
        print("  (no venue_ledgers in report; conservation cannot be checked)")


if __name__ == "__main__":
    main()
