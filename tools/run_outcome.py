"""Classify how a run ended, so a killed arm is never mistaken for a failed one.

A collapsed market and a process someone terminated both leave a log directory
with no report in it. They mean opposite things: one is a result, the other is
an accident. This reads the run's stderr alongside its artifacts and says which.

Usage:
    python tools/run_outcome.py logs/<run> [logs/<run> ...] [--stderr-dir .cache]
"""

from __future__ import annotations

import argparse
import json
import subprocess
from pathlib import Path

# A market that ends with no valid two-sided mark has collapsed. Anything else
# missing a report either never finished or died for an unrelated reason.
COLLAPSE_MARKERS = ("missing valid", "no valid risk mark", "requires two-sided")


def classify(run: Path, stderr_dir: Path) -> tuple[str, str]:
    report = run / "greeks.json"
    stderr_path = stderr_dir / f"{run.name}.out"
    stderr = stderr_path.read_text() if stderr_path.exists() else ""

    if report.exists():
        try:
            json.loads(report.read_text())
        except json.JSONDecodeError:
            return "TRUNCATED", "report present but unparseable; the run was interrupted while writing"
        return "COMPLETED", ""

    for marker in COLLAPSE_MARKERS:
        if marker in stderr:
            line = next(l for l in stderr.splitlines() if marker in l)
            return "COLLAPSED", line.strip()
    if not run.exists():
        return "NEVER STARTED", "no log directory"
    if stderr.strip():
        return "FAILED", stderr.strip().splitlines()[-1]
    return "INCOMPLETE", "no report and no error: still running, or terminated by something outside the simulator"


def build_warning(run: Path, head: str | None) -> str:
    """Flag a run produced by a binary that is not the current source.

    Three experiments in this campaign were run against a binary compiled before
    the fix under test. Each looked like a real null result until something else
    exposed it, so the check belongs in the tooling rather than in vigilance.
    """
    manifest_path = run / "manifest.json"
    if not manifest_path.exists():
        return ""
    try:
        build = json.loads(manifest_path.read_text()).get("build") or {}
    except json.JSONDecodeError:
        return ""
    revision = build.get("revision", "")
    if not revision or revision == "unknown":
        return "  [build unknown — cannot verify the source it ran]"
    # Staleness is checked first: a binary built from an older commit is the
    # failure that has actually cost runs, and a modified tree must not hide it.
    if head and revision != head:
        # Only a change to compiled source can make a binary behave differently.
        # Flagging documentation commits as stale trains the reader to ignore
        # the warning, which is how the original failure survived a contract
        # entry telling me to watch for it.
        changed = subprocess.run(
            ["git", "diff", "--name-only", revision, head, "--", "*.go"],
            capture_output=True, text=True,
        )
        if changed.returncode != 0:
            return f"  [STALE: built at {revision[:8]}, current source is {head[:8]}]"
        if changed.stdout.strip():
            count = len(changed.stdout.strip().splitlines())
            return f"  [STALE: built at {revision[:8]}, {count} Go file(s) changed since]"
        return ""
    if build.get("modified"):
        return f"  [built at {revision[:8]} from a modified tree]"
    return ""


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runs", nargs="+", type=Path)
    parser.add_argument("--stderr-dir", type=Path, default=Path(".cache"))
    args = parser.parse_args()
    head = None
    try:
        head = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True,
                              text=True, check=True).stdout.strip()
    except Exception:
        pass

    for run in args.runs:
        outcome, detail = classify(run, args.stderr_dir)
        print(f"{run.name:26s} {outcome:14s} {detail}{build_warning(run, head)}")


if __name__ == "__main__":
    main()
