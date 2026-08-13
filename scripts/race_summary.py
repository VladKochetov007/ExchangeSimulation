#!/usr/bin/env python3
"""Summarize latency-race capture across seeds.

Reads loganalyzer metrics from logs/seed_<mode>_<seed>/ and asks whether
profit capture depends on speed. Strict monotonicity across four tiers is too
brittle a test on a handful of seeds, so the headline statistic is the rank
correlation between speed and capture, averaged over seeds, plus the mean
capture per tier.
"""
import json
import subprocess
import sys
from pathlib import Path

TIERS = {"12": 1.0, "13": 0.5, "14": 0.2, "15": 0.05}
BTC, USD = 10**8, 10**5
MARK = 50000  # ABC marked at its nominal level to convert inventory to wealth


def wealth(deltas):
    return deltas.get("USD", 0) / USD + deltas.get("ABC", 0) / BTC * MARK


def load(run_dir):
    metrics = run_dir / "metrics.json"
    if not metrics.exists():
        subprocess.run(["./bin/loganalyzer", "-dir", str(run_dir)],
                       check=True, capture_output=True)
    return json.loads(metrics.read_text())["client_deltas"]


def spearman(xs, ys):
    """Rank correlation; n is tiny and ties are unlikely in the wealth ranks."""
    def rank(vals):
        order = sorted(range(len(vals)), key=lambda i: vals[i])
        r = [0.0] * len(vals)
        for pos, i in enumerate(order):
            r[i] = float(pos)
        return r
    rx, ry = rank(xs), rank(ys)
    n = len(xs)
    d2 = sum((rx[i] - ry[i]) ** 2 for i in range(n))
    return 1 - 6 * d2 / (n * (n * n - 1))


def main():
    seeds = sys.argv[1:] or ["101", "202", "303", "404", "505"]
    for mode in ("polling", "reactive"):
        print(f"\n=== {mode} ===")
        header = " ".join(f"{t:>7}x" for t in TIERS.values())
        print(f"{'seed':>6} {header}   rank-corr")
        per_tier = {cid: [] for cid in TIERS}
        corrs = []
        for seed in seeds:
            run_dir = Path(f"logs/seed_{mode}_{seed}")
            if not run_dir.exists():
                continue
            deltas = load(run_dir)
            rel, base = [], None
            for cid in TIERS:
                w = wealth(deltas.get(cid, {}))
                if base is None:
                    base = w
                rel.append(w / base if base else 0.0)
            for cid, r in zip(TIERS, rel):
                per_tier[cid].append(r)
            # Speed is the inverse of the latency multiplier.
            speeds = [1.0 / t for t in TIERS.values()]
            c = spearman(speeds, rel)
            corrs.append(c)
            print(f"{seed:>6} " + " ".join(f"{r:8.2f}" for r in rel) + f"   {c:+.2f}")
        if corrs:
            means = [sum(per_tier[cid]) / len(per_tier[cid]) for cid in TIERS]
            print(f"{'mean':>6} " + " ".join(f"{m:8.2f}" for m in means) +
                  f"   {sum(corrs) / len(corrs):+.2f}  (n={len(corrs)} seeds)")


if __name__ == "__main__":
    main()
