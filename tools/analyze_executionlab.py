"""Summarize paired immediate-versus-TWAP execution-lab JSONL results."""

from __future__ import annotations

import argparse
import json
import math
import random
import statistics
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any

JsonObject = dict[str, Any]


def parse_args() -> argparse.Namespace:
    """Parse study input and deterministic bootstrap settings."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="executionlab JSONL")
    parser.add_argument("--output", type=Path, help="JSON output; defaults to stdout")
    parser.add_argument(
        "--bootstrap-samples",
        type=int,
        default=20_000,
        help="deterministic paired-bootstrap draws; default 20000",
    )
    return parser.parse_args()


def load_rows(path: Path) -> list[JsonObject]:
    """Load nonempty JSONL rows and reject malformed records."""
    rows: list[JsonObject] = []
    with path.open(encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, start=1):
            if not line.strip():
                continue
            row = json.loads(line)
            if not isinstance(row, dict):
                raise TypeError(f"{path}:{line_number}: expected an object")
            rows.append(row)
    if not rows:
        raise ValueError(f"{path}: no rows")
    return rows


def reports(row: JsonObject) -> list[JsonObject]:
    """Read all parent reports, accepting legacy one-parent output."""
    value = row.get("reports")
    if value is None:
        value = [row.get("report")]
    if (
        not isinstance(value, list)
        or not value
        or not all(isinstance(report, dict) for report in value)
    ):
        raise TypeError("row has no nonempty report list")
    return value


def paired_rows(rows: list[JsonObject]) -> list[tuple[int, JsonObject, JsonObject]]:
    """Pair one immediate and one TWAP world for every seed."""
    grouped: dict[int, dict[str, JsonObject]] = defaultdict(dict)
    for row in rows:
        seed = row.get("seed")
        policy = row.get("policy")
        if not isinstance(seed, int):
            raise TypeError("row seed must be an integer")
        if policy not in {"immediate", "twap"}:
            raise ValueError(f"seed {seed}: unknown policy {policy!r}")
        if policy in grouped[seed]:
            raise ValueError(f"seed {seed}: duplicate {policy} row")
        grouped[seed][policy] = row

    pairs: list[tuple[int, JsonObject, JsonObject]] = []
    for seed in sorted(grouped):
        worlds = grouped[seed]
        if set(worlds) != {"immediate", "twap"}:
            raise ValueError(f"seed {seed}: incomplete immediate/TWAP pair")
        pairs.append((seed, worlds["immediate"], worlds["twap"]))
    return pairs


def finite_number(value: object, field: str) -> float:
    """Require a finite scalar measurement."""
    if not isinstance(value, int | float) or not math.isfinite(value):
        raise ValueError(f"report field {field} is missing or non-finite")
    return float(value)


def integer(value: object, field: str) -> int:
    """Require an integer report field without accepting booleans."""
    if not isinstance(value, int) or isinstance(value, bool):
        raise TypeError(f"report field {field} must be an integer")
    return value


def summarize_world(row: JsonObject) -> JsonObject:
    """Aggregate a policy world without turning invalid parents into zeros."""
    parent_reports = reports(row)
    target_shortfall_bps: list[float] = []
    filled_shortfall_bps: list[float] = []
    total_target = 0
    total_filled = 0
    total_unfilled = 0
    complete = 0
    valid = 0
    schedule: list[tuple[str, int, int]] = []

    for report in parent_reports:
        target = integer(report.get("TargetQty"), "TargetQty")
        filled = integer(report.get("FilledQty"), "FilledQty")
        unfilled = integer(report.get("UnfilledQty"), "UnfilledQty")
        decision_at = integer(report.get("DecisionAt"), "DecisionAt")
        side = report.get("Side")
        if target <= 0 or filled < 0 or unfilled < 0 or filled + unfilled != target:
            raise ValueError("invalid parent quantity accounting")
        if not isinstance(side, str) or decision_at <= 0:
            raise ValueError("missing parent side or decision timestamp")
        schedule.append((side, target, decision_at))
        total_target += target
        total_filled += filled
        total_unfilled += unfilled
        if unfilled == 0:
            complete += 1
        filled_shortfall_bps.append(
            finite_number(report.get("ShortfallBps"), "ShortfallBps")
        )
        if report.get("TargetShortfallValid") is True:
            valid += 1
            target_shortfall_bps.append(
                finite_number(report.get("TargetShortfallBps"), "TargetShortfallBps")
            )

    if valid != len(parent_reports):
        raise ValueError("world has an invalid terminal target-shortfall parent")
    return {
        "parent_count": len(parent_reports),
        "complete_parent_count": complete,
        "valid_target_shortfall_count": valid,
        "total_target_qty": total_target,
        "total_filled_qty": total_filled,
        "total_unfilled_qty": total_unfilled,
        "mean_filled_shortfall_bps": statistics.fmean(filled_shortfall_bps),
        "mean_target_shortfall_bps": statistics.fmean(target_shortfall_bps),
        "schedule": schedule,
    }


def percentile(values: list[float], probability: float) -> float:
    """Return a linearly interpolated percentile of finite values."""
    if not values or not 0 <= probability <= 1:
        raise ValueError("invalid percentile input")
    ordered = sorted(values)
    index = probability * (len(ordered) - 1)
    low = math.floor(index)
    high = math.ceil(index)
    if low == high:
        return ordered[low]
    return ordered[low] + (ordered[high] - ordered[low]) * (index - low)


def paired_bootstrap(differences: list[float], samples: int) -> tuple[float, float]:
    """Calculate a deterministic paired-bootstrap percentile interval."""
    if samples <= 0 or not differences:
        raise ValueError("bootstrap requires positive samples and differences")
    generator = random.Random(0)
    count = len(differences)
    means = [
        sum(differences[generator.randrange(count)] for _ in range(count)) / count
        for _ in range(samples)
    ]
    return percentile(means, 0.025), percentile(means, 0.975)


def sign_test(differences: list[float]) -> tuple[int, int, float | None]:
    """Return TWAP-lower/higher counts and an exact two-sided sign p-value."""
    lower = sum(value < 0 for value in differences)
    higher = sum(value > 0 for value in differences)
    nonzero = lower + higher
    if nonzero == 0:
        return lower, higher, None
    tail = min(lower, higher)
    probability = (
        sum(math.comb(nonzero, value) for value in range(tail + 1)) / 2**nonzero
    )
    return lower, higher, min(1.0, 2 * probability)


def summarize(rows: list[JsonObject], bootstrap_samples: int) -> JsonObject:
    """Summarize correctly paired worlds, rejecting schedule mismatches."""
    pairs = paired_rows(rows)
    immediate_worlds = [summarize_world(immediate) for _, immediate, _ in pairs]
    twap_worlds = [summarize_world(twap) for _, _, twap in pairs]
    for (seed, _, _), immediate, twap in zip(
        pairs, immediate_worlds, twap_worlds, strict=True
    ):
        if immediate["schedule"] != twap["schedule"]:
            raise ValueError(f"seed {seed}: parent schedules differ by policy")

    immediate_cost = [world["mean_target_shortfall_bps"] for world in immediate_worlds]
    twap_cost = [world["mean_target_shortfall_bps"] for world in twap_worlds]
    differences = [
        twap_value - immediate_value
        for twap_value, immediate_value in zip(twap_cost, immediate_cost, strict=True)
    ]
    ci_low, ci_high = paired_bootstrap(differences, bootstrap_samples)
    lower, higher, p_value = sign_test(differences)
    return {
        "schema_version": 1,
        "comparison": "twap_minus_immediate",
        "negative_difference_interpretation": "TWAP lower target shortfall",
        "seeds": [seed for seed, _, _ in pairs],
        "mean_immediate_target_shortfall_bps": statistics.fmean(immediate_cost),
        "mean_twap_target_shortfall_bps": statistics.fmean(twap_cost),
        "mean_twap_minus_immediate_bps": statistics.fmean(differences),
        "bootstrap_95_ci_twap_minus_immediate_bps": [ci_low, ci_high],
        "twap_lower_count": lower,
        "twap_higher_count": higher,
        "two_sided_sign_test_p": p_value,
        "data_quality": {
            "parent_count_per_world": sorted(
                {world["parent_count"] for world in immediate_worlds + twap_worlds}
            ),
            "minimum_complete_parent_count": min(
                world["complete_parent_count"]
                for world in immediate_worlds + twap_worlds
            ),
            "maximum_total_unfilled_qty": max(
                world["total_unfilled_qty"] for world in immediate_worlds + twap_worlds
            ),
        },
    }


def main() -> int:
    """Write a summary or a data-quality failure."""
    args = parse_args()
    try:
        if args.bootstrap_samples <= 0:
            raise ValueError("bootstrap samples must be positive")
        result = summarize(load_rows(args.input), args.bootstrap_samples)
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        print(f"analyze_executionlab: {error}", file=sys.stderr)
        return 2
    encoded = json.dumps(result, indent=2, sort_keys=True)
    if args.output:
        args.output.write_text(f"{encoded}\n", encoding="utf-8")
    else:
        print(encoded)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
