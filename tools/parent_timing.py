"""Per-desk metaorder timing: execution duration and the gap between parents.

Records from several desks are merged in the run report, and record IDs restart
at one per desk. Grouping by venue alone interleaves desks and produces
overlapping intervals, so every sequential measure must group by TraderID.
"""
import argparse
import json
import statistics as st
from pathlib import Path


def timing(run: Path):
    report = json.loads((run / "greeks.json").read_text())
    parents = report.get("metaorders") or []
    if parents and "trader_id" not in parents[0]:
        raise SystemExit(f"{run}: records predate TraderID; per-desk timing is not recoverable")
    by_desk: dict[tuple, list] = {}
    for record in parents:
        by_desk.setdefault((record["venue_id"], record["trader_id"]), []).append(record)
    gaps, durations = [], []
    for records in by_desk.values():
        records.sort(key=lambda r: r["start_timestamp"])
        for record in records:
            if record.get("end_timestamp"):
                durations.append((record["end_timestamp"] - record["start_timestamp"]) / 1e9)
        for earlier, later in zip(records, records[1:]):
            if not earlier.get("end_timestamp"):
                continue
            gaps.append((later["start_timestamp"] - earlier["end_timestamp"]) / 1e9)
    return by_desk, gaps, durations


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runs", nargs="+", type=Path)
    args = parser.parse_args()
    print(f'{"run":28s}{"desks":>6}{"parents":>9}{"durMed":>9}{"gapMed":>9}{"gapMean":>9}{"gapMax":>9}{"neg":>5}')
    for run in args.runs:
        by_desk, gaps, durations = timing(run)
        negative = sum(1 for g in gaps if g < 0)
        print(f"{run.name:28s}{len(by_desk):6d}{sum(len(v) for v in by_desk.values()):9d}"
              f"{st.median(durations):9.2f}{st.median(gaps):9.2f}{st.mean(gaps):9.2f}"
              f"{max(gaps):9.2f}{negative:5d}")


if __name__ == "__main__":
    main()
