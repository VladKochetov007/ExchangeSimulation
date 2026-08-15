"""Test the Wyart-Bouchaud relation between spread and volatility per trade.

arXiv:physics/0603084 derives, from a marginal-profitability condition on
market making, that the bid-ask spread should be proportional to the volatility
measured *per trade* rather than per unit time, with a constant of order unity
and a tight fit.

The test is not one run: it is whether a scatter of runs with different
activity levels falls on a single line. Crucially, sweeping different controls
must trace the *same* line. If varying maker risk aversion and varying noise
intensity give different slopes, there is no universal relation in this market.

Usage:
    python tools/sweep_spread_volatility.py --binary .cache/multivenue \
        --base research/ffa-ecology-control-2026-08-15.json \
        --out logs/research/spread-volatility.jsonl
"""

from __future__ import annotations

import argparse
import json
import subprocess
import tempfile
from pathlib import Path

# Each control is swept separately so their slopes can be compared.
CONTROLS: dict[str, tuple[str, list]] = {
    "noise_intensity": ("noise_trader_count", [1, 2, 4, 8]),
    # Recalibrated after the market-data subscription fix: above ~1e-4 the
    # market diverges, below ~2e-6 the spread is tick-floor bound.
    "maker_risk_aversion": ("stoikov_risk_aversion", [1e-6, 2e-6, 5e-6, 1e-5]),
    "fundamental_vol": ("fundamental_log_vol_per_step", [0.00005, 0.0001, 0.0002, 0.0004]),
    "informed_count": ("value_trader_count", [0, 1, 2, 4]),
}


def run_cell(binary: Path, base: dict, overrides: dict, duration: str, workdir: Path) -> tuple[list, list]:
    config = dict(base)
    config.update(overrides)
    config["log_mode"] = "none"
    (workdir / "config.json").write_text(json.dumps(config))
    logdir = workdir / "logs"
    result = subprocess.run(
        [str(binary), f"-config={workdir / 'config.json'}", f"-duration={duration}", f"-logdir={logdir}"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(f"{overrides}: {result.stderr.strip().splitlines()[-1:]}")
    report = json.loads((logdir / "greeks.json").read_text())
    return report["microstructure"], report["mispricing"]


def ols(xs: list[float], ys: list[float]) -> tuple[float, float, float]:
    """Slope through the origin, intercept-model slope, and R^2 of the latter."""
    n = len(xs)
    mean_x, mean_y = sum(xs) / n, sum(ys) / n
    sxx = sum((x - mean_x) ** 2 for x in xs)
    sxy = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys))
    slope = sxy / sxx if sxx else float("nan")
    intercept = mean_y - slope * mean_x
    ss_tot = sum((y - mean_y) ** 2 for y in ys)
    ss_res = sum((y - (intercept + slope * x)) ** 2 for x, y in zip(xs, ys))
    r2 = 1 - ss_res / ss_tot if ss_tot else float("nan")
    through_origin = sum(x * y for x, y in zip(xs, ys)) / sum(x * x for x in xs)
    return through_origin, slope, r2


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--base", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--duration", default="30m")
    parser.add_argument("--seeds", type=int, nargs="+", default=[91, 92])
    args = parser.parse_args()

    base = json.loads(args.base.read_text())
    args.out.parent.mkdir(parents=True, exist_ok=True)
    rows: list[dict] = []

    with tempfile.TemporaryDirectory(dir=".cache") as tmp:
        workdir = Path(tmp)
        for control, (field, values) in CONTROLS.items():
            for value in values:
                for seed in args.seeds:
                    micro, mis = run_cell(args.binary, base, {field: value, "seed": seed}, args.duration, workdir)
                    for m, p in zip(micro, mis):
                        rows.append({
                            "control": control, "field": field, "value": value, "seed": seed,
                            "venue_id": m["venue_id"], "trades": m["trades"],
                            "mean_relative_spread": m["mean_relative_spread"],
                            "mean_spread_ticks": m["mean_spread_ticks"],
                            "sigma_per_trade": m["sigma_per_trade"],
                            "sigma_per_sample": m["sigma_per_sample"],
                            "trades_per_sample": m["trades_per_sample"],
                            "mean_abs_log_deviation": p["mean_abs_log_deviation"],
                        })
                    print(f"{control}={value} seed={seed}: "
                          f"S={sum(m['mean_relative_spread'] for m in micro) / len(micro):.6f} "
                          f"sigma1={sum(m['sigma_per_trade'] for m in micro) / len(micro):.6f}")

    with args.out.open("w") as handle:
        for row in rows:
            handle.write(json.dumps(row) + "\n")

    print(f"\nwrote {len(rows)} rows to {args.out}\n")
    print(f"{'control':>20} {'n':>4} {'c (origin)':>11} {'slope':>10} {'R^2':>7}")
    for control in CONTROLS:
        cell = [r for r in rows if r["control"] == control and r["sigma_per_trade"] > 0]
        if len(cell) < 3:
            continue
        xs = [r["sigma_per_trade"] for r in cell]
        ys = [r["mean_relative_spread"] for r in cell]
        origin, slope, r2 = ols(xs, ys)
        print(f"{control:>20} {len(cell):>4} {origin:>11.2f} {slope:>10.2f} {r2:>7.3f}")

    pooled = [r for r in rows if r["sigma_per_trade"] > 0]
    origin, slope, r2 = ols([r["sigma_per_trade"] for r in pooled], [r["mean_relative_spread"] for r in pooled])
    print(f"{'POOLED':>20} {len(pooled):>4} {origin:>11.2f} {slope:>10.2f} {r2:>7.3f}")


if __name__ == "__main__":
    main()
