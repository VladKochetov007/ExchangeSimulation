#!/usr/bin/env python3
"""Render random-walk spot/perp snapshots as a compact GIF or MP4."""

from __future__ import annotations

import argparse
import json
import math
from bisect import bisect_right
from pathlib import Path
from typing import Any

import matplotlib.pyplot as plt
from matplotlib.animation import FFMpegWriter, FuncAnimation, PillowWriter

USD_PRECISION = 100_000
NANOSECONDS_PER_SECOND = 1_000_000_000
ASSETS = ("ABC", "DEF", "GHI")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--logdir", type=Path, required=True, help="randomwalk JSONL directory"
    )
    parser.add_argument(
        "--out", type=Path, required=True, help="output .gif or .mp4 path"
    )
    parser.add_argument(
        "--frames", type=int, default=120, help="maximum rendered frames"
    )
    parser.add_argument(
        "--fps", type=float, default=12, help="output frames per second"
    )
    return parser.parse_args()


def midpoint(snapshot: dict[str, Any]) -> float | None:
    bids = snapshot.get("bids", [])
    asks = snapshot.get("asks", [])
    if not bids or not asks:
        return None
    return (bids[0]["price"] + asks[0]["price"]) / (2 * USD_PRECISION)


def load_series(path: Path) -> tuple[list[int], list[float | None]]:
    records: list[tuple[int, float | None]] = []
    with path.open(encoding="utf-8") as handle:
        for line in handle:
            record = json.loads(line)
            if record.get("event") != "BookSnapshot" or record.get("client_id") != 0:
                continue
            records.append((record["sim_ts"], midpoint(record["data"])))
    if not records:
        raise ValueError(f"no exchange-owned book snapshots in {path}")
    records.sort(key=lambda record: record[0])
    timestamps: list[int] = []
    mids: list[float | None] = []
    for timestamp, mid in records:
        if timestamps and timestamp == timestamps[-1]:
            # A timestamp identifies a simulated instant; the final snapshot is its state.
            mids[-1] = mid
        else:
            timestamps.append(timestamp)
            mids.append(mid)
    return timestamps, mids


def clip_series(
    timestamps: list[int], values: list[float | None], start: int, end: int
) -> tuple[list[float], list[float | None]]:
    """Map snapshots to one common display interval without moving their timestamps."""
    start_index = bisect_right(timestamps, start) - 1
    if start_index < 0:
        raise ValueError("series has no snapshot at or before the common interval")
    end_index = bisect_right(timestamps, end)
    seconds = [0.0]
    clipped_values = [values[start_index]]
    for timestamp, value in zip(
        timestamps[start_index + 1 : end_index], values[start_index + 1 : end_index]
    ):
        seconds.append((timestamp - start) / NANOSECONDS_PER_SECOND)
        clipped_values.append(value)
    return seconds, clipped_values


def playback_duration(frame_count: int, fps: float) -> float:
    """Return the encoded video duration, including the final frame's display time."""
    return frame_count / fps


def main() -> None:
    args = parse_args()
    if args.frames < 2:
        raise ValueError("--frames must be at least 2")
    if not math.isfinite(args.fps) or args.fps <= 0:
        raise ValueError("--fps must be finite and positive")

    series: dict[str, tuple[list[int], list[float | None]]] = {}
    for asset in ASSETS:
        series[f"{asset}-USD"] = load_series(
            args.logdir / "spot" / f"{asset}-USD.jsonl"
        )
        series[f"{asset}-PERP"] = load_series(
            args.logdir / "perp" / f"{asset}-PERP.jsonl"
        )

    start = max(timestamps[0] for timestamps, _ in series.values())
    end = min(timestamps[-1] for timestamps, _ in series.values())
    if end <= start:
        raise ValueError("no common snapshot interval across symbols")
    available = min(args.frames, max(2, int((end - start) / 1_000_000_000) + 1))
    frame_times = [
        start + (end - start) * index // (available - 1) for index in range(available)
    ]
    duration_seconds = (end - start) / NANOSECONDS_PER_SECOND
    display_series = {
        name: clip_series(timestamps, values, start, end)
        for name, (timestamps, values) in series.items()
    }

    figure, axes = plt.subplots(3, 1, figsize=(9, 9), constrained_layout=True)
    lines = []
    for axis, asset in zip(axes, ASSETS, strict=True):
        axis.set_title(f"{asset}: spot vs perpetual")
        axis.set_ylabel("USD")
        axis.grid(alpha=0.25)
        axis.ticklabel_format(axis="y", style="plain", useOffset=False)
        axis.set_xlim(0, duration_seconds)
        finite = [
            value
            for value in display_series[f"{asset}-USD"][1]
            + display_series[f"{asset}-PERP"][1]
            if value is not None
        ]
        if not finite:
            raise ValueError(f"no two-sided quotes for {asset}")
        span = max(max(finite) - min(finite), 0.01)
        axis.set_ylim(min(finite) - span * 0.15, max(finite) + span * 0.15)
        (spot_line,) = axis.plot(
            [], [], color="#166534", drawstyle="steps-post", label="spot"
        )
        (perp_line,) = axis.plot(
            [], [], color="#b45309", drawstyle="steps-post", label="perp"
        )
        axis.legend(loc="upper left")
        lines.append((spot_line, perp_line, asset))
    axes[-1].set_xlabel("simulated seconds")
    title = figure.suptitle("Random-walk venue | simulated t=0.0s")

    def update(frame: int) -> list[Any]:
        current = frame_times[frame]
        current_seconds = (current - start) / NANOSECONDS_PER_SECOND
        artists: list[Any] = [title]
        for axis, (spot_line, perp_line, asset) in zip(axes, lines, strict=True):
            spot_seconds, spots = display_series[f"{asset}-USD"]
            perp_seconds, perps = display_series[f"{asset}-PERP"]
            spot_visible = bisect_right(spot_seconds, current_seconds)
            perp_visible = bisect_right(perp_seconds, current_seconds)
            spot_line.set_data(spot_seconds[:spot_visible], spots[:spot_visible])
            perp_line.set_data(perp_seconds[:perp_visible], perps[:perp_visible])
            artists.extend((spot_line, perp_line))
        title.set_text(f"Random-walk venue | simulated t={current_seconds:.1f}s")
        return artists

    animation = FuncAnimation(
        figure, update, frames=len(frame_times), interval=1000 / args.fps, blit=False
    )
    args.out.parent.mkdir(parents=True, exist_ok=True)
    if args.out.suffix.lower() == ".mp4":
        animation.save(args.out, writer=FFMpegWriter(fps=args.fps))
    else:
        animation.save(args.out, writer=PillowWriter(fps=args.fps))
    playback_seconds = playback_duration(len(frame_times), args.fps)
    print(
        f"wrote {args.out} ({len(frame_times)} frames, {playback_seconds:.1f}s playback, "
        f"{duration_seconds / playback_seconds:.1f}x timelapse)"
    )


if __name__ == "__main__":
    main()
