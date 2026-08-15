"""Regression coverage for paired multivenue treatment analysis."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parent
ANALYZER = TOOLS_DIR / "analyze_paired_multivenue_risk.py"


def world(seed: int, arm: str, value: float) -> dict[str, object]:
    """Create one minimal but complete world-summary row."""
    aggregate = {
        "mean_abs_net_delta": value,
        "mean_max_abs_net_delta": value + 1,
        "mean_abs_gamma": value + 2,
        "mean_abs_vega": value + 3,
        "mean_equity_change": value + 4,
        "mean_terminal_maintenance": value + 5,
        "positive_horizon_position_rows": 7,
        "venue_count": 3,
    }
    return {"seed": seed, "dealer_hedge_mode": arm, "world": aggregate}


class PairedMultivenueRiskTest(unittest.TestCase):
    """Exercise the command boundary with small deterministic paired data."""

    def test_reports_paired_direction_and_bootstrap(self) -> None:
        rows = [
            world(1, "on", 1),
            world(1, "off", 3),
            world(2, "on", 4),
            world(2, "off", 8),
        ]
        with tempfile.TemporaryDirectory() as directory:
            input_path = Path(directory) / "worlds.jsonl"
            output_path = Path(directory) / "result.json"
            input_path.write_text(
                "".join(f"{json.dumps(row)}\n" for row in rows), encoding="utf-8"
            )
            result = subprocess.run(
                [
                    sys.executable,
                    str(ANALYZER),
                    str(input_path),
                    "--output",
                    str(output_path),
                    "--bootstrap-samples",
                    "100",
                ],
                check=True,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.stderr, "")
            report = json.loads(output_path.read_text(encoding="utf-8"))

        delta = report["metrics"][0]
        self.assertEqual(report["seeds"], [1, 2])
        self.assertEqual(delta["n_pairs"], 2)
        self.assertEqual(delta["mean_on_minus_off"], -3)
        self.assertEqual(delta["hedge_on_lower_count"], 2)
        self.assertEqual(delta["hedge_on_higher_count"], 0)
        self.assertEqual(report["data_quality"]["venue_count_per_world"], [3])

    def test_rejects_unpaired_seed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            input_path = Path(directory) / "worlds.jsonl"
            input_path.write_text(json.dumps(world(1, "on", 1)), encoding="utf-8")
            result = subprocess.run(
                [sys.executable, str(ANALYZER), str(input_path)],
                check=False,
                capture_output=True,
                text=True,
            )
        self.assertEqual(result.returncode, 2)
        self.assertIn("incomplete treatment pair", result.stderr)


if __name__ == "__main__":
    unittest.main()
