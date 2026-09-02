#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
filter="$root_dir/scripts/v2-r2-sv1-paired-survival.jq"
temp_root=$(mktemp -d "${TMPDIR:-/tmp}/sv1-paired-survival.XXXXXX")
trap 'rm -rf -- "$temp_root"' EXIT

fail() {
	printf 'SV1 paired survival contract failure: %s\n' "$*" >&2
	exit 1
}

write_summary() {
	local path=$1 empty=$2
	jq -n --argjson empty "$empty" '
		{schema_version: 1, predicates: {aggregate_two_sided_98pct: ($empty == 0)},
		 venues: [{snapshots: 100, empty_side_snapshots: $empty}],
		 window_metrics: [{empty_side_share: ($empty / 100)}]}' >"$path"
}

write_summary "$temp_root/treatment.json" 0
write_summary "$temp_root/control.json" 4
jq -n -e --slurpfile treatment "$temp_root/treatment.json" --slurpfile control "$temp_root/control.json" \
	--argjson seed 643 -f "$filter" >"$temp_root/effect.json"
jq -e '
	.predicates.matched_measurements_valid and
	.predicates.treatment_not_worse and
	.predicates.strict_aggregate_reduction and
	.effect.aggregate_empty_side_share_reduction == 0.04' "$temp_root/effect.json" >/dev/null ||
	fail "strict treatment improvement was not identified"

write_summary "$temp_root/treatment-worse.json" 5
jq -n -e --slurpfile treatment "$temp_root/treatment-worse.json" --slurpfile control "$temp_root/control.json" \
	--argjson seed 647 -f "$filter" >"$temp_root/worse-effect.json"
jq -e '
	.predicates.matched_measurements_valid and
	(.predicates.treatment_not_worse | not) and
	(.predicates.strict_aggregate_reduction | not)' "$temp_root/worse-effect.json" >/dev/null ||
	fail "worse treatment was accepted as non-worse"

printf '✓ SV1 paired treatment/control survival estimand\n'
