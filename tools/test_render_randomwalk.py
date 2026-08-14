"""Regression tests for the random-walk renderer's evidence-preserving transforms."""

from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory


def load_renderer() -> object:
    path = Path(__file__).with_name("render_randomwalk.py")
    spec = importlib.util.spec_from_file_location("render_randomwalk", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"could not load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


RENDERER = load_renderer()
SECOND = 1_000_000_000


class ClipSeriesTest(unittest.TestCase):
    def test_clip_series_preserves_snapshot_times_and_one_sided_gaps(self) -> None:
        seconds, values = RENDERER.clip_series(
            [10 * SECOND, 12 * SECOND, 20 * SECOND],
            [101.0, None, 103.0],
            11 * SECOND,
            25 * SECOND,
        )

        self.assertEqual(seconds, [0.0, 1.0, 9.0])
        self.assertEqual(values, [101.0, None, 103.0])

    def test_clip_series_does_not_extend_the_final_snapshot_to_interval_end(
        self,
    ) -> None:
        seconds, values = RENDERER.clip_series(
            [10 * SECOND, 20 * SECOND], [101.0, 102.0], 10 * SECOND, 40 * SECOND
        )

        self.assertEqual(seconds, [0.0, 10.0])
        self.assertEqual(values, [101.0, 102.0])


class LoadSeriesTest(unittest.TestCase):
    def test_load_series_sorts_and_keeps_final_same_timestamp_state(self) -> None:
        records = [
            {
                "event": "BookSnapshot",
                "client_id": 0,
                "sim_ts": 20,
                "data": {"bids": [{"price": 200}], "asks": [{"price": 400}]},
            },
            {
                "event": "BookSnapshot",
                "client_id": 0,
                "sim_ts": 10,
                "data": {"bids": [{"price": 100}], "asks": [{"price": 300}]},
            },
            {
                "event": "BookSnapshot",
                "client_id": 0,
                "sim_ts": 20,
                "data": {"bids": [], "asks": []},
            },
        ]
        with TemporaryDirectory() as directory:
            path = Path(directory) / "book.jsonl"
            path.write_text(
                "\n".join(json.dumps(record) for record in records), encoding="utf-8"
            )
            timestamps, mids = RENDERER.load_series(path)

        self.assertEqual(timestamps, [10, 20])
        self.assertEqual(mids, [0.002, None])


class PlaybackDurationTest(unittest.TestCase):
    def test_playback_duration_includes_each_encoded_frame(self) -> None:
        self.assertEqual(RENDERER.playback_duration(24, 6.0), 4.0)


if __name__ == "__main__":
    unittest.main()
