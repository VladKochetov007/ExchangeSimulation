#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
summary_filter="$root_dir/scripts/v2-r2-sv1-survival-summary.jq"
temp_root=$(mktemp -d)
trap 'rm -rf -- "$temp_root"' EXIT

start_nano=1735693200000000000
window_nano=3600000000000
expected_windows=23

jq -n --argjson start "$start_nano" --argjson window "$window_nano" '
	["central", "north", "south"] as $venues |
	[range(0; 23)] as $indices |
	{result: {
		windows: [$venues[] as $venue | $indices[] as $index | {
			symbol: "CDF/USD", venue_id: $venue,
			start: ($start + $index * $window), end: ($start + ($index + 1) * $window),
			snapshots: 1, empty_side_snapshots: 0
		}],
		book_summaries: [$venues[] | {
			symbol: "CDF/USD", venue_id: ., windows: 23, viable: true,
			snapshots: 23, empty_side_snapshots: 0
		}]
	}}' >"$temp_root/valid.json"

render_summary() {
	local input=$1 output=$2
	jq -e --arg cell "fixture" --argjson seed 607 \
		--arg contract "v2-r2-sv1-24h-survival-side-availability-v2" \
		--argjson start_nano "$start_nano" --argjson window_nano "$window_nano" \
		--argjson expected_windows "$expected_windows" --argjson max_empty 0.02 \
		-f "$summary_filter" "$input" >"$output"
}

render_summary "$temp_root/valid.json" "$temp_root/valid-summary.json"
jq -e '
	(.predicates | all(to_entries[]; .value == true)) and
	.predicates.exact_post_warmup_window_coverage == true' "$temp_root/valid-summary.json" >/dev/null

jq '.result.windows |= .[:-1]' "$temp_root/valid.json" >"$temp_root/missing-window.json"
render_summary "$temp_root/missing-window.json" "$temp_root/missing-window-summary.json"
jq -e '.predicates.exact_post_warmup_window_coverage == false' \
	"$temp_root/missing-window-summary.json" >/dev/null
if jq -e '.predicates | all(to_entries[]; .value == true)' \
	"$temp_root/missing-window-summary.json" >/dev/null; then
	echo "missing survival window was accepted" >&2
	exit 1
fi

# jq uses IEEE-754 numbers; a one-nanosecond mutation is not representable at
# epoch-scale timestamps, while the registered viability grid is second-based.
jq '.result.windows[0].start += 1000000000' "$temp_root/valid.json" >"$temp_root/shifted-window.json"
render_summary "$temp_root/shifted-window.json" "$temp_root/shifted-window-summary.json"
jq -e '.predicates.exact_post_warmup_window_coverage == false' \
	"$temp_root/shifted-window-summary.json" >/dev/null

jq '.result.windows += [{symbol: "CDF/USD", venue_id: "west", start: 1735693200000000000, end: 1735696800000000000, snapshots: 1, empty_side_snapshots: 0}]' \
	"$temp_root/valid.json" >"$temp_root/extra-venue.json"
render_summary "$temp_root/extra-venue.json" "$temp_root/extra-venue-summary.json"
jq -e '.predicates.exact_post_warmup_window_coverage == false and .predicates.cdf_books_present == true' \
	"$temp_root/extra-venue-summary.json" >/dev/null

echo "V2-R2-SV1 survival summary contract: pass"
