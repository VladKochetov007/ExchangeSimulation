"""Profile a configuration before comparisons are run on it.

Four experiments in this campaign were undermined not by their hypothesis but
by a property of the base configuration nobody had measured: a class that never
traded, a class left unmetered, budgets that collapsed the book, a threshold
that erased the arbitrage under study. Each was discovered one failed arm at a
time.

This prints what a configuration actually does, so a comparison can be checked
against it first: which classes are active, how much they trade, what the venue
did with their requests, and whether the market survived at all.

Usage:
    python tools/characterize_run.py --run logs/<name>
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


def payoffs(run: Path) -> dict[str, float]:
    out = subprocess.run(
        [sys.executable, "tools/population_payoff.py", "--report", str(run / "greeks.json")],
        capture_output=True, text=True,
    ).stdout
    results: dict[str, float] = {}
    for line in out.splitlines():
        parts = line.split()
        if len(parts) >= 4 and parts[0].islower() and "_" in parts[0]:
            try:
                results[parts[0]] = float(parts[-1].replace(",", ""))
            except ValueError:
                continue
    return results


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True, type=Path)
    parser.add_argument("--min-requests", type=int, default=1,
                        help="classes below this are reported as inactive")
    args = parser.parse_args()

    report_path = args.run / "greeks.json"
    if not report_path.exists():
        print(f"{args.run.name}: no report — the run did not complete")
        return
    report = json.loads(report_path.read_text())

    print(f"# {args.run.name}")

    # Request activity and how the venue answered it.
    budgets = report.get("request_budgets") or []
    activity: dict[str, dict[str, int]] = {}
    for row in budgets:
        entry = activity.setdefault(row["role"], {"admitted": 0, "refused": 0})
        entry["admitted"] += row["admitted"]
        entry["refused"] += row["rate_limited"] + row["overloaded"]

    result = payoffs(args.run)
    classes = sorted(set(activity) | set(result))

    print(f"\n{'class':24s} {'requests':>10s} {'refused':>8s} {'per member':>14s}  note")
    for name in classes:
        entry = activity.get(name, {})
        requests = entry.get("admitted", 0) + entry.get("refused", 0)
        refused = entry.get("refused", 0)
        payoff = result.get(name)
        # A class that never traded has no active result at all. That is a
        # stronger signal than a low request count, since a market maker
        # legitimately expresses a standing quote with very few messages.
        note = ""
        if payoff is not None and payoff == 0.0:
            note = "NEVER TRADED — cannot be compared in this configuration"
        elif budgets and requests < args.min_requests:
            note = "barely active"
        elif budgets and refused == 0 and requests > 0:
            note = "never refused"
        payoff_text = f"{payoff:,.2f}" if payoff is not None else "-"
        print(f"{name:24s} {requests:10d} {refused:8d} {payoff_text:>14s}  {note}")

    if not budgets:
        print("\nNo request budgets configured, so request activity is unavailable.")
        print("A configuration cannot be checked for inactive classes without it:")
        print("set rate_limit_tiers with generous budgets purely to obtain the counts.")


if __name__ == "__main__":
    main()
