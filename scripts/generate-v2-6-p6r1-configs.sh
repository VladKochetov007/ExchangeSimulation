#!/usr/bin/env bash
# Materialise the immutable V2-6 P6-R1 config family from the completed P6
# stage files.  The only simulation field added is the explicit cross-asset
# collateral-mark contract; all other stage deltas remain inherited.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_dir="$root_dir/research/configs/v2-6-p6"
target_dir="$root_dir/research/configs/v2-6-p6r1"
mkdir -p "$target_dir"

for stage in O0 O1 O2 O3 O4; do
	for seed in 211 213; do
		source="$source_dir/$stage-$seed.json"
		target="$target_dir/$stage-$seed.json"
		if [[ ! -s "$source" ]]; then
			echo "missing source P6 config: $source" >&2
			exit 1
		fi
		if [[ -e "$target" ]]; then
			echo "refusing to overwrite P6-R1 config: $target" >&2
			exit 1
		fi
		jq --arg stage "$stage" --argjson seed "$seed" '
			.cross_asset_collateral_marks = true
			| .hypothesis_id = "V2-6-P6R1-CROSS-ASSET-MARK"
			| .experiment_id = ("v2-6-p6r1-cross-asset-mark-" + $stage + "-seed-" + ($seed|tostring))
			| .description = ("V2-6 P6-R1 explicit cross-asset collateral-mark viability " + $stage + "; no other stage or ecology delta.")
		' "$source" >"$target"
		done
done

echo "generated V2-6 P6-R1 configs in $target_dir"
