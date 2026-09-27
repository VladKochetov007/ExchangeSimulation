"""Render the locked ME-015 development occupancy surface; no raw-log analysis."""

import argparse
import hashlib
import json
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


COMPOSITIONS = ("P", "A", "M1", "M2")
SEEDS = (18101, 18111, 18117)


def render(surface_path: Path, output_path: Path) -> str:
    if output_path.exists():
        raise FileExistsError(f"refusing to overwrite {output_path}")
    raw = surface_path.read_bytes()
    surface = json.loads(raw)
    if (
        surface.get("study_id") != "ME-015"
        or surface.get("valid_economic_cells") != 24
        or len(surface.get("cells", [])) != 24
        or surface.get("primary_contrast_status") != "ESTIMATED_DEVELOPMENT_ONLY"
    ):
        raise ValueError("not a complete validated ME-015 surface")
    by_assignment = {}
    for record in surface["cells"]:
        cell = record["cell"]
        key = (cell["reference_mode"], cell["composition"], cell["seed"])
        if key in by_assignment:
            raise ValueError(f"duplicate cell: {key}")
        measured = record["measurement_book_durations"]
        if measured["horizon_ns"] != 2_700_000_000_000:
            raise ValueError(f"unexpected window: {key}")
        by_assignment[key] = 100 * measured["two_sided_ns"] / measured["horizon_ns"]
    if set(by_assignment) != {
        (mode, composition, seed)
        for mode in ("ON", "OFF")
        for composition in COMPOSITIONS
        for seed in SEEDS
    }:
        raise ValueError("incomplete ME-015 assignment set")

    fig, axes = plt.subplots(2, 2, figsize=(8.5, 5.6), sharex=True, sharey=True)
    for ax, composition in zip(axes.flat, COMPOSITIONS):
        for mode, color, marker in (("ON", "#176b8c", "o"), ("OFF", "#b0553c", "x")):
            values = [by_assignment[(mode, composition, seed)] for seed in SEEDS]
            ax.scatter(SEEDS, values, marker=marker, color=color, s=52,
                       label=f"Reference {mode}", zorder=3)
        ax.set_title({"P": "Four pure makers (primary)", "A": "Four AS-style makers",
                      "M1": "Mixed slots P–A–P–A", "M2": "Mixed slots A–P–A–P"}[composition])
        ax.set_xticks(SEEDS)
        ax.set_ylim(-3, 50)
        ax.grid(axis="y", alpha=0.25)
        ax.spines[["top", "right"]].set_visible(False)
    for ax in axes[:, 0]:
        ax.set_ylabel("Public two-sided time (%)")
    for ax in axes[-1, :]:
        ax.set_xlabel("Development seed")
    handles, labels = axes.flat[0].get_legend_handles_labels()
    fig.legend(handles, labels, loc="upper center", ncol=2, bbox_to_anchor=(0.5, 1.02))
    fig.suptitle("ME-015: fixed 45-minute window, three development seeds", y=1.08)
    fig.text(0.5, -0.015,
             "OFF and ON are whole-world assignments. Marker points are worlds, not independent events."
             " All terminal marks unavailable.",
             ha="center", fontsize=8)
    fig.tight_layout()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, dpi=180, bbox_inches="tight")
    plt.close(fig)
    return hashlib.sha256(raw).hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--surface", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    print(f"surface_sha256={render(args.surface, args.out)}")


if __name__ == "__main__":
    main()
