"""Render the fixed ME-002-B development matrix from its versioned Go surface."""

import argparse
import json
from pathlib import Path

import matplotlib

matplotlib.use("Agg")

import matplotlib.pyplot as plt
import numpy as np


def render_cadence_surface(surface_path: Path, output_path: Path) -> None:
    with surface_path.open(encoding="utf-8") as source:
        surface = json.load(source)
    if surface.get("schema_version") != 2 or surface.get("valid_worlds") != 12:
        raise ValueError("expected the corrected, complete ME-002-B surface v2")
    contrasts = surface["paired_contrasts"]
    if [row["seed"] for row in contrasts] != [13001, 13011, 13017]:
        raise ValueError("unexpected development seed order")

    arm_keys = ("p1_n1", "p80_n1", "p1_n90", "p80_n90")
    arm_labels = ("1 ms poll\n1 ms network", "80 ms poll\n1 ms network",
                  "1 ms poll\n90 ms network", "80 ms poll\n90 ms network")
    fractions = np.array([[row[key] * 100 for key in arm_keys] for row in contrasts])
    poll_effects = np.array([
        [row["poll_effect_n1"] * 100, row["poll_effect_n90"] * 100]
        for row in contrasts
    ])

    fig, axes = plt.subplots(1, 2, figsize=(11.8, 4.5),
                             gridspec_kw={"width_ratios": [1.25, 1]})
    heatmap = axes[0].imshow(fractions, vmin=0, vmax=100, cmap="Blues", aspect="auto")
    axes[0].set_xticks(range(4), arm_labels, fontsize=8)
    axes[0].set_yticks(range(3), [str(row["seed"]) for row in contrasts])
    axes[0].set_ylabel("Development seed")
    axes[0].set_title("Filled target (%)")
    for seed_index in range(3):
        for arm_index in range(4):
            value = fractions[seed_index, arm_index]
            axes[0].text(arm_index, seed_index, f"{value:.2f}",
                         ha="center", va="center",
                         color="white" if value >= 65 else "black", fontsize=9)
    fig.colorbar(heatmap, ax=axes[0], label="Percent of assigned 5 ABC target",
                 fraction=0.046, pad=0.04)

    colors = ("#0072B2", "#D55E00", "#009E73")
    for index, row in enumerate(contrasts):
        axes[1].plot([0, 1], poll_effects[index], marker="o", linewidth=1.7,
                     color=colors[index], label=str(row["seed"]))
    axes[1].axhline(0, color="0.35", linewidth=0.8)
    axes[1].set_xticks([0, 1], ["1 ms network", "90 ms network"])
    axes[1].set_ylabel("80 ms − 1 ms poll effect (percentage points)")
    axes[1].set_title("Within-seed first-action contrast")
    axes[1].legend(title="Development seed", frameon=False)
    axes[1].grid(axis="y", alpha=0.25)

    fig.suptitle("ME-002-B: fixed-phase first action, not recurring trading")
    fig.text(0.5, 0.015,
             "12 development worlds; one 5 ABC BUY child; continuous opportunity lifetime not identified",
             ha="center", fontsize=8)
    fig.tight_layout(rect=(0, 0.04, 1, 0.94))
    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, dpi=180)
    plt.close(fig)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--surface", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    render_cadence_surface(args.surface, args.out)


if __name__ == "__main__":
    main()
