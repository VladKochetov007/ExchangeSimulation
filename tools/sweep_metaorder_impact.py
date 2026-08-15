"""Test whether metaorder impact follows a square-root law.

arXiv:1105.1694 reports impact growing as the square root of executed size,
and — the part that actually discriminates between explanations — an exponent
that does not depend on the participation rate. If impact were simple
mechanical depletion of a static book, executing the same quantity faster would
move the price further and the exponent would drift with participation.

Each cell runs the simulator with one participation rate. Impact is measured
per metaorder as the signed mid-to-mid move over the execution, binned by
executed quantity, with the median taken per bin because impact is heavy
tailed.

Usage:
    python tools/sweep_metaorder_impact.py --binary .cache/multivenue \
        --base research/ffa-metaorder-impact-2026-08-16.json \
        --out logs/research/metaorder-impact.jsonl
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
import subprocess
import tempfile
from pathlib import Path


def run_cell(binary: Path, base: dict, rate: float, seed: int, duration: str, workdir: Path) -> list[dict]:
    config = json.loads(json.dumps(base))
    config["log_mode"] = "none"
    config["seed"] = seed
    config["metaorder_traders"]["participation_rate"] = rate
    (workdir / "config.json").write_text(json.dumps(config))
    logdir = workdir / "logs"
    result = subprocess.run(
        [str(binary), f"-config={workdir / 'config.json'}", f"-duration={duration}", f"-logdir={logdir}"],
        capture_output=True, text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(f"rate={rate} seed={seed}: {result.stderr.strip().splitlines()[-1:]}")
    return json.loads((logdir / "greeks.json").read_text()).get("metaorders") or []


def fit_exponent(records: list[dict], bins: int = 5) -> tuple[float, float, list[tuple[float, float, int]]]:
    """Median impact per size bin, then a log-log least-squares slope."""
    usable = [r for r in records if r["filled_qty"] > 0 and r["start_mid"] > 0]
    usable.sort(key=lambda r: r["filled_qty"])
    points: list[tuple[float, float, int]] = []
    n = len(usable)
    for i in range(bins):
        group = usable[i * n // bins:(i + 1) * n // bins]
        if len(group) < 3:
            continue
        q = statistics.median(r["filled_qty"] for r in group)
        impact = statistics.median(r["signed_impact"] for r in group)
        if q > 0 and impact > 0:
            points.append((q, impact, len(group)))
    if len(points) < 3:
        return float("nan"), float("nan"), points
    xs = [math.log(p[0]) for p in points]
    ys = [math.log(p[1]) for p in points]
    mean_x, mean_y = sum(xs) / len(xs), sum(ys) / len(ys)
    sxx = sum((x - mean_x) ** 2 for x in xs)
    slope = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys)) / sxx if sxx else float("nan")
    intercept = mean_y - slope * mean_x
    ss_tot = sum((y - mean_y) ** 2 for y in ys)
    ss_res = sum((y - (intercept + slope * x)) ** 2 for x, y in zip(xs, ys))
    r2 = 1 - ss_res / ss_tot if ss_tot else float("nan")
    return slope, r2, points


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--base", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--duration", default="4h")
    parser.add_argument("--rates", type=float, nargs="+", default=[0.02, 0.05, 0.10, 0.25])
    parser.add_argument("--seeds", type=int, nargs="+", default=[91, 92])
    args = parser.parse_args()

    base = json.loads(args.base.read_text())
    args.out.parent.mkdir(parents=True, exist_ok=True)
    rows: list[dict] = []
    failed: list[dict] = []

    with tempfile.TemporaryDirectory(dir=".cache") as tmp:
        workdir = Path(tmp)
        for rate in args.rates:
            for seed in args.seeds:
                try:
                    records = run_cell(args.binary, base, rate, seed, args.duration, workdir)
                except RuntimeError as failure:
                    # A cell can fail because the metaorder flow exhausts the
                    # book. That is information about capacity, not a reason to
                    # drop the cell silently.
                    failed.append({"participation_rate": rate, "seed": seed, "error": str(failure)})
                    print(f"rate={rate:.2f} seed={seed}: FAILED")
                    continue
                for record in records:
                    rows.append({"participation_rate": rate, "seed": seed, **record})
                print(f"rate={rate:.2f} seed={seed}: {len(records)} metaorders")

    with args.out.open("w") as handle:
        for row in rows:
            handle.write(json.dumps(row) + "\n")

    if failed:
        print(f"\n{len(failed)} cells failed:")
        for cell in failed:
            print(f"  rate={cell['participation_rate']:.2f} seed={cell['seed']}")
    print(f"\nwrote {len(rows)} metaorders to {args.out}\n")
    print(f"{'rate':>6} {'n':>6} {'realised':>9} {'exponent':>9} {'R^2':>7}  bins (Q in base units -> impact bps)")
    for rate in args.rates:
        cell = [r for r in rows if r["participation_rate"] == rate]
        slope, r2, points = fit_exponent(cell)
        realised = statistics.median([r["realized_participation"] for r in cell if r["realized_participation"] > 0] or [0])
        bins = "  ".join(f"{p[0] / 1e8:.4f}->{p[1] * 1e4:.2f}" for p in points)
        print(f"{rate:>6.2f} {len(cell):>6} {realised:>9.3f} {slope:>9.3f} {r2:>7.3f}  {bins}")


if __name__ == "__main__":
    main()
