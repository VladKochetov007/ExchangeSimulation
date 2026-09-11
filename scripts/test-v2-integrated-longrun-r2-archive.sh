#!/usr/bin/env bash
# Hermetic G8 archive regression. It creates only a temporary canonical R2
# fixture, exercises the real parity and archive scripts, and removes only the
# fixture after the success and fail-closed cases complete.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
tmp_root=$(mktemp -d)
created_output=false
created_attestations=false

cleanup() {
	if [[ "$created_output" == true ]]; then
		rm -rf -- "$v2_r2_output_root"
	fi
	if [[ "$created_attestations" == true ]]; then
		rm -rf -- "$v2_r2_attestation_root"
	fi
	rm -rf -- "$tmp_root"
}
trap cleanup EXIT

fail() {
	printf 'integrated long-run R2 archive test failure: %s\n' "$*" >&2
	exit 1
}

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		fail "command unexpectedly succeeded: $*"
	fi
}

if ! v2_r2_acquire_namespace_lock; then
	printf 'integrated long-run R2 archive tests: skipped (namespace lock busy)\n'
	exit 0
fi
if ! mkdir -- "$v2_r2_output_root" 2>/dev/null; then
	printf 'integrated long-run R2 archive tests: skipped (R2 output root already exists)\n'
	exit 0
fi
created_output=true
if ! mkdir -- "$v2_r2_attestation_root" 2>/dev/null; then
	if rmdir -- "$v2_r2_output_root"; then
		created_output=false
		printf 'integrated long-run R2 archive tests: skipped (R2 attestation root already exists)\n'
		exit 0
	fi
	created_output=false
	fail "could not safely abandon the test output root"
fi
created_attestations=true

current_revision=$(git -C "$root_dir" rev-parse HEAD)
stale_revision=fedcba9876543210fedcba9876543210fedcba98
matching_binary_sha256=0000000000000000000000000000000000000000000000000000000000000000
matching_prunegate_sha256=1111111111111111111111111111111111111111111111111111111111111111
analyzer="$tmp_root/mvanalyze"
CGO_ENABLED=0 go build -trimpath -o "$analyzer" ./cmd/mvanalyze
renderer="$tmp_root/evsrender"
CGO_ENABLED=0 go build -trimpath -o "$renderer" ./cmd/evsrender
fixture_binary="$tmp_root/evstream-fixture"
CGO_ENABLED=0 go build -trimpath -o "$fixture_binary" ./scripts/testdata/evstream-fixture

expect_failure v2_r2_require_current_source_revision "$stale_revision" "$current_revision" "$current_revision"
expect_failure v2_r2_require_current_source_revision "$current_revision" "$current_revision" "$stale_revision"
expect_failure v2_r2_require_matching_revision "$stale_revision" "$current_revision"

write_metadata() {
	local cell=$1
	local cell_name
	cell_name=$(basename "$cell")
	local seed=$2
	local log_mode=$3
	local gomaxprocs=$4
	local config=$5
	local hypothesis_id=$6
	local config_sha256
	config_sha256=$(sha256sum -- "$config" | awk '{print $1}')
	jq -n --arg cell "$cell_name" --argjson seed "$seed" --arg log_mode "$log_mode" \
		--argjson gomaxprocs "$gomaxprocs" --arg hypothesis_id "$hypothesis_id" \
		--arg config_sha256 "$config_sha256" --arg binary_sha256 "$matching_binary_sha256" \
		--arg source_revision "$current_revision" --arg prunegate_sha256 "$matching_prunegate_sha256" \
		'{schema_version: 6, runner_contract: "v2-integrated-longrun-r2-runner-v2", cell: $cell,
		 seed: $seed, holdout: false, log_mode: $log_mode, evidence_format: "evstream_v3", gomaxprocs: $gomaxprocs,
		 hypothesis_id: $hypothesis_id, git_revision: $source_revision,
		 config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		 binary_vcs_revision: $source_revision, binary_vcs_modified: false,
		 binary_trimpath: true, binary_cgo_enabled: "0",
		 binary_go_version: "go1.27.0", prunegate_vcs_revision: $source_revision,
		 prunegate_vcs_modified: false, prunegate_trimpath: true, prunegate_cgo_enabled: "0",
		 prunegate_go_version: "go1.27.0", prunegate_sha256: $prunegate_sha256}' \
		>"$cell/run-metadata.json"
}

write_common_files() {
	local cell=$1
	local experiment_id=$2
	local log_mode evidence_contract_version
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json")
	evidence_contract_version=$(jq -er '.evidence_contract_version' "$cell/run-config.json")
	jq -n --arg revision "$current_revision" --arg experiment_id "$experiment_id" --arg log_mode "$log_mode" \
		--argjson evidence_contract_version "$evidence_contract_version" \
		'{build: {revision: $revision, modified: false}, config: {experiment_id: $experiment_id, evidence_format: "evstream_v3", log_mode: $log_mode, evidence_contract_version: $evidence_contract_version}}' \
		>"$cell/manifest.json"
	jq -n '{initial_accounts: [], terminal_accounts: []}' >"$cell/greeks.json"
	jq -n '{latency: []}' >"$cell/latency.json"
	printf '%s\n' '{"checkpoint":1}' >"$cell/checkpoints.jsonl"
}

write_status() {
	local cell=$1
	local metadata_sha256 manifest_sha256 greeks_sha256 latency_sha256 checkpoints_sha256 evidence_manifest_sha256 binary_attestation_sha256
	local record_market_data_receipts market_data_evidence_sha256='' market_data_schedules_sha256='' market_data_receipts_sha256='' market_data_decisions_sha256=''
	record_market_data_receipts=$(jq -r '.record_market_data_receipts // false' "$cell/run-config.json")
	metadata_sha256=$(sha256sum -- "$cell/run-metadata.json" | awk '{print $1}')
	manifest_sha256=$(sha256sum -- "$cell/manifest.json" | awk '{print $1}')
	greeks_sha256=$(sha256sum -- "$cell/greeks.json" | awk '{print $1}')
	latency_sha256=$(sha256sum -- "$cell/latency.json" | awk '{print $1}')
	checkpoints_sha256=$(sha256sum -- "$cell/checkpoints.jsonl" | awk '{print $1}')
	evidence_manifest_sha256=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}')
	binary_attestation_sha256=$(sha256sum -- "$cell/binary-evidence-attestation.json" | awk '{print $1}')
	if [[ "$record_market_data_receipts" == true ]]; then
		market_data_evidence_sha256=$(sha256sum -- "$cell/market-data-evidence-v2.json" | awk '{print $1}')
		market_data_schedules_sha256=$(sha256sum -- "$cell/market-data-schedules-v2.bin" | awk '{print $1}')
		market_data_receipts_sha256=$(sha256sum -- "$cell/market-data-receipts-v2.bin" | awk '{print $1}')
		market_data_decisions_sha256=$(sha256sum -- "$cell/market-data-decisions-v2.bin" | awk '{print $1}')
	fi
	jq -n --arg metadata_sha256 "$metadata_sha256" --arg manifest_sha256 "$manifest_sha256" \
		--arg greeks_sha256 "$greeks_sha256" --arg latency_sha256 "$latency_sha256" \
		--arg checkpoints_sha256 "$checkpoints_sha256" --arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
		--arg binary_attestation_sha256 "$binary_attestation_sha256" \
		--argjson record_market_data_receipts "$record_market_data_receipts" \
		--arg market_data_evidence_sha256 "$market_data_evidence_sha256" \
		--arg market_data_schedules_sha256 "$market_data_schedules_sha256" \
		--arg market_data_receipts_sha256 "$market_data_receipts_sha256" \
		--arg market_data_decisions_sha256 "$market_data_decisions_sha256" \
		'{schema_version: 1, exit_status: 0, completion_verified: true, simulated_horizon: "24h",
		 simulation_start_nano: 1735689600000000000, simulation_end_nano: 1735776000000000000,
		 completion_sentinels: ["greeks.json", "latency.json"],
		 run_metadata_sha256: $metadata_sha256, manifest_sha256: $manifest_sha256,
		 greeks_sha256: $greeks_sha256, latency_sha256: $latency_sha256,
		 checkpoints_sha256: $checkpoints_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
		 binary_evidence_attestation_sha256: $binary_attestation_sha256} |
		(if $record_market_data_receipts then . + {
			market_data_evidence_sha256: $market_data_evidence_sha256,
			market_data_schedules_sha256: $market_data_schedules_sha256,
			market_data_receipts_sha256: $market_data_receipts_sha256,
			market_data_decisions_sha256: $market_data_decisions_sha256
		} else . end)' \
		>"$cell/run-status.json"
}

write_full_cell() {
	local cell=$1
	local config=$2
	local gomaxprocs=$3
	local hypothesis_id=$4
	local experiment_id=$5
	mkdir -p -- "$cell/venues/north"
	cp -- "$config" "$cell/run-config.json"
	write_metadata "$cell" 607 full "$gomaxprocs" "$config" "$hypothesis_id"
	write_common_files "$cell" "$experiment_id"
	"$fixture_binary" -out "$cell/events.evs" -attestation "$cell/binary-evidence-attestation.json" -sequence 2
	printf '%s\n' '{}' >"$cell/market-data-evidence-v2.json"
	printf '%s\n' 'schedule-fixture' >"$cell/market-data-schedules-v2.bin"
	printf '%s\n' 'receipt-fixture' >"$cell/market-data-receipts-v2.bin"
	printf '%s\n' 'decision-fixture' >"$cell/market-data-decisions-v2.bin"
	v2_r2_write_evidence_manifest "$cell" || fail "could not create full evidence manifest: $cell"
	write_status "$cell"
}

write_none_cell() {
	local cell=$1
	local config=$2
	mkdir -p -- "$cell/venues"
	cp -- "$config" "$cell/run-config.json"
	write_metadata "$cell" 607 none 4 "$config" "V2-INTEGRATED-LONG-R2-CANDIDATE-PARITY"
	write_common_files "$cell" "v2-integrated-longrun-r2-dev-607-none"
	"$fixture_binary" -out "$cell/events.evs" -attestation "$cell/binary-evidence-attestation.json" -sequence 2
	v2_r2_write_evidence_manifest "$cell" || fail "could not create no-log evidence manifest"
	write_status "$cell"
}

full_config="$root_dir/research/configs/v2-integrated-longrun-r2/dev-607.json"
none_config="$root_dir/research/configs/v2-integrated-longrun-r2/dev-607-none.json"
write_full_cell "$v2_r2_output_root/dev-607" "$full_config" 4 "V2-INTEGRATED-LONG-R2-CANDIDATE" "v2-integrated-longrun-r2-dev-607"
write_none_cell "$v2_r2_output_root/dev-607-none" "$none_config"
write_full_cell "$v2_r2_output_root/dev-607-g8" "$full_config" 8 "V2-INTEGRATED-LONG-R2-CANDIDATE" "v2-integrated-longrun-r2-dev-607"

jq -n '{result: {contract: "v2-integrated-longrun-r2-candidate-v2", predicates: {calendar_behavior_attested: true}}}' \
	>"$v2_r2_output_root/dev-607/activation.json"
jq -n '{contract: "v2-integrated-longrun-r2-candidate-v2", predicates: {fixture: true}}' \
	>"$v2_r2_output_root/dev-607/integrity.json"
for cell in dev-607 dev-607-none dev-607-g8; do
	v2_r2_write_attestation "$v2_r2_output_root/$cell" || fail "could not write fixture attestation: $cell"
done

GOMAXPROCS=1 MVANALYZE_BIN="$analyzer" EVSRENDER_BIN="$renderer" \
	"$root_dir/scripts/check-v2-integrated-longrun-r2-parity.sh" "$v2_r2_output_root" >/dev/null ||
	fail "matching G8 parity fixture was rejected"
expect_failure env GOMAXPROCS=1 MVANALYZE_BIN="$analyzer" \
	"$root_dir/scripts/archive-v2-integrated-longrun-r2-cell.sh" \
		"$v2_r2_output_root/dev-607-g8" --prune-after-verify
[[ -e "$v2_r2_output_root/dev-607-g8/events.evs" ]] || fail "binary archive rejection removed canonical evidence"
[[ -d "$v2_r2_output_root/dev-607-g8/venues/north" ]] || fail "binary archive rejection changed venue namespace"

printf 'integrated long-run R2 archive tests: pass\n'
