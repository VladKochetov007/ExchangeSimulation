"""Summarise the spot price path a run produced.

A purely order-driven market has no reference level, so the question is not how
far the price sits from anything but whether the path stays in a band, trends,
or oscillates. This reports the range, the terminal drift, and the largest
excursion, from the spot midpoints already captured in the risk timeline.

Usage:
    python tools/price_path.py logs/<run> [logs/<run> ...]
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def path_for(run: Path, venue: str = "north") -> list[float]:
    report = json.loads((run / "greeks.json").read_text())
    rows = report.get("risk_timeline", {}).get(venue, [])
    mids: list[float] = []
    for row in rows:
        profile = row.get("greek_profile") or {}
        mid = profile.get("spot_mid")
        if mid:
            mids.append(mid / 1e8)
    return mids


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runs", nargs="+", type=Path)
    parser.add_argument("--venue", default="north")
    args = parser.parse_args()

    for run in args.runs:
        mids = path_for(run, args.venue)
        if not mids:
            print(f"{run.name}: no spot midpoints in the timeline")
            continue
        start, end = mids[0], mids[-1]
        low, high = min(mids), max(mids)
        # Largest move away from the starting level, in either direction.
        excursion = max(abs(low - start), abs(high - start))
        # Realised volatility of the path itself. In a market where nobody knows
        # a value, this is an output of the population rather than an input.
        returns = [
            (b - a) / a for a, b in zip(mids, mids[1:]) if a > 0
        ]
        mean = sum(returns) / len(returns) if returns else 0.0
        variance = (
            sum((r - mean) ** 2 for r in returns) / (len(returns) - 1)
            if len(returns) > 1
            else 0.0
        )
        realised = variance ** 0.5
        print(
            f"{run.name:28s} samples={len(mids):5d} start={start:8.3f} end={end:8.3f} "
            f"drift={100*(end-start)/start:+7.2f}% band=[{low:.3f},{high:.3f}] "
            f"max excursion={100*excursion/start:6.2f}% "
            f"realised vol/sample={100*realised:7.4f}%"
        )


if __name__ == "__main__":
    main()
