"""Summarize exchange-owned multivenue risk telemetry for paired studies."""

from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any

JsonObject = dict[str, Any]  # JSON decoder necessarily returns heterogeneous values.


def finite_values(rows: list[JsonObject], field: str) -> list[float]:
    """Return finite numeric values from one Greek-profile field."""
    values: list[float] = []
    for row in rows:
        profile = row.get("greek_profile", {})
        value = profile.get(field)
        if isinstance(value, int | float) and math.isfinite(value):
            values.append(float(value))
    return values


def mean(values: list[float]) -> float | None:
    """Return the arithmetic mean, or None for an empty sample."""
    if not values:
        return None
    return sum(values) / len(values)


def numeric_values(rows: list[JsonObject], field: str) -> list[float]:
    """Return finite top-level numeric values from already summarized rows."""
    values: list[float] = []
    for row in rows:
        value = row.get(field)
        if isinstance(value, int | float) and math.isfinite(value):
            values.append(float(value))
    return values


def summarize_venue(venue_id: str, output: JsonObject) -> JsonObject:
    """Summarize one venue's risk path without inferring unobserved PnL."""
    initial = output["initial_risk"][venue_id]
    terminal = output["terminal_risk"][venue_id]
    timeline: list[JsonObject] = output["risk_timeline"][venue_id]

    net_delta = finite_values(timeline, "net_delta")
    gamma = finite_values(timeline, "gamma")
    vega = finite_values(timeline, "vega")
    positive_horizon_positions = sum(
        1
        for row in timeline
        for position in row.get("greek_positions", [])
        if position.get("time_to_expiry_nano", 0) > 0
    )
    initial_equity = initial["account"]["equity"]
    terminal_equity = terminal["account"]["equity"]
    return {
        "venue_id": venue_id,
        "timeline_rows": len(timeline),
        "positive_horizon_position_rows": positive_horizon_positions,
        "initial_equity": initial_equity,
        "terminal_equity": terminal_equity,
        "equity_change": terminal_equity - initial_equity,
        "mean_abs_net_delta": mean([abs(value) for value in net_delta]),
        "max_abs_net_delta": max((abs(value) for value in net_delta), default=None),
        "mean_abs_gamma": mean([abs(value) for value in gamma]),
        "max_abs_gamma": max((abs(value) for value in gamma), default=None),
        "mean_abs_vega": mean([abs(value) for value in vega]),
        "max_abs_vega": max((abs(value) for value in vega), default=None),
        "terminal_maintenance": terminal["account"]["maintenance"],
    }


def summarize_world(venues: list[JsonObject]) -> JsonObject:
    """Aggregate venue summaries without treating venues as independent seeds."""
    return {
        "venue_count": len(venues),
        "positive_horizon_position_rows": sum(
            int(row["positive_horizon_position_rows"]) for row in venues
        ),
        "mean_equity_change": mean(numeric_values(venues, "equity_change")),
        "mean_abs_net_delta": mean(numeric_values(venues, "mean_abs_net_delta")),
        "mean_max_abs_net_delta": mean(numeric_values(venues, "max_abs_net_delta")),
        "mean_abs_gamma": mean(numeric_values(venues, "mean_abs_gamma")),
        "mean_abs_vega": mean(numeric_values(venues, "mean_abs_vega")),
        "mean_terminal_maintenance": mean(
            numeric_values(venues, "terminal_maintenance")
        ),
    }


def load_manifest_config(path: Path) -> JsonObject:
    """Load run configuration that identifies a replicated treatment world."""
    manifest_path = path.parent / "manifest.json"
    with manifest_path.open(encoding="utf-8") as handle:
        manifest: JsonObject = json.load(handle)
    config = manifest.get("config")
    if not isinstance(config, dict):
        raise TypeError(f"{manifest_path}: missing config")
    return config


def summarize_file(path: Path) -> JsonObject:
    """Load and summarize one schema-v3 greeks.json output.

    Args:
        path: Multivenue greeks.json path.

    Returns:
        A JSON-serializable world summary.

    Raises:
        ValueError: If the file lacks the strict exchange-owned schema.
    """
    with path.open(encoding="utf-8") as handle:
        output: JsonObject = json.load(handle)
    if output.get("schema_version") != 3:
        raise ValueError(f"{path}: expected schema_version 3")
    venue_ids = sorted(output.get("initial_risk", {}))
    if not venue_ids:
        raise ValueError(f"{path}: no initial_risk venue rows")
    summaries = [summarize_venue(venue_id, output) for venue_id in venue_ids]
    config = load_manifest_config(path)
    seed = config.get("seed")
    hedge_mode = config.get("dealer_hedge_mode")
    log_mode = config.get("log_mode")
    if not isinstance(seed, int):
        raise TypeError(
            f"{path.parent / 'manifest.json'}: config seed is not an integer"
        )
    if hedge_mode not in {"on", "off"}:
        raise ValueError(f"{path.parent / 'manifest.json'}: invalid dealer hedge mode")
    if log_mode not in {"full", "none"}:
        raise ValueError(f"{path.parent / 'manifest.json'}: invalid log mode")
    return {
        "source": str(path),
        "seed": seed,
        "dealer_hedge_mode": hedge_mode,
        "log_mode": log_mode,
        "venues": summaries,
        "world": summarize_world(summaries),
    }


def parse_args() -> argparse.Namespace:
    """Parse the explicit input files and optional JSONL destination."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "input", nargs="+", type=Path, help="schema-v3 greeks.json files"
    )
    parser.add_argument(
        "--output", type=Path, help="JSONL output path; default is stdout"
    )
    return parser.parse_args()


def main() -> int:
    """Write one reproducible world summary per input file."""
    args = parse_args()
    try:
        rows = [summarize_file(path) for path in args.input]
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        print(f"summarize_multivenue_risk: {error}", file=sys.stderr)
        return 2
    destination = args.output.open("w", encoding="utf-8") if args.output else sys.stdout
    try:
        for row in rows:
            destination.write(json.dumps(row, sort_keys=True))
            destination.write("\n")
    finally:
        if args.output:
            destination.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
