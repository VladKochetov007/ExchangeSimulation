#!/usr/bin/env bash
# Materialize the immutable SV1 24-hour development namespace from the
# accepted R2 configs. This never edits the historical R2 config directory.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-r2-sv1-24h"
source_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
activation_config="$root_dir/research/configs/v2-r2-sv1/activation-607.json"
normalizer=${V2_R2_SV1_CONFIG_NORMALIZER_BIN:-"$root_dir/bin/multivenue"}

[[ -d "$config_dir" ]] || mkdir -p -- "$config_dir"
[[ -s "$activation_config" ]] || { echo "missing accepted activation roster: $activation_config" >&2; exit 1; }
[[ -x "$normalizer" ]] || { echo "missing config normalizer: $normalizer" >&2; exit 1; }
cdf_roster=$(jq -c -e '.elastic_liquidity_suppliers | type == "array" and length == 4' "$activation_config" >/dev/null && jq -c '.elastic_liquidity_suppliers' "$activation_config")

write_config() {
	local output=$1 source=$2 mode=$3 experiment=$4 hypothesis=$5
	[[ ! -e "$output" && ! -L "$output" ]] || {
		echo "refusing to overwrite registered config: $output" >&2
		exit 1
	}
	local temporary normalized_dir normalized
	temporary=$(mktemp "$output.tmp-XXXXXX")
	if [[ "$mode" == treatment ]]; then
		jq --arg experiment "$experiment" --arg hypothesis "$hypothesis" --arg description "V2-R2-SV1-24H finite CDF/USD liquidity treatment; development-only successor" --argjson roster "$cdf_roster" \
			'.experiment_id = $experiment |
			 .hypothesis_id = $hypothesis |
			 .description = $description |
			 .date = "2026-09-01" |
			 .status = "registered-development" |
			 .elastic_liquidity_suppliers = $roster |
			 .record_elastic_liquidity_supplier_decisions = true |
			 .market_data_receipt_roles = ((.market_data_receipt_roles + ["cdf_elastic_supplier"]) | unique)' "$source" >"$temporary"
	else
		jq --arg experiment "$experiment" --arg hypothesis "$hypothesis" --arg description "V2-R2-SV1-24H matched no-CDF control; development-only successor" \
			'.experiment_id = $experiment |
			 .hypothesis_id = $hypothesis |
			 .description = $description |
			 .date = "2026-09-01" |
			 .status = "registered-development"' "$source" >"$temporary"
	fi
	normalized_dir=$(mktemp -d)
	normalized="$normalized_dir/run-config.json"
"$normalizer" -config "$temporary" -logdir "$normalized_dir" -write-effective-config "$normalized" >/dev/null 2>&1
	[[ -s "$normalized" ]] || { echo "config normalizer produced no effective config: $output" >&2; exit 1; }
	mv -- "$normalized" "$output"
	rmdir -- "$normalized_dir" 2>/dev/null || true
	rm -- "$temporary"
}

candidate_hypothesis="V2-R2-SV1-CDF-LIQUIDITY-24H"
control_hypothesis="V2-R2-SV1-CDF-LIQUIDITY-24H-CONTROL"
for seed in 607 613 617; do
	write_config "$config_dir/treatment-$seed.json" "$source_dir/dev-$seed.json" treatment \
		"v2-r2-sv1-24h-treatment-$seed" "$candidate_hypothesis"
	write_config "$config_dir/control-$seed.json" "$source_dir/dev-$seed.json" control \
		"v2-r2-sv1-24h-control-$seed" "$control_hypothesis"
done

# CDF decisions require persisted evidence by contract, so the no-log parity
# run is deliberately the matched no-CDF control. It tests the unchanged R2
# trajectory's logging neutrality without pretending to test CDF activation.
write_config "$config_dir/control-607-none.json" "$source_dir/dev-607-none.json" control \
	"v2-r2-sv1-24h-control-607-none" "$control_hypothesis"

echo "rendered SV1 24-hour configs in $config_dir"
sha256sum "$config_dir"/*.json
