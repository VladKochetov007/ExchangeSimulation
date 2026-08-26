#!/usr/bin/env bash
# Materialise the preregistered untouched P6-R1 holdout configs only after the
# paired development viability screen has passed. Stage values are copied
# from the hash-pinned development configs; only seed and provenance text vary.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-6-p6r1"
holdout_seeds=(223 227 229)

"$root_dir/scripts/check-v2-6-p6r1-configs.sh" >/dev/null

for stage in O0 O1 O2 O3 O4; do
	for seed in "${holdout_seeds[@]}"; do
		source="$config_dir/$stage-211.json"
		target="$config_dir/$stage-$seed.json"
		if [[ ! -s "$source" ]]; then
			echo "missing development source config: $source" >&2
			exit 1
		fi
		if [[ -e "$target" ]]; then
			echo "refusing to overwrite holdout config: $target" >&2
			exit 1
		fi
		jq --arg stage "$stage" --argjson seed "$seed" '
			.seed = $seed
			| .experiment_id = ("v2-6-p6r1-cross-asset-mark-" + $stage + "-seed-" + ($seed|tostring))
			| .description = ("V2-6 P6-R1 explicit cross-asset collateral-mark viability " + $stage + "; untouched holdout; no other stage or ecology delta.")
		' "$source" >"$target"
	done
done

echo "generated V2-6 P6-R1 holdout configs in $config_dir"
