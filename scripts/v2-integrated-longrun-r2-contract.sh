#!/usr/bin/env bash

# Shared fail-closed checks for the immutable R2 evidence namespace. This
# namespace is deliberately distinct from r4: r4 remains historical evidence,
# including its stale public-entry position/settlement derivations.
v2_r2_output_root="/home/vlad/v2-integrated-longrun-r2-candidate-20260830-v1"
v2_r2_attestation_root="/home/vlad/v2-integrated-longrun-r2-candidate-20260830-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-integrated-longrun-r2-candidate.lock"

v2_r2_acquire_namespace_lock() {
	[[ ! -L "$v2_r2_namespace_lock_path" ]] || return 1
	local inherited_fd=${V2_R2_NAMESPACE_LOCK_FD:-}
	if [[ "$inherited_fd" =~ ^[0-9]+$ ]] &&
		[[ "$(readlink "/proc/$$/fd/$inherited_fd" 2>/dev/null)" == "$v2_r2_namespace_lock_path" ]] &&
		flock -n "$inherited_fd"; then
		export V2_R2_NAMESPACE_LOCK_FD="$inherited_fd"
		return 0
	fi
	local lock_fd
	exec {lock_fd}>"$v2_r2_namespace_lock_path" || return 1
	flock -n "$lock_fd" || return 1
	export V2_R2_NAMESPACE_LOCK_FD="$lock_fd"
}

v2_r2_require_output_root() {
	local output_root=$1
	[[ "$output_root" == "$v2_r2_output_root" ]] || return 1
	[[ ! -L "$output_root" ]] || return 1
	[[ "$(realpath -m -- "$output_root")" == "$v2_r2_output_root" ]] || return 1
	if [[ -e "$output_root" ]]; then
		[[ -d "$output_root" ]] || return 1
		[[ "$(realpath -e -- "$output_root")" == "$v2_r2_output_root" ]] || return 1
	fi
}

v2_r2_require_cell_path() {
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
	[[ "$(realpath -e -- "$v2_r2_output_root")" == "$v2_r2_output_root" ]] || return 1
	[[ "${canonical_cell%/*}" == "$v2_r2_output_root" ]] || return 1
	[[ "$(realpath -m -- "$cell_path")" == "$canonical_cell" ]]
}

v2_r2_binary_go_version() {
	go version -m "$1" | sed -n '1s/.*: //p'
}

v2_r2_is_go_127() {
	[[ "$1" == go1.27* ]]
}

v2_r2_require_matching_revision() {
	local actual_revision=$1
	local expected_revision=$2
	[[ "$actual_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	[[ "$actual_revision" == "$expected_revision" ]]
}

v2_r2_require_current_source_revision() {
	local source_revision=$1
	local head_revision=$2
	local analyzer_revision=$3
	v2_r2_require_matching_revision "$source_revision" "$head_revision" || return 1
	v2_r2_require_matching_revision "$source_revision" "$analyzer_revision"
}

v2_r2_require_attestation_path() {
	local cell=$1
	[[ "$cell" == dev-607 || "$cell" == dev-613 || "$cell" == dev-617 ||
		"$cell" == dev-607-none || "$cell" == dev-607-g8 ]] || return 1
	[[ ! -L "$v2_r2_attestation_root" ]] || return 1
	[[ ! -L "$v2_r2_attestation_root/$cell.json" ]] || return 1
	[[ "$(realpath -m -- "$v2_r2_attestation_root/$cell.json")" == "$v2_r2_attestation_root/$cell.json" ]]
}

v2_r2_raw_archive_attestation_path() {
	printf '%s/%s.raw-archive.json\n' "$v2_r2_attestation_root" "$1"
}

v2_r2_require_raw_archive_attestation_path() {
	local cell=$1
	v2_r2_require_attestation_path "$cell" || return 1
	local path
	path=$(v2_r2_raw_archive_attestation_path "$cell")
	[[ ! -L "$path" ]] || return 1
	[[ "$(realpath -m -- "$path")" == "$path" ]]
}

v2_r2_write_evidence_manifest() {
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

v2_r2_verify_evidence_manifest() {
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
	v2_r2_verify_venue_namespace "$cell" || return 1
	while IFS=$'\t' read -r relative expected_bytes expected_digest; do
		path="$cell/$relative"
		[[ -f "$path" && ! -L "$path" ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$path") || return 1
		actual_digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_digest" == "$expected_digest" ]] || return 1
	done < <(jq -r '.fixed_files[] | [.path, (.bytes | tostring), .sha256] | @tsv' "$manifest")
	if [[ "$log_mode" == full ]] && ! find "$cell/venues" -type f -name '*.jsonl' -print -quit 2>/dev/null | grep -q .; then
		v2_r2_verify_raw_evidence_archive "$cell" || return 1
		[[ "$(jq -er '.source_revision' "$manifest")" == "$(jq -er '.git_revision' "$cell/run-metadata.json")" ]] || return 1
		return 0
	fi
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
	[[ "$(jq -er '.source_revision' "$manifest")" == "$(jq -er '.git_revision' "$cell/run-metadata.json")" ]] || return 1
	return 0
}

v2_r2_raw_archive_path() {
	printf '%s/raw-evidence.tar.zst\n' "$1"
}

v2_r2_raw_archive_descriptor_path() {
	printf '%s/raw-evidence-archive.json\n' "$1"
}

v2_r2_validate_raw_path() {
	local relative=$1
	[[ "$relative" == venues/*.jsonl && "$relative" != *$'\n'* && "$relative" != *$'\t'* ]] || return 1
	local components component
	IFS=/ read -r -a components <<<"$relative"
	[[ "${components[0]:-}" == venues && "${#components[@]}" -ge 2 ]] || return 1
	for component in "${components[@]:1}"; do
		[[ -n "$component" && "$component" != . && "$component" != .. ]] || return 1
	done
}

v2_r2_verify_venue_namespace() {
	local cell=$1
	[[ ! -L "$cell/venues" ]] || return 1
	if [[ -e "$cell/venues" ]]; then
		[[ -d "$cell/venues" && ! -L "$cell/venues" ]] || return 1
	else
		return 0
	fi
	if find "$cell/venues" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	if find "$cell/venues" -type f ! -name '*.jsonl' -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
}

v2_r2_verify_venue_directories() {
	local cell=$1 manifest=$2
	local expected actual
	expected=$({
		printf 'venues\n'
		jq -r '.raw_files[].path' "$manifest" | awk -F/ '{
		path = $1
		for (level = 2; level < NF; level++) {
			path = path "/" $level
			print path
		}
		}'
	} | sort -u)
	actual=$({
		printf 'venues\n'
		find "$cell/venues" -mindepth 1 -type d -printf '%P\n' | sed 's#^#venues/#'
	} | sort -u)
	[[ "$actual" == "$expected" ]]
}

v2_r2_verify_raw_evidence_archive() {
	local cell=$1
	local archive descriptor evidence_manifest
	archive=$(v2_r2_raw_archive_path "$cell")
	descriptor=$(v2_r2_raw_archive_descriptor_path "$cell")
	evidence_manifest="$cell/evidence-manifest.json"
	[[ -f "$archive" && ! -L "$archive" && -s "$archive" ]] || return 1
	[[ -f "$descriptor" && ! -L "$descriptor" && -s "$descriptor" ]] || return 1
	[[ -f "$evidence_manifest" && ! -L "$evidence_manifest" && -s "$evidence_manifest" ]] || return 1
	local expected_archive_sha actual_archive_sha
	expected_archive_sha=$(jq -er '.archive_sha256' "$descriptor") || return 1
	[[ "$expected_archive_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
	actual_archive_sha=$(sha256sum -- "$archive" | awk '{print $1}') || return 1
	[[ "$actual_archive_sha" == "$expected_archive_sha" ]] || return 1
	local expected_manifest_sha actual_manifest_sha
	expected_manifest_sha=$(jq -er '.evidence_manifest_sha256' "$descriptor") || return 1
	actual_manifest_sha=$(sha256sum -- "$evidence_manifest" | awk '{print $1}') || return 1
	[[ "$actual_manifest_sha" == "$expected_manifest_sha" ]] || return 1
	local expected_status_sha actual_status_sha
	[[ -s "$cell/run-status.json" ]] || return 1
	expected_status_sha=$(jq -er '.run_status_sha256' "$descriptor") || return 1
	actual_status_sha=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}') || return 1
	[[ "$actual_status_sha" == "$expected_status_sha" ]] || return 1
	local cell_name log_mode source_revision
	cell_name=$(basename "$cell")
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json") || return 1
	source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json") || return 1
	jq -e --arg cell "$cell_name" --arg log_mode "$log_mode" --arg source_revision "$source_revision" \
		'.schema_version == 1 and .contract == "v2-integrated-longrun-raw-archive-v1" and
		 .cell == $cell and .log_mode == $log_mode and .source_revision == $source_revision and
		 .archive == "raw-evidence.tar.zst" and (.archive_bytes | type) == "number" and
		 (.archive_sha256 | test("^[0-9a-f]{64}$")) and
		 (.evidence_manifest_sha256 | test("^[0-9a-f]{64}$")) and
		 (.run_status_sha256 | test("^[0-9a-f]{64}$")) and
		 (.raw_files | type) == "array" and (.raw_jsonl_files | type) == "number" and
		 (.raw_jsonl_bytes | type) == "number"' "$descriptor" >/dev/null || return 1
	local expected_raw actual_raw
	expected_raw=$(jq -S -c '.raw_files' "$evidence_manifest") || return 1
	actual_raw=$(jq -S -c '.raw_files' "$descriptor") || return 1
	[[ "$expected_raw" == "$actual_raw" ]] || return 1
	local relative
	while IFS= read -r relative; do
		v2_r2_validate_raw_path "$relative" || return 1
	done < <(jq -r '.raw_files[].path' "$evidence_manifest")
	v2_r2_verify_venue_namespace "$cell" || return 1
	if [[ "$log_mode" == full ]]; then
		v2_r2_verify_venue_directories "$cell" "$evidence_manifest" || return 1
	fi
	local expected_archive_bytes actual_archive_bytes
	expected_archive_bytes=$(jq -er '.archive_bytes' "$descriptor") || return 1
	[[ "$expected_archive_bytes" =~ ^[0-9]+$ ]] || return 1
	actual_archive_bytes=$(stat -c '%s' -- "$archive") || return 1
	[[ "$actual_archive_bytes" == "$expected_archive_bytes" ]] || return 1
	local listed expected
	listed=$(tar --use-compress-program='zstd -q' -tf "$archive" | sed '/\/$/d' | sort) || return 1
	expected=$(jq -r '.raw_files[].path' "$evidence_manifest" | sort) || return 1
	[[ "$listed" == "$expected" ]] || return 1
	local listed_directories expected_directories
	listed_directories=$(tar --use-compress-program='zstd -q' -tf "$archive" | awk '/\/$/ {sub(/\/$/, ""); print}' | sort) || return 1
	expected_directories=$({
		printf 'venues\n'
		jq -r '.raw_files[].path' "$evidence_manifest" | awk -F/ '{
		path = $1
		for (level = 2; level < NF; level++) {
			path = path "/" $level
			print path
		}
		}'
	} | sort -u) || return 1
	[[ "$listed_directories" == "$expected_directories" ]] || return 1
	if tar --use-compress-program='zstd -q' -tvf "$archive" | awk '$1 !~ /^[d-]/ {exit 1}'; then
		:
	else
		return 1
	fi
	local expected_raw_bytes actual_raw_bytes
	expected_raw_bytes=$(jq -er '.raw_jsonl_bytes' "$evidence_manifest") || return 1
	[[ "$expected_raw_bytes" =~ ^[0-9]+$ ]] || return 1
	actual_raw_bytes=$(jq -r '.raw_files[].bytes' "$evidence_manifest" | awk '{total += $1} END {print total + 0}') || return 1
	[[ "$actual_raw_bytes" == "$expected_raw_bytes" ]] || return 1
	jq -e --argjson raw_jsonl_files "$(jq -er '.raw_jsonl_files' "$evidence_manifest")" \
		--argjson raw_jsonl_bytes "$expected_raw_bytes" \
		'.raw_jsonl_files == $raw_jsonl_files and .raw_jsonl_bytes == $raw_jsonl_bytes' \
		"$descriptor" >/dev/null || return 1
	local archive_attestation descriptor_sha256 actual_descriptor_sha256
	archive_attestation=$(v2_r2_raw_archive_attestation_path "$cell_name")
	v2_r2_require_raw_archive_attestation_path "$cell_name" || return 1
	[[ -s "$archive_attestation" && ! -L "$archive_attestation" ]] || return 1
	descriptor_sha256=$(sha256sum -- "$descriptor" | awk '{print $1}') || return 1
	actual_descriptor_sha256=$(jq -er '.descriptor_sha256' "$archive_attestation") || return 1
	[[ "$actual_descriptor_sha256" == "$descriptor_sha256" ]] || return 1
	local attestation_archive_sha256 attestation_manifest_sha256 attestation_status_sha256
	attestation_archive_sha256=$(jq -er '.archive_sha256' "$archive_attestation") || return 1
	attestation_manifest_sha256=$(jq -er '.evidence_manifest_sha256' "$archive_attestation") || return 1
	attestation_status_sha256=$(jq -er '.run_status_sha256' "$archive_attestation") || return 1
	[[ "$attestation_archive_sha256" == "$expected_archive_sha" &&
		"$attestation_manifest_sha256" == "$expected_manifest_sha" &&
		"$attestation_status_sha256" == "$expected_status_sha" ]] || return 1
	jq -e --arg cell "$cell_name" --arg source_revision "$source_revision" \
		--arg descriptor_sha256 "$descriptor_sha256" --arg archive_sha256 "$expected_archive_sha" \
		--arg manifest_sha256 "$expected_manifest_sha" --arg status_sha256 "$expected_status_sha" \
		'.schema_version == 1 and .contract == "v2-integrated-longrun-raw-archive-attestation-v1" and
		 .cell == $cell and .source_revision == $source_revision and
		 .descriptor_sha256 == $descriptor_sha256 and .archive_sha256 == $archive_sha256 and
		 .evidence_manifest_sha256 == $manifest_sha256 and .run_status_sha256 == $status_sha256' \
		"$archive_attestation" >/dev/null || return 1
	local expected_tsv measure_command measure_status
	expected_tsv=$(mktemp) || return 1
	if ! jq -r '.raw_files[] | [.path, (.bytes | tostring), .sha256] | @tsv' \
		"$evidence_manifest" >"$expected_tsv"; then
		rm -f -- "$expected_tsv"
		return 1
	fi
	export V2_R2_ARCHIVE_EXPECTED_TSV="$expected_tsv"
	measure_command='set -eu
tab=$(printf "\t")
expected_line=$(awk -F "\t" -v path="$TAR_FILENAME" "(\$1 == path) { print \$2 \"\t\" \$3; found=1; exit } END { if (!found) exit 1 }" "$V2_R2_ARCHIVE_EXPECTED_TSV")
expected_bytes=$(printf "%s\n" "$expected_line" | cut -f1)
expected_digest=$(printf "%s\n" "$expected_line" | cut -f2)
measure_dir=$(mktemp -d)
trap "rm -rf -- \"$measure_dir\"" EXIT
mkfifo "$measure_dir/raw"
sha256sum < "$measure_dir/raw" > "$measure_dir/sha" &
sha_pid=$!
actual_bytes=$(tee "$measure_dir/raw" | wc -c)
wait "$sha_pid"
actual_digest=$(cut -d " " -f1 "$measure_dir/sha")
[ "$actual_bytes" -eq "$expected_bytes" ]
[ "$actual_digest" = "$expected_digest" ]'
	measure_status=0
	if ! tar --use-compress-program='zstd -q' --to-command="$measure_command" -xf "$archive" -C "$cell"; then
		measure_status=1
	fi
	rm -f -- "$expected_tsv"
	unset V2_R2_ARCHIVE_EXPECTED_TSV
	[[ "$measure_status" == 0 ]] || return 1
	return 0
}

v2_r2_stage_raw_evidence() {
	local cell=$1
	local marker archive
	marker="$cell/.raw-evidence-staged.$$"
	v2_r2_verify_venue_namespace "$cell" || return 1
	if find "$cell/venues" -type f -name '*.jsonl' -print -quit 2>/dev/null | grep -q .; then
		if v2_r2_verify_evidence_manifest "$cell"; then
			return 0
		fi
	fi
	archive=$(v2_r2_raw_archive_path "$cell")
	[[ -s "$archive" ]] || return 1
	v2_r2_verify_raw_evidence_archive "$cell" || return 1
	[[ ! -e "$marker" ]] || return 1
	mkdir -p -- "$cell/venues"
	touch -- "$marker"
	tar --use-compress-program='zstd -q' -xf "$archive" -C "$cell" || return 1
	local relative path expected_bytes expected_digest actual_bytes actual_digest
	while IFS=$'\t' read -r relative expected_bytes expected_digest; do
		path="$cell/$relative"
		v2_r2_validate_raw_path "$relative" || return 1
		[[ -f "$path" && ! -L "$path" ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$path") || return 1
		actual_digest=$(sha256sum -- "$path" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_digest" == "$expected_digest" ]] || return 1
	done < <(jq -r '.raw_files[] | [.path, (.bytes | tostring), .sha256] | @tsv' "$cell/evidence-manifest.json")
	local listed expected
	listed=$(find "$cell/venues" -type f -name '*.jsonl' -printf '%P\n' | sed 's#^#venues/#' | sort)
	expected=$(jq -r '.raw_files[].path' "$cell/evidence-manifest.json" | sort)
	[[ "$listed" == "$expected" ]] || return 1
}

v2_r2_cleanup_staged_raw_evidence() {
	local cell=$1
	local marker="$cell/.raw-evidence-staged.$$"
	[[ -e "$marker" ]] || return 0
	local relative path
	while IFS= read -r relative; do
		v2_r2_validate_raw_path "$relative" || return 1
		path="$cell/$relative"
		if [[ -L "$path" ]]; then
			rm -- "$path"
		elif [[ -f "$path" ]]; then
			rm -- "$path"
		elif [[ -e "$path" ]]; then
			return 1
		fi
		done < <(jq -r '.raw_files[].path' "$cell/evidence-manifest.json")
	rm -- "$marker"
}

# Raw JSONL evidence is an ordered manifest. Object-key ordering is irrelevant,
# but array order is part of the contract because it is the only binding from a
# persisted file digest to its causal path. Sorting this array would let a
# permutation of files masquerade as a parallel execution with the same set.
v2_r2_compare_ordered_raw_manifests() {
	local left=$1 right=$2
	[[ -s "$left/evidence-manifest.json" && -s "$right/evidence-manifest.json" ]] || return 1
	cmp -s <(jq -S -c '.raw_files' "$left/evidence-manifest.json") \
		<(jq -S -c '.raw_files' "$right/evidence-manifest.json")
}

v2_r2_write_attestation() {
	local cell=$1
	local cell_name
	cell_name=$(basename "$cell")
	v2_r2_require_attestation_path "$cell_name" || return 1
	mkdir -p -- "$v2_r2_attestation_root"
	local output="$v2_r2_attestation_root/$cell_name.json"
	[[ ! -e "$output" ]] || return 1
	local temporary="$output.tmp-$$"
	local manifest_sha status_sha
	manifest_sha=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}') || return 1
	status_sha=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}') || return 1
	local source_revision binary_sha256 config_sha256 prunegate_sha256
	source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json") || return 1
	binary_sha256=$(jq -er '.binary_sha256' "$cell/run-metadata.json") || return 1
	config_sha256=$(jq -er '.config_sha256' "$cell/run-metadata.json") || return 1
	prunegate_sha256=$(jq -er '.prunegate_sha256' "$cell/run-metadata.json") || return 1
	jq -n --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
		--arg config_sha256 "$config_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
		'{schema_version: 1, contract: "v2-integrated-longrun-external-attestation-v1", cell: $cell,
		 evidence_manifest_sha256: $manifest_sha, run_status_sha256: $status_sha,
		 source_revision: $source_revision, binary_sha256: $binary_sha256,
		 config_sha256: $config_sha256, prunegate_sha256: $prunegate_sha256,
		 attestation_scope: "runner-produced evidence manifest and completion status"}' >"$temporary" || return 1
	mv -- "$temporary" "$output"
}

v2_r2_verify_attestation() {
	local cell=$1
	local cell_name
	cell_name=$(basename "$cell")
	v2_r2_require_attestation_path "$cell_name" || return 1
	local attestation="$v2_r2_attestation_root/$cell_name.json"
	[[ -s "$attestation" ]] || return 1
	local manifest_sha status_sha
	manifest_sha=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}') || return 1
	status_sha=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}') || return 1
	local source_revision binary_sha256 config_sha256 prunegate_sha256
	source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json") || return 1
	binary_sha256=$(jq -er '.binary_sha256' "$cell/run-metadata.json") || return 1
	config_sha256=$(jq -er '.config_sha256' "$cell/run-metadata.json") || return 1
	prunegate_sha256=$(jq -er '.prunegate_sha256' "$cell/run-metadata.json") || return 1
	jq -e --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
		--arg config_sha256 "$config_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
		'.schema_version == 1 and .contract == "v2-integrated-longrun-external-attestation-v1" and
		 .cell == $cell and .evidence_manifest_sha256 == $manifest_sha and .run_status_sha256 == $status_sha and
		 .source_revision == $source_revision and .binary_sha256 == $binary_sha256 and
		 .config_sha256 == $config_sha256 and .prunegate_sha256 == $prunegate_sha256' \
		"$attestation" >/dev/null
}
