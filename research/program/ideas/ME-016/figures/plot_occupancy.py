"""Plot the registered ME-016 occupancy contrast from the Go machine surface."""

import argparse
import json
from pathlib import Path

import matplotlib.pyplot as plt


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--surface", type=Path, default=Path(__file__).resolve().parents[1] / "surface.json")
    parser.add_argument("--out", type=Path, default=Path(__file__).resolve().parent / "occupancy.png")
    args = parser.parse_args()

    surface = json.loads(args.surface.read_text(encoding="utf-8"))
    if surface["study_id"] != "ME-016" or surface["valid_economic_cells"] != 12:
        raise ValueError("expected the complete ME-016 Go surface")

    panels = [surface["primary_m1_pairs"], surface["sensitivity_m2_pairs"]]
    seeds = [str(pair["seed"]) for pair in panels[0]]
    if [str(pair["seed"]) for pair in panels[1]] != seeds:
        raise ValueError("slot assignments use different seed labels")

    figure, axes = plt.subplots(1, 2, figsize=(10.6, 4.1), layout="constrained")
    colors = ["#176f86", "#b25731"]
    for panel, pairs, color, label in zip(axes, panels, colors, ["M1 primary", "M2 slot sensitivity"]):
        denominator = surface["window_denominator_ns"]
        changes = [100 * pair["gain_two_minus_zero_ns"] / denominator for pair in pairs]
        panel.axhline(0, color="#555555", linewidth=0.9)
        panel.scatter(seeds, changes, color=color, s=48, zorder=3)
        panel.set_title(label)
        panel.set_xlabel("Development seed")
        panel.set_ylabel("Gain 2 − gain 0 (percentage points)")
        panel.set_ylim(-0.36, 0.26)
        panel.grid(axis="y", alpha=0.22)
        for x, value, pair in zip(seeds, changes, pairs):
            panel.annotate(f"{pair['gain_two_minus_zero_ns'] / 1e9:+.0f} s", (x, value),
                           xytext=(0, 8 if value >= 0 else -15), textcoords="offset points",
                           ha="center", fontsize=8)

    figure.suptitle("ME-016: public two-sided-book time, gain assignment from world start", fontsize=11)
    figure.savefig(args.out, dpi=180)
    plt.close(figure)


if __name__ == "__main__":
    main()
