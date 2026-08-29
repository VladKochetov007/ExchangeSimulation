#!/usr/bin/env bash

# Shared fail-closed checks for the immutable r5 evidence namespace. This
# namespace is deliberately distinct from r4: r4 remains historical evidence,
# including its stale public-entry position/settlement derivations.
v2_r5_output_root="/home/vlad/v2-integrated-longrun-candidate-20260828-v5"
v2_r5_attestation_root="/home/vlad/v2-integrated-longrun-candidate-20260828-v5-attestations"

v2_r5_require_output_root() {
	local output_root=$1
	[[ "$output_root" == "$v2_r5_output_root" ]] || return 1
	[[ ! -L "$output_root" ]] || return 1
	[[ "$(realpath -m -- "$output_root")" == "$v2_r5_output_root" ]] || return 1
	if [[ -e "$output_root" ]]; then
		[[ -d "$output_root" ]] || return 1
		[[ "$(realpath -e -- "$output_root")" == "$v2_r5_output_root" ]] || return 1
	fi
}

v2_r5_require_cell_path() {
	local cell_path=$1
	[[ "$cell_path" == /* ]] || return 1
	local current=/ component
	local path_without_root=${cell_path#/}
	IFS=/ read -r -a path_components <<< "$path_without_root"
	for component in "${path_components[@]}"; do
		[[ -n "$component" ]] || continue
		current="${current%/}/$component"
		[[ ! -L "$current" ]] || return 1
	done
	[[ -d "$cell_path" && ! -L "$cell_path" ]] || return 1
	local canonical_cell
	canonical_cell=$(realpath -e -- "$cell_path") || return 1
	[[ "$(realpath -e -- "$v2_r5_output_root")" == "$v2_r5_output_root" ]] || return 1
	[[ "${canonical_cell%/*}" == "$v2_r5_output_root" ]] || return 1
	[[ "$(realpath -m -- "$cell_path")" == "$canonical_cell" ]]
}

v2_r5_binary_go_version() {
	go version -m "$1" | sed -n '1s/.*: //p'
}

v2_r5_is_go_127() {
	[[ "$1" == go1.27* ]]
}

v2_r5_require_attestation_path() {
	local cell=$1
	[[ "$cell" == dev-607 || "$cell" == dev-613 || "$cell" == dev-617 ||
		"$cell" == dev-607-none || "$cell" == dev-607-g8 ]] || return 1
	[[ ! -L "$v2_r5_attestation_root" ]] || return 1
	[[ ! -L "$v2_r5_attestation_root/$cell.json" ]] || return 1
	[[ "$(realpath -m -- "$v2_r5_attestation_root/$cell.json")" == "$v2_r5_attestation_root/$cell.json" ]]
}

v2_r5_write_evidence_manifest() {
	local cell=$1
	local output="$cell/evidence-manifest.json"
	local log_mode
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json") || return 1
	case "$log_mode" in
		full) local fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-artifact-hash.json) ;;
		none) local fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl) ;;
		*) return 1 ;;
	esac
	local fixed_records='[]'
	local relative path bytes digest
	for relative in "${fixed_files[@]}"; do
		path="$cell/$relative"
		[[ -s "$path" ]] || return 1
		bytes=$(stat -c '%s' -- "$path") || return 1
		digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		fixed_records=$(jq -c --arg path "$relative" --arg digest "$digest" --argjson bytes "$bytes" \
			'. + [{path: $path, bytes: $bytes, sha256: $digest}]' <<<"$fixed_records") || return 1
	done
	local raw_records='[]'
	local raw_count=0
	local raw_bytes=0
	[[ -d "$cell/venues" ]] || return 1
	while IFS= read -r -d '' path; do
		relative=${path#"$cell/"}
		bytes=$(stat -c '%s' -- "$path") || return 1
		digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		raw_records=$(jq -c --arg path "$relative" --arg digest "$digest" --argjson bytes "$bytes" \
			'. + [{path: $path, bytes: $bytes, sha256: $digest}]' <<<"$raw_records") || return 1
		raw_count=$((raw_count + 1))
		raw_bytes=$((raw_bytes + bytes))
	done < <(find "$cell/venues" -type f -name '*.jsonl' -print0 | sort -z)
	if [[ "$log_mode" == full ]]; then
		(( raw_count > 0 )) || return 1
	else
		(( raw_count == 0 )) || return 1
	fi
	local temporary="$output.tmp-$$"
	jq -n \
		--arg contract "v2-integrated-longrun-evidence-manifest-v1" \
		--arg cell "$(basename "$cell")" \
		--arg log_mode "$log_mode" \
		--arg source_revision "$(jq -er '.git_revision' "$cell/run-metadata.json")" \
		--argjson fixed_files "$fixed_records" --argjson raw_files "$raw_records" \
		--argjson raw_count "$raw_count" --argjson raw_bytes "$raw_bytes" \
		'{schema_version: 1, contract: $contract, cell: $cell, log_mode: $log_mode, source_revision: $source_revision,
			fixed_files: $fixed_files, raw_jsonl_files: $raw_count, raw_jsonl_bytes: $raw_bytes,
		 raw_files: $raw_files}' >"$temporary" || return 1
	mv -- "$temporary" "$output"
}

v2_r5_verify_evidence_manifest() {
	local cell=$1
	local manifest="$cell/evidence-manifest.json"
	[[ -s "$manifest" ]] || return 1
	local log_mode
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json") || return 1
	local expected_fixed
	case "$log_mode" in
		full) expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-artifact-hash.json | sort) ;;
		none) expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl | sort) ;;
		*) return 1 ;;
	esac
	jq -e --arg cell "$(basename "$cell")" --arg log_mode "$log_mode" \
		'.schema_version == 1 and .contract == "v2-integrated-longrun-evidence-manifest-v1" and .cell == $cell and
			.log_mode == $log_mode and
			(.source_revision | test("^[0-9a-f]{40}$")) and (.fixed_files | type) == "array" and
			(.raw_files | type) == "array" and .raw_jsonl_files == (.raw_files | length) and
			(if $log_mode == "full" then .raw_jsonl_files > 0 else .raw_jsonl_files == 0 end)' "$manifest" >/dev/null || return 1
	local listed
	listed=$(jq -r '.fixed_files[].path' "$manifest" | sort)
	[[ "$listed" == "$expected_fixed" ]] || return 1
	local relative path expected_bytes expected_digest actual_bytes actual_digest
	local actual_raw_bytes=0
	while IFS=$'\t' read -r relative expected_bytes expected_digest; do
		path="$cell/$relative"
		[[ -f "$path" && ! -L "$path" ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$path") || return 1
		actual_digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_digest" == "$expected_digest" ]] || return 1
	done < <(jq -r '.fixed_files[] | [.path, (.bytes | tostring), .sha256] | @tsv' "$manifest")
	while IFS=$'\t' read -r relative expected_bytes expected_digest; do
		path="$cell/$relative"
		[[ "$relative" == venues/*.jsonl && -f "$path" && ! -L "$path" ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$path") || return 1
		actual_digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_digest" == "$expected_digest" ]] || return 1
		actual_raw_bytes=$((actual_raw_bytes + actual_bytes))
	done < <(jq -r '.raw_files[] | [.path, (.bytes | tostring), .sha256] | @tsv' "$manifest")
	[[ "$actual_raw_bytes" == "$(jq -er '.raw_jsonl_bytes' "$manifest")" ]] || return 1
	local actual
	listed=$(jq -r '.raw_files[].path' "$manifest" | sort)
	actual=$(find "$cell/venues" -type f -name '*.jsonl' -printf '%P\n' | sed 's#^#venues/#' | sort)
	[[ "$listed" == "$actual" ]] || return 1
	return 0
}

v2_r5_write_attestation() {
	local cell=$1
	local cell_name
	cell_name=$(basename "$cell")
	v2_r5_require_attestation_path "$cell_name" || return 1
	mkdir -p -- "$v2_r5_attestation_root"
	local output="$v2_r5_attestation_root/$cell_name.json"
	[[ ! -e "$output" ]] || return 1
	local temporary="$output.tmp-$$"
	local manifest_sha status_sha
	manifest_sha=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}') || return 1
	status_sha=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}') || return 1
	jq -n --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		'{schema_version: 1, contract: "v2-integrated-longrun-external-attestation-v1", cell: $cell,
		 evidence_manifest_sha256: $manifest_sha, run_status_sha256: $status_sha,
		 attestation_scope: "runner-produced evidence manifest and completion status"}' >"$temporary" || return 1
	mv -- "$temporary" "$output"
}

v2_r5_verify_attestation() {
	local cell=$1
	local cell_name
	cell_name=$(basename "$cell")
	v2_r5_require_attestation_path "$cell_name" || return 1
	local attestation="$v2_r5_attestation_root/$cell_name.json"
	[[ -s "$attestation" ]] || return 1
	local manifest_sha status_sha
	manifest_sha=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}') || return 1
	status_sha=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}') || return 1
	jq -e --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		'.schema_version == 1 and .contract == "v2-integrated-longrun-external-attestation-v1" and
		 .cell == $cell and .evidence_manifest_sha256 == $manifest_sha and .run_status_sha256 == $status_sha' \
		"$attestation" >/dev/null
}
