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

[[ ! -e "$v2_r2_output_root" && ! -L "$v2_r2_output_root" ]] ||
	fail "refusing to reuse an existing R2 output root"
[[ ! -e "$v2_r2_attestation_root" && ! -L "$v2_r2_attestation_root" ]] ||
	fail "refusing to reuse an existing R2 attestation root"
mkdir -p -- "$v2_r2_output_root" "$v2_r2_attestation_root"
created_output=true
created_attestations=true

current_revision=$(git -C "$root_dir" rev-parse HEAD)
stale_revision=fedcba9876543210fedcba9876543210fedcba987
matching_binary_sha256=0000000000000000000000000000000000000000000000000000000000000000
matching_prunegate_sha256=1111111111111111111111111111111111111111111111111111111111111111
analyzer="$tmp_root/mvanalyze"
CGO_ENABLED=0 go build -trimpath -o "$analyzer" ./cmd/mvanalyze

expect_failure v2_r2_require_current_source_revision "$stale_revision" "$current_revision" "$current_revision"
expect_failure v2_r2_require_current_source_revision "$current_revision" "$current_revision" "$stale_revision"
expect_failure v2_r2_require_matching_revision "$stale_revision" "$current_revision"

write_metadata() {
	local cell=$1
	local seed=$2
	local log_mode=$3
	local gomaxprocs=$4
	local config=$5
	local hypothesis_id=$6
	local config_sha256
	config_sha256=$(sha256sum -- "$config" | awk '{print $1}')
	jq -n --arg cell "$cell" --argjson seed "$seed" --arg log_mode "$log_mode" \
		--argjson gomaxprocs "$gomaxprocs" --arg hypothesis_id "$hypothesis_id" \
		--arg config_sha256 "$config_sha256" --arg binary_sha256 "$matching_binary_sha256" \
		--arg source_revision "$current_revision" --arg prunegate_sha256 "$matching_prunegate_sha256" \
		'{schema_version: 5, runner_contract: "v2-integrated-longrun-r2-runner-v1", cell: $cell,
		 seed: $seed, holdout: false, log_mode: $log_mode, gomaxprocs: $gomaxprocs,
		 hypothesis_id: $hypothesis_id, git_revision: $source_revision,
		 config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		 binary_vcs_modified: false, binary_trimpath: true, binary_cgo_enabled: "0",
		 binary_go_version: "go1.27.0", prunegate_vcs_revision: $source_revision,
		 prunegate_vcs_modified: false, prunegate_trimpath: true, prunegate_cgo_enabled: "0",
		 prunegate_go_version: "go1.27.0", prunegate_sha256: $prunegate_sha256}' \
		>"$cell/run-metadata.json"
}

write_common_files() {
	local cell=$1
	local experiment_id=$2
	jq -n --arg revision "$current_revision" --arg experiment_id "$experiment_id" \
		'{build: {revision: $revision, modified: false}, config: {experiment_id: $experiment_id}}' \
		>"$cell/manifest.json"
	jq -n '{initial_accounts: [], terminal_accounts: []}' >"$cell/greeks.json"
	jq -n '{latency: []}' >"$cell/latency.json"
	printf '%s\n' '{"checkpoint":1}' >"$cell/checkpoints.jsonl"
}

write_status() {
	local cell=$1
	local metadata_sha256 manifest_sha256 greeks_sha256 latency_sha256 checkpoints_sha256 evidence_manifest_sha256
	metadata_sha256=$(sha256sum -- "$cell/run-metadata.json" | awk '{print $1}')
	manifest_sha256=$(sha256sum -- "$cell/manifest.json" | awk '{print $1}')
	greeks_sha256=$(sha256sum -- "$cell/greeks.json" | awk '{print $1}')
	latency_sha256=$(sha256sum -- "$cell/latency.json" | awk '{print $1}')
	checkpoints_sha256=$(sha256sum -- "$cell/checkpoints.jsonl" | awk '{print $1}')
	evidence_manifest_sha256=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}')
	jq -n --arg metadata_sha256 "$metadata_sha256" --arg manifest_sha256 "$manifest_sha256" \
		--arg greeks_sha256 "$greeks_sha256" --arg latency_sha256 "$latency_sha256" \
		--arg checkpoints_sha256 "$checkpoints_sha256" --arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
		'{exit_status: 0, completion_verified: true, simulated_horizon: "24h",
		 simulation_start_nano: 1735689600000000000, simulation_end_nano: 1735776000000000000,
		 completion_sentinels: ["greeks.json", "latency.json"],
		 run_metadata_sha256: $metadata_sha256, manifest_sha256: $manifest_sha256,
		 greeks_sha256: $greeks_sha256, latency_sha256: $latency_sha256,
		 checkpoints_sha256: $checkpoints_sha256, evidence_manifest_sha256: $evidence_manifest_sha256}' \
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
	printf '%s\n' '{"event":"archive-test","sequence":1}' >"$cell/venues/north/events.jsonl"
	artifact_result=$("$analyzer" -metric evidenceartifacthash -json "$cell")
	jq -e '.result | type == "object"' <<<"$artifact_result" >"$cell/evidence-artifact-hash.json"
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
	v2_r2_write_evidence_manifest "$cell" || fail "could not create no-log evidence manifest"
	write_status "$cell"
}

full_config="$root_dir/research/configs/v2-integrated-longrun-r2/dev-607.json"
none_config="$root_dir/research/configs/v2-integrated-longrun-r2/dev-607-none.json"
write_full_cell "$v2_r2_output_root/dev-607" "$full_config" 4 "V2-INTEGRATED-LONG-R2-CANDIDATE" "v2-integrated-longrun-r2-dev-607"
write_none_cell "$v2_r2_output_root/dev-607-none" "$none_config"
write_full_cell "$v2_r2_output_root/dev-607-g8" "$full_config" 8 "V2-INTEGRATED-LONG-R2-CANDIDATE" "v2-integrated-longrun-r2-dev-607"

jq -n '{result: {contract: "v2-integrated-longrun-r2-candidate-v1", predicates: {calendar_behavior_attested: true}}}' \
	>"$v2_r2_output_root/dev-607/activation.json"
jq -n '{contract: "v2-integrated-longrun-r2-candidate-v1", predicates: {fixture: true}}' \
	>"$v2_r2_output_root/dev-607/integrity.json"
for cell in dev-607 dev-607-none dev-607-g8; do
	v2_r2_write_attestation "$v2_r2_output_root/$cell" || fail "could not write fixture attestation: $cell"
done

GOMAXPROCS=1 MVANALYZE_BIN="$analyzer" \
	"$root_dir/scripts/check-v2-integrated-longrun-r2-parity.sh" "$v2_r2_output_root" >/dev/null ||
	fail "matching G8 parity fixture was rejected"
G8_ARCHIVE_OUTPUT=$(GOMAXPROCS=1 MVANALYZE_BIN="$analyzer" \
	"$root_dir/scripts/archive-v2-integrated-longrun-r2-cell.sh" \
		"$v2_r2_output_root/dev-607-g8" --prune-after-verify) ||
	fail "matching G8 archive/prune fixture was rejected"
[[ -n "$G8_ARCHIVE_OUTPUT" ]] || fail "matching G8 archive produced no completion output"
[[ ! -e "$v2_r2_output_root/dev-607-g8/venues/north/events.jsonl" ]] ||
	fail "successful G8 archive did not prune its raw fixture"

v2_r2_stage_raw_evidence "$v2_r2_output_root/dev-607-g8" || fail "could not restore G8 fixture for stale-gate test"
jq --arg stale_revision "$stale_revision" \
	'.prunegate_vcs_revision = $stale_revision' \
	"$v2_r2_output_root/dev-607/run-metadata.json" >"$tmp_root/stale-metadata.json"
mv -- "$tmp_root/stale-metadata.json" "$v2_r2_output_root/dev-607/run-metadata.json"
stale_metadata_sha256=$(sha256sum -- "$v2_r2_output_root/dev-607/run-metadata.json" | awk '{print $1}')
stale_metadata_bytes=$(stat -c '%s' -- "$v2_r2_output_root/dev-607/run-metadata.json")
jq --arg sha256 "$stale_metadata_sha256" --argjson bytes "$stale_metadata_bytes" \
	'.fixed_files |= map(if .path == "run-metadata.json" then .sha256 = $sha256 | .bytes = $bytes else . end)' \
	"$v2_r2_output_root/dev-607/evidence-manifest.json" >"$tmp_root/stale-manifest.json"
mv -- "$tmp_root/stale-manifest.json" "$v2_r2_output_root/dev-607/evidence-manifest.json"
new_status_sha256=$(sha256sum -- "$v2_r2_output_root/dev-607/run-status.json" | awk '{print $1}')
new_manifest_sha256=$(sha256sum -- "$v2_r2_output_root/dev-607/evidence-manifest.json" | awk '{print $1}')
jq --arg metadata_sha256 "$stale_metadata_sha256" \
	'.run_metadata_sha256 = $metadata_sha256' \
	"$v2_r2_output_root/dev-607/run-status.json" >"$tmp_root/stale-status.json"
mv -- "$tmp_root/stale-status.json" "$v2_r2_output_root/dev-607/run-status.json"
new_status_sha256=$(sha256sum -- "$v2_r2_output_root/dev-607/run-status.json" | awk '{print $1}')
jq --arg status_sha256 "$new_status_sha256" \
	'.run_status_sha256 = $status_sha256' \
	"$v2_r2_attestation_root/dev-607.json" >"$tmp_root/stale-attestation.json"
mv -- "$tmp_root/stale-attestation.json" "$v2_r2_attestation_root/dev-607.json"
expect_failure env GOMAXPROCS=1 MVANALYZE_BIN="$analyzer" \
	"$root_dir/scripts/archive-v2-integrated-longrun-r2-cell.sh" \
		"$v2_r2_output_root/dev-607-g8" --prune-after-verify
[[ -e "$v2_r2_output_root/dev-607-g8/venues/north/events.jsonl" ]] ||
	fail "failed G8 provenance check deleted raw evidence"
v2_r2_cleanup_staged_raw_evidence "$v2_r2_output_root/dev-607-g8" ||
	fail "stale-gate fixture cleanup failed"

printf 'integrated long-run R2 archive tests: pass\n'
