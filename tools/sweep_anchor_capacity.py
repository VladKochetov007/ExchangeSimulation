"""Sweep informed-trader capacity against how far price leaves fundamental value.

FFA-10 predicts that anchoring is capacity limited: market makers quote around
their own inventory and noise traders hold no view, so only informed
participants pull price back toward the exogenous fundamental value. If that is
the mechanism, mispricing should fall as informed participants are added and
rise as their edge threshold widens, and the no-informed-flow arm should be the
worst.

Each cell is one simulated run. The simulator computes the mispricing summary
itself, so runs need no raw logs.

Usage:
    python tools/sweep_anchor_capacity.py --binary .cache/multivenue \
        --base research/ffa-ecology-control-2026-08-15.json \
        --out logs/research/anchor-sweep.jsonl
"""

from __future__ import annotations

import argparse
import json
import statistics
import subprocess
import tempfile
from pathlib import Path


def run_cell(binary: Path, base: dict, overrides: dict, duration: str, workdir: Path) -> list[dict]:
    config = dict(base)
    config.update(overrides)
    config["log_mode"] = "none"
    config_path = workdir / "config.json"
    config_path.write_text(json.dumps(config))
    logdir = workdir / "logs"
    result = subprocess.run(
        [str(binary), f"-config={config_path}", f"-duration={duration}", f"-logdir={logdir}"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(f"run failed for {overrides}: {result.stderr.strip().splitlines()[-1:]}")
    report = json.loads((logdir / "greeks.json").read_text())
    return report["mispricing"]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--base", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--duration", default="1h")
    parser.add_argument("--seeds", type=int, nargs="+", default=[91, 92, 93])
    parser.add_argument("--counts", type=int, nargs="+", default=[0, 1, 2, 4, 8])
    parser.add_argument("--edges", type=int, nargs="+", default=[10, 50])
    args = parser.parse_args()

    base = json.loads(args.base.read_text())
    args.out.parent.mkdir(parents=True, exist_ok=True)
    rows: list[dict] = []

    with tempfile.TemporaryDirectory(dir=".cache") as tmp:
        workdir = Path(tmp)
        for count in args.counts:
            for edge in args.edges:
                for seed in args.seeds:
                    cell = {"value_trader_count": count, "value_trader_edge_bps": edge, "seed": seed}
                    venues = run_cell(args.binary, base, cell, args.duration, workdir)
                    for venue in venues:
                        rows.append({**cell, "duration": args.duration, **venue})
                    means = [v["mean_abs_log_deviation"] for v in venues]
                    print(f"count={count:2d} edge={edge:3d} seed={seed}  mean|dev|={statistics.mean(means):.4f}")

    with args.out.open("w") as handle:
        for row in rows:
            handle.write(json.dumps(row) + "\n")

    print(f"\nwrote {len(rows)} venue-rows to {args.out}\n")
    print(f"{'count':>5} {'edge':>5} {'mean|dev|':>10} {'max|dev|':>9} {'frac>band':>10} {'longest s':>10}")
    for count in args.counts:
        for edge in args.edges:
            cell = [r for r in rows if r["value_trader_count"] == count and r["value_trader_edge_bps"] == edge]
            if not cell:
                continue
            print(
                f"{count:>5} {edge:>5} "
                f"{statistics.mean(r['mean_abs_log_deviation'] for r in cell):>10.4f} "
                f"{statistics.mean(r['max_abs_log_deviation'] for r in cell):>9.4f} "
                f"{statistics.mean(r['fraction_beyond_band'] for r in cell):>10.3f} "
                f"{statistics.mean(r['longest_excursion_seconds'] for r in cell):>10.0f}"
            )


if __name__ == "__main__":
    main()
