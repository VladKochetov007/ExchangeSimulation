"""Compute seed-paired treatment statistics from multivenue risk summaries."""

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

METRICS = (
    "mean_abs_net_delta",
    "mean_max_abs_net_delta",
    "mean_abs_gamma",
    "mean_abs_vega",
    "mean_equity_change",
    "mean_terminal_maintenance",
)


def parse_args() -> argparse.Namespace:
    """Parse one world-summary JSONL file and statistical output options."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="world-summary JSONL")
    parser.add_argument("--output", type=Path, help="JSON output; defaults to stdout")
    parser.add_argument(
        "--bootstrap-samples",
        type=int,
        default=20_000,
        help="deterministic paired-bootstrap draws; default 20000",
    )
    return parser.parse_args()


def load_worlds(path: Path) -> list[JsonObject]:
    """Read nonempty JSONL rows and reject malformed study inputs."""
    rows: list[JsonObject] = []
    with path.open(encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, start=1):
            if not line.strip():
                continue
            value = json.loads(line)
            if not isinstance(value, dict):
                raise TypeError(f"{path}:{line_number}: expected an object")
            rows.append(value)
    if not rows:
        raise ValueError(f"{path}: no world rows")
    return rows


def paired_worlds(rows: list[JsonObject]) -> list[tuple[int, JsonObject, JsonObject]]:
    """Return one on/off pair per seed, rejecting missing or duplicate arms."""
    by_seed: dict[int, dict[str, JsonObject]] = defaultdict(dict)
    for row in rows:
        seed = row.get("seed")
        arm = row.get("dealer_hedge_mode")
        if not isinstance(seed, int):
            raise TypeError("world seed must be an integer")
        if arm not in {"on", "off"}:
            raise ValueError(f"seed {seed}: invalid hedge arm {arm!r}")
        if arm in by_seed[seed]:
            raise ValueError(f"seed {seed}: duplicate hedge arm {arm}")
        by_seed[seed][arm] = row

    pairs: list[tuple[int, JsonObject, JsonObject]] = []
    for seed in sorted(by_seed):
        arms = by_seed[seed]
        if set(arms) != {"on", "off"}:
            raise ValueError(f"seed {seed}: incomplete treatment pair")
        pairs.append((seed, arms["on"], arms["off"]))
    return pairs


def metric(row: JsonObject, name: str) -> float:
    """Read a finite world metric, rejecting missing telemetry."""
    world = row.get("world")
    if not isinstance(world, dict):
        raise TypeError("world row has no world aggregate")
    value = world.get(name)
    if not isinstance(value, int | float) or not math.isfinite(value):
        raise ValueError(f"world metric {name} is missing or non-finite")
    return float(value)


def percentile(values: list[float], probability: float) -> float:
    """Linearly interpolate a percentile of an already finite sample."""
    if not values or probability < 0 or probability > 1:
        raise ValueError("invalid percentile input")
    ordered = sorted(values)
    index = probability * (len(ordered) - 1)
    lower = math.floor(index)
    upper = math.ceil(index)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] + (ordered[upper] - ordered[lower]) * (index - lower)


def paired_bootstrap_interval(
    differences: list[float], samples: int
) -> tuple[float, float]:
    """Return a deterministic percentile interval over resampled seed pairs."""
    if samples <= 0:
        raise ValueError("bootstrap samples must be positive")
    if not differences:
        raise ValueError("cannot bootstrap empty paired differences")
    generator = random.Random(0)
    count = len(differences)
    means = [
        sum(differences[generator.randrange(count)] for _ in range(count)) / count
        for _ in range(samples)
    ]
    return percentile(means, 0.025), percentile(means, 0.975)


def two_sided_sign_test(differences: list[float]) -> tuple[int, int, float | None]:
    """Return lower/higher counts and an exact two-sided sign-test p-value."""
    lower = sum(value < 0 for value in differences)
    higher = sum(value > 0 for value in differences)
    nonzero = lower + higher
    if nonzero == 0:
        return lower, higher, None
    tail = min(lower, higher)
    probability = (
        sum(math.comb(nonzero, observed) for observed in range(tail + 1)) / 2**nonzero
    )
    return lower, higher, min(1.0, 2 * probability)


def summarize_metric(
    pairs: list[tuple[int, JsonObject, JsonObject]], name: str, bootstrap_samples: int
) -> JsonObject:
    """Calculate a treatment contrast where negative means hedge-on is lower."""
    hedge_on = [metric(on, name) for _, on, _ in pairs]
    hedge_off = [metric(off, name) for _, _, off in pairs]
    differences = [on - off for on, off in zip(hedge_on, hedge_off, strict=True)]
    ci_low, ci_high = paired_bootstrap_interval(differences, bootstrap_samples)
    lower, higher, sign_p = two_sided_sign_test(differences)
    return {
        "metric": name,
        "n_pairs": len(pairs),
        "mean_hedge_on": statistics.fmean(hedge_on),
        "mean_hedge_off": statistics.fmean(hedge_off),
        "mean_on_minus_off": statistics.fmean(differences),
        "median_on_minus_off": statistics.median(differences),
        "bootstrap_95_ci_on_minus_off": [ci_low, ci_high],
        "hedge_on_lower_count": lower,
        "hedge_on_higher_count": higher,
        "two_sided_sign_test_p": sign_p,
    }


def summarize(
    pairs: list[tuple[int, JsonObject, JsonObject]], bootstrap_samples: int
) -> JsonObject:
    """Summarize paired treatment worlds and their telemetry completeness."""
    return {
        "schema_version": 1,
        "comparison": "hedge_on_minus_hedge_off",
        "negative_difference_interpretation": "hedge-on lower",
        "seeds": [seed for seed, _, _ in pairs],
        "metrics": [
            summarize_metric(pairs, name, bootstrap_samples) for name in METRICS
        ],
        "data_quality": {
            "min_positive_horizon_position_rows": min(
                int(row["world"]["positive_horizon_position_rows"])
                for _, on, off in pairs
                for row in (on, off)
            ),
            "venue_count_per_world": sorted(
                {
                    int(row["world"]["venue_count"])
                    for _, on, off in pairs
                    for row in (on, off)
                }
            ),
        },
    }


def main() -> int:
    """Write deterministic paired statistics or a clear data-quality error."""
    args = parse_args()
    try:
        if args.bootstrap_samples <= 0:
            raise ValueError("bootstrap samples must be positive")
        result = summarize(
            paired_worlds(load_worlds(args.input)), args.bootstrap_samples
        )
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        print(f"analyze_paired_multivenue_risk: {error}", file=sys.stderr)
        return 2
    encoded = json.dumps(result, indent=2, sort_keys=True)
    if args.output:
        args.output.write_text(f"{encoded}\n", encoding="utf-8")
    else:
        print(encoded)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
