#!/usr/bin/env bash
# Run the separately registered, development-only V2-R2-SV1 activation probe.
# This is intentionally not a registered integrated-long-run cell and cannot
# select a holdout seed. It creates a fresh paired treatment/control namespace
# and leaves every artifact in place for independent review.
set -euo pipefail

if [[ $# -gt 2 ]]; then
	echo "usage: $0 [multivenue-binary] [cdf-liquidity-audit-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

go_bin_dir=/usr/local/go/bin
[[ -x "$go_bin_dir/go" ]] || go_bin_dir=$(dirname -- "$(command -v go)")
PATH="$go_bin_dir:$PATH"
export PATH

[[ -z "${EXSIM_BINARY_EVIDENCE:-}" ]] || {
	echo "activation probe refuses prototype EXSIM_BINARY_EVIDENCE overrides" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "activation probe requires a clean scientific worktree" >&2
	exit 1
}

head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || {
	echo "invalid scientific HEAD: $head_revision" >&2
	exit 1
}

config_dir="$root_dir/research/configs/v2-r2-sv1"
treatment_config="$config_dir/activation-607.json"
control_config="$config_dir/activation-607-control.json"
binary=${1:-"$root_dir/bin/multivenue"}
audit_binary=${2:-"$root_dir/bin/cdf-liquidity-audit"}
[[ -x "$binary" && -x "$audit_binary" && -s "$treatment_config" && -s "$control_config" ]] || {
	echo "missing activation configs or executable: $treatment_config $control_config $binary $audit_binary" >&2
	exit 1
}

[[ "$(jq -er '.seed' "$treatment_config")" == 607 && "$(jq -er '.seed' "$control_config")" == 607 ]] || {
	echo "activation probe is restricted to development seed 607" >&2
	exit 1
}
[[ "$(jq -er '.evidence_format' "$treatment_config")" == evstream_v3 && "$(jq -er '.evidence_format' "$control_config")" == evstream_v3 ]] || {
	echo "activation probe requires evstream_v3 in both arms" >&2
	exit 1
}
[[ "$(jq -er '.record_market_data_receipts' "$treatment_config")" == true && "$(jq -er '.record_market_data_receipts' "$control_config")" == true ]] || {
	echo "activation probe requires receipt evidence in both arms" >&2
	exit 1
}
jq -e '(.elastic_liquidity_suppliers | type == "array" and length > 0) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") != null)' "$treatment_config" >/dev/null || {
	echo "treatment does not declare the CDF roster and receipt role" >&2
	exit 1
}
jq -e '(.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") == null)' "$control_config" >/dev/null || {
	echo "control is not a no-CDF paired population" >&2
	exit 1
}

binary_revision=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
binary_go_version=$(v2_r2_binary_go_version "$binary")
audit_revision=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
audit_modified=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
audit_trimpath=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
audit_cgo_enabled=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
audit_go_version=$(v2_r2_binary_go_version "$audit_binary")
[[ "$binary_revision" == "$head_revision" && "$binary_modified" == false && "$binary_trimpath" == true && "$binary_cgo_enabled" == 0 ]] || {
	echo "multivenue binary is not a clean reproducible build of HEAD" >&2
	exit 1
}
[[ "$audit_revision" == "$head_revision" && "$audit_modified" == false && "$audit_trimpath" == true && "$audit_cgo_enabled" == 0 ]] || {
	echo "CDF analyzer is not a clean reproducible build of HEAD" >&2
	exit 1
}
v2_r2_is_go_127 "$binary_go_version" || { echo "multivenue binary is not Go 1.27: $binary_go_version" >&2; exit 1; }
v2_r2_is_go_127 "$audit_go_version" || { echo "CDF analyzer is not Go 1.27: $audit_go_version" >&2; exit 1; }

v2_r2_acquire_namespace_lock || {
	echo "could not acquire the R2 evidence namespace lock" >&2
	exit 1
}

horizon=5m
simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
output_root=${V2_R2_SV1_ACTIVATION_ROOT:-"/home/vlad/external-scratch/v2-r2-sv1-activation-$head_revision"}
[[ "$output_root" == /* && "$output_root" != */ && "$output_root" != *$'\n'* && "$output_root" != *$'\t'* ]] || {
	echo "activation output root must be an absolute, non-empty path" >&2
	exit 1
}
[[ ! -e "$output_root" && ! -L "$output_root" ]] || {
	echo "refusing to overwrite activation output root: $output_root" >&2
	exit 1
}
mkdir -p -- "$(dirname -- "$output_root")"
mkdir -- "$output_root"
treatment_dir="$output_root/treatment"
control_dir="$output_root/control"

prepare_arm() {
	local arm=$1 config=$2
	mkdir -- "$arm"
	# The simulator's normalized manifest is the effective scientific config:
	# only the CLI-defaulted full log mode is added to these explicit inputs.
	jq '.log_mode = "full"' "$config" >"$arm/run-config.json"
	local config_sha binary_sha experiment hypothesis
	config_sha=$(sha256sum -- "$arm/run-config.json" | awk '{print $1}')
	binary_sha=$(sha256sum -- "$binary" | awk '{print $1}')
	experiment=$(jq -er '.experiment_id' "$config")
	hypothesis=$(jq -er '.hypothesis_id' "$config")
	jq -n \
		--arg cell "v2-r2-sv1-activation-607-$(basename "$arm")" \
		--argjson seed 607 \
		--arg horizon "$horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" \
		--argjson simulation_end_nano "$simulation_end_nano" \
		--arg config_sha256 "$config_sha" \
		--arg binary_sha256 "$binary_sha" \
		--arg git_revision "$head_revision" \
		--arg experiment "$experiment" \
		--arg hypothesis "$hypothesis" \
		--arg evidence_format evstream_v3 \
		--arg log_mode full \
		--arg binary_path "$binary" \
		--arg binary_go_version "$binary_go_version" \
		--argjson venue_ids "$(jq -c '.venue_ids' "$config")" \
		'{schema_version: 1, contract: "v2-r2-sv1-activation-provenance-v1",
		 cell: $cell, seed: $seed, simulated_horizon: $horizon,
		 simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		 config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		 git_revision: $git_revision, config_experiment_id: $experiment,
		 hypothesis_id: $hypothesis, evidence_format: $evidence_format, log_mode: $log_mode,
		 venue_ids: $venue_ids, binary_path: $binary_path, binary_go_version: $binary_go_version,
		 command: ["multivenue", "-config", "run-config.json", "-duration", $horizon,
		           "-logdir", ".", "-log-mode", $log_mode, "-evidence-format", $evidence_format]}' \
		>"$arm/run-metadata.json"
}

run_arm() {
	local arm=$1
	local metadata_sha_before
	metadata_sha_before=$(sha256sum -- "$arm/run-metadata.json" | awk '{print $1}')
	local stdout_log="$output_root/$(basename "$arm").stdout.log"
	local stderr_log="$output_root/$(basename "$arm").stderr.log"
	"$binary" -config "$arm/run-config.json" -duration "$horizon" -logdir "$arm" -log-mode full -evidence-format evstream_v3 \
		>"$stdout_log" 2>"$stderr_log"
	[[ -s "$arm/greeks.json" && -s "$arm/latency.json" && -s "$arm/manifest.json" && -s "$arm/checkpoints.jsonl" && -s "$arm/events.evs" && -s "$arm/binary-evidence-attestation.json" ]] || {
		echo "activation arm did not produce all completion evidence: $arm" >&2
		return 1
	}
	jq -e --arg revision "$head_revision" --argjson seed 607 --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		'.build.revision == $revision and .build.modified == false and .venue_ids == ["north", "central", "south"] and
		 .config.seed == $seed and .config.log_mode == "full" and .config.evidence_format == "evstream_v3"' \
		"$arm/manifest.json" >/dev/null
	jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		'(.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $simulation_start_nano)) and
		 (.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $simulation_end_nano))' \
		"$arm/greeks.json" >/dev/null
	v2_r2_require_checkpoint_stream "$arm/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano"
	[[ "$metadata_sha_before" == "$(sha256sum -- "$arm/run-metadata.json" | awk '{print $1}')" ]] || {
		echo "activation metadata changed during simulation: $arm" >&2
		return 1
	}
	v2_r2_write_evidence_manifest "$arm"
	v2_r2_verify_evidence_manifest "$arm"
}

prepare_arm "$treatment_dir" "$treatment_config"
prepare_arm "$control_dir" "$control_config"
run_arm "$treatment_dir"
run_arm "$control_dir"

comparison_tmp="$output_root/cdf-liquidity-comparison.json.tmp-$$"
"$audit_binary" -treatment "$treatment_dir" -control "$control_dir" >"$comparison_tmp"
mv -- "$comparison_tmp" "$output_root/cdf-liquidity-comparison.json"
jq -e \
	'.valid == true and .provenance.valid == true and
	 .treatment.valid == true and .control.valid == true and
	 .treatment.supplier_count > 0 and .control.supplier_count == 0 and
	 .treatment.trading_supplier_count == .treatment.supplier_count and
	 .treatment.pnl_changing_supplier_count == .treatment.supplier_count and
	 (.treatment.cancel_count + .treatment.withdraw_count) > 0 and
	 all(.treatment.suppliers[]; .max_position <= .configured_max_position and
		.max_gross_base_balance <= .configured_max_inventory and
		.max_quote_qty <= .configured_max_quote_qty)' \
	"$output_root/cdf-liquidity-comparison.json" >/dev/null || {
	echo "activation contract failed: suppliers did not demonstrate finite bounded activity" >&2
	exit 1
}

comparison_sha=$(sha256sum -- "$output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
jq -n \
	--arg contract "v2-r2-sv1-activation-pair-v1" \
	--arg candidate "$head_revision" \
	--arg output_root "$output_root" \
	--arg treatment "$treatment_dir" \
	--arg control "$control_dir" \
	--arg treatment_config_sha256 "$(sha256sum -- "$treatment_config" | awk '{print $1}')" \
	--arg control_config_sha256 "$(sha256sum -- "$control_config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum -- "$binary" | awk '{print $1}')" \
	--arg analyzer_sha256 "$(sha256sum -- "$audit_binary" | awk '{print $1}')" \
	--arg comparison_sha256 "$comparison_sha" \
	--argjson seed 607 --arg horizon "$horizon" \
	'{schema_version: 1, contract: $contract, candidate_revision: $candidate,
	 seed: $seed, simulated_horizon: $horizon, output_root: $output_root,
	 treatment_dir: $treatment, control_dir: $control,
	 treatment_config_sha256: $treatment_config_sha256, control_config_sha256: $control_config_sha256,
	 simulator_binary_sha256: $binary_sha256, analyzer_binary_sha256: $analyzer_sha256,
	 comparison_sha256: $comparison_sha256, holdouts_consumed: false,
	 scope: "development-only mechanism activation; not a 24-hour survival claim"}' \
	>"$output_root/activation-provenance.json"

echo "completed V2-R2-SV1 activation probe: $output_root"
