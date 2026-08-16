"""Measure each strategy's result against different opponent mixtures.

A ranking inside one population is not a non-transitive structure. What would
show one is a strategy that earns against opponent A, loses against opponent B,
while B loses to A. So this runs a focal strategy against several opponent
mixtures on a fixed substrate of makers and noise, and reports the focal
strategy's result per member in each.

Everything is measured per member so that mixtures with different participant
counts stay comparable.

Usage:
    python tools/sweep_population_matrix.py --binary .cache/multivenue \
        --base research/ffa-ecology-population-2026-08-16.json --out logs/research/matrix.jsonl
"""

from __future__ import annotations

import argparse
import collections
import json
import statistics
import subprocess
import tempfile
from pathlib import Path

PRECISION = {"USD": 100_000, "ABC": 100_000_000, "CDF": 100_000_000}

# Each opponent mixture sets the counts of every optional class, so a cell
# differs only in who is present.
MIXTURES: dict[str, dict[str, int]] = {
    "noise_only": {"value_trader_count": 0, "carry_arbitrageur_count": 0, "metaorder_trader_count": 0, "round_trip_trader_count": 0, "elastic_supplier_count": 0},
    "informed": {"value_trader_count": 6, "carry_arbitrageur_count": 0, "metaorder_trader_count": 0, "round_trip_trader_count": 0, "elastic_supplier_count": 0},
    "carry": {"value_trader_count": 0, "carry_arbitrageur_count": 6, "metaorder_trader_count": 0, "round_trip_trader_count": 0, "elastic_supplier_count": 0},
    "execution": {"value_trader_count": 0, "carry_arbitrageur_count": 0, "metaorder_trader_count": 6, "round_trip_trader_count": 0, "elastic_supplier_count": 0},
}

# The focal strategy is added to each mixture at a fixed small size.
FOCALS: dict[str, tuple[str, int]] = {
    "value_trader": ("value_trader_count", 2),
    "carry_arb": ("carry_arbitrageur_count", 2),
    "metaorder_trader": ("metaorder_trader_count", 2),
}


def role_group(role: str) -> str:
    head, _, tail = role.rpartition("_")
    return head if head and tail.isdigit() else role


def per_member(report: dict) -> dict[str, float]:
    initial = {(row["venue_id"], row["role"]): row for row in report["initial_accounts"]}
    total: dict[str, float] = collections.defaultdict(float)
    members: dict[str, int] = collections.defaultdict(int)
    for row in report["terminal_accounts"]:
        start = initial.get((row["venue_id"], row["role"]))
        if start is None:
            continue
        marks = row.get("marks") or {}
        benchmark = 0
        for wallet in ("spot_balances", "perp_balances"):
            for balance in start["account"].get(wallet) or []:
                precision = PRECISION.get(balance["asset"])
                if precision:
                    benchmark += balance["net_asset"] * marks.get(balance["asset"], 0) // precision
        group = role_group(row["role"])
        total[group] += (row["account"]["equity"] - benchmark) / PRECISION["USD"]
        members[group] += 1
    return {group: total[group] / members[group] for group in total}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--base", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--duration", default="12h")
    parser.add_argument("--seeds", type=int, nargs="+", default=[91, 92])
    parser.add_argument("--focals", nargs="+", default=list(FOCALS))
    args = parser.parse_args()

    base = json.loads(args.base.read_text())
    args.out.parent.mkdir(parents=True, exist_ok=True)
    rows: list[dict] = []

    with tempfile.TemporaryDirectory(dir=".cache") as tmp:
        workdir = Path(tmp)
        for focal in args.focals:
            field, count = FOCALS[focal]
            for mixture, counts in MIXTURES.items():
                results: list[float] = []
                for seed in args.seeds:
                    config = json.loads(json.dumps(base))
                    config.update(counts)
                    config["log_mode"] = "none"
                    config["seed"] = seed
                    # The focal strategy is added on top of the mixture.
                    config[field] = config.get(field, 0) + count
                    (workdir / "config.json").write_text(json.dumps(config))
                    logdir = workdir / "logs"
                    outcome = subprocess.run(
                        [str(args.binary), f"-config={workdir / 'config.json'}",
                         f"-duration={args.duration}", f"-logdir={logdir}"],
                        capture_output=True, text=True)
                    if outcome.returncode != 0:
                        print(f"{focal} vs {mixture} seed {seed}: FAILED")
                        continue
                    report = json.loads((logdir / "greeks.json").read_text())
                    results.append(per_member(report).get(focal, float("nan")))
                if results:
                    rows.append({"focal": focal, "mixture": mixture, "results": results,
                                 "median": statistics.median(results)})
                    print(f"{focal:<18} vs {mixture:<12} {statistics.median(results):>14,.0f}")

    with args.out.open("w") as handle:
        for row in rows:
            handle.write(json.dumps(row) + "\n")

    print(f"\n{'focal':<18}" + "".join(f"{m:>14}" for m in MIXTURES))
    for focal in args.focals:
        cells = {row["mixture"]: row["median"] for row in rows if row["focal"] == focal}
        print(f"{focal:<18}" + "".join(f"{cells.get(m, float('nan')):>14,.0f}" for m in MIXTURES))


if __name__ == "__main__":
    main()
