#!/usr/bin/env python3
"""Render random-walk spot/perp snapshots as a compact GIF or MP4."""

from __future__ import annotations

import argparse
import json
from bisect import bisect_right
from pathlib import Path
from typing import Any

import matplotlib.pyplot as plt
from matplotlib.animation import FuncAnimation, FFMpegWriter, PillowWriter


USD_PRECISION = 100_000
ASSETS = ("ABC", "DEF", "GHI")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--logdir", type=Path, required=True, help="randomwalk JSONL directory")
    parser.add_argument("--out", type=Path, required=True, help="output .gif or .mp4 path")
    parser.add_argument("--frames", type=int, default=120, help="maximum rendered frames")
    parser.add_argument("--fps", type=int, default=12, help="output frames per second")
    return parser.parse_args()


def midpoint(snapshot: dict[str, Any]) -> float | None:
    bids = snapshot.get("bids", [])
    asks = snapshot.get("asks", [])
    if not bids or not asks:
        return None
    return (bids[0]["price"] + asks[0]["price"]) / (2 * USD_PRECISION)


def load_series(path: Path) -> tuple[list[int], list[float]]:
    timestamps: list[int] = []
    mids: list[float] = []
    with path.open(encoding="utf-8") as handle:
        for line in handle:
            record = json.loads(line)
            if record.get("event") != "BookSnapshot" or record.get("client_id") != 0:
                continue
            mid = midpoint(record["data"])
            if mid is not None:
                timestamps.append(record["sim_ts"])
                mids.append(mid)
    if not timestamps:
        raise ValueError(f"no exchange-owned book snapshots in {path}")
    return timestamps, mids


def sample_at(timestamps: list[int], values: list[float], timestamp: int) -> float | None:
    index = bisect_right(timestamps, timestamp) - 1
    return values[index] if index >= 0 else None


def main() -> None:
    args = parse_args()
    series: dict[str, tuple[list[int], list[float]]] = {}
    for asset in ASSETS:
        series[f"{asset}-USD"] = load_series(args.logdir / "spot" / f"{asset}-USD.jsonl")
        series[f"{asset}-PERP"] = load_series(args.logdir / "perp" / f"{asset}-PERP.jsonl")

    start = max(timestamps[0] for timestamps, _ in series.values())
    end = min(timestamps[-1] for timestamps, _ in series.values())
    if end <= start:
        raise ValueError("no common snapshot interval across symbols")
    available = min(args.frames, max(2, int((end - start) / 1_000_000_000) + 1))
    frame_times = [start + (end - start) * index // (available - 1) for index in range(available)]

    figure, axes = plt.subplots(3, 1, figsize=(9, 9), constrained_layout=True)
    lines = []
    for axis, asset in zip(axes, ASSETS, strict=True):
        axis.set_title(f"{asset}: spot vs perpetual")
        axis.set_ylabel("USD")
        axis.grid(alpha=0.25)
        spot_line, = axis.plot([], [], color="#166534", label="spot")
        perp_line, = axis.plot([], [], color="#b45309", label="perp")
        axis.legend(loc="upper left")
        lines.append((spot_line, perp_line, asset))
    axes[-1].set_xlabel("simulated seconds")
    title = figure.suptitle("")

    def update(frame: int) -> list[Any]:
        current = frame_times[frame]
        seconds = [(timestamp - start) / 1_000_000_000 for timestamp in frame_times[: frame + 1]]
        artists: list[Any] = [title]
        for axis, (spot_line, perp_line, asset) in zip(axes, lines, strict=True):
            spot_ts, spot_values = series[f"{asset}-USD"]
            perp_ts, perp_values = series[f"{asset}-PERP"]
            spots = [sample_at(spot_ts, spot_values, timestamp) for timestamp in frame_times[: frame + 1]]
            perps = [sample_at(perp_ts, perp_values, timestamp) for timestamp in frame_times[: frame + 1]]
            spot_line.set_data(seconds, spots)
            perp_line.set_data(seconds, perps)
            finite = [value for value in spots + perps if value is not None]
            axis.set_xlim(0, max(seconds[-1], 1.0))
            if finite:
                span = max(max(finite) - min(finite), 0.01)
                axis.set_ylim(min(finite) - span * 0.15, max(finite) + span * 0.15)
            artists.extend((spot_line, perp_line))
        title.set_text(f"Random-walk venue | simulated t={((current - start) / 1e9):.1f}s")
        return artists

    animation = FuncAnimation(figure, update, frames=len(frame_times), interval=1000 / args.fps, blit=False)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    if args.out.suffix.lower() == ".mp4":
        animation.save(args.out, writer=FFMpegWriter(fps=args.fps))
    else:
        animation.save(args.out, writer=PillowWriter(fps=args.fps))
    print(f"wrote {args.out} ({len(frame_times)} frames)")


if __name__ == "__main__":
    main()
