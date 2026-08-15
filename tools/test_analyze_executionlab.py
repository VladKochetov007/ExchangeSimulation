"""Regression coverage for paired execution-lab result analysis."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

TOOLS_DIR = Path(__file__).resolve().parent
ANALYZER = TOOLS_DIR / "analyze_executionlab.py"


def report(side: str, decision_at: int, shortfall: float) -> dict[str, object]:
    """Create a complete synthetic parent report."""
    return {
        "Side": side,
        "DecisionAt": decision_at,
        "TargetQty": 10,
        "FilledQty": 10,
        "UnfilledQty": 0,
        "ShortfallBps": shortfall,
        "TargetShortfallBps": shortfall,
        "TargetShortfallValid": True,
    }


def world(seed: int, policy: str, values: list[float]) -> dict[str, object]:
    """Create a paired-world row with an alternating parent schedule."""
    reports = [
        report("BUY" if index % 2 == 0 else "SELL", index + 1, value)
        for index, value in enumerate(values)
    ]
    return {"seed": seed, "policy": policy, "report": reports[0], "reports": reports}


class ExecutionLabAnalysisTest(unittest.TestCase):
    """Exercise the command boundary on small valid and invalid studies."""

    def run_analyzer(
        self, rows: list[dict[str, object]]
    ) -> subprocess.CompletedProcess[str]:
        """Run the analyzer over temporary JSONL and return its process result."""
        with tempfile.TemporaryDirectory() as directory:
            input_path = Path(directory) / "worlds.jsonl"
            input_path.write_text(
                "".join(f"{json.dumps(row)}\n" for row in rows), encoding="utf-8"
            )
            return subprocess.run(
                [
                    sys.executable,
                    str(ANALYZER),
                    str(input_path),
                    "--bootstrap-samples",
                    "100",
                ],
                check=False,
                capture_output=True,
                text=True,
            )

    def test_reports_paired_parent_costs(self) -> None:
        result = self.run_analyzer(
            [
                world(1, "immediate", [10, 14]),
                world(1, "twap", [7, 9]),
                world(2, "immediate", [12, 16]),
                world(2, "twap", [8, 10]),
            ]
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        summary = json.loads(result.stdout)
        self.assertEqual(summary["seeds"], [1, 2])
        self.assertEqual(summary["mean_immediate_target_shortfall_bps"], 13)
        self.assertEqual(summary["mean_twap_target_shortfall_bps"], 8.5)
        self.assertEqual(summary["mean_twap_minus_immediate_bps"], -4.5)
        self.assertEqual(summary["twap_lower_count"], 2)
        self.assertEqual(summary["data_quality"]["parent_count_per_world"], [2])

    def test_rejects_policy_schedule_mismatch(self) -> None:
        immediate = world(1, "immediate", [10, 14])
        twap = world(1, "twap", [7, 9])
        twap["reports"] = [report("BUY", 1, 7), report("SELL", 3, 9)]
        result = self.run_analyzer([immediate, twap])
        self.assertEqual(result.returncode, 2)
        self.assertIn("parent schedules differ", result.stderr)


if __name__ == "__main__":
    unittest.main()
