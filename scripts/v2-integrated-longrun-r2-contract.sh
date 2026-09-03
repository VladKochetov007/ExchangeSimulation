#!/usr/bin/env bash

# Shared fail-closed checks for the immutable R2 evidence namespace. This
# namespace is deliberately distinct from r4: r4 remains historical evidence,
# including its stale public-entry position/settlement derivations.
v2_r2_output_root="/home/vlad/v2-integrated-longrun-r2-candidate-20260830-v1"
v2_r2_attestation_root="/home/vlad/v2-integrated-longrun-r2-candidate-20260830-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-integrated-longrun-r2-candidate.lock"

v2_r2_expected_calendar_listing_timeline() {
	local calendar_epoch_nano=1735689600000000000
	local calendar_hour_nano=3600000000000
	local calendar_end_nano=$((calendar_epoch_nano + 24 * calendar_hour_nano))
	# The exchange registers the one-second expiry/listing automation ticker at
	# the calendar epoch. Its first callback therefore observes epoch-origin
	# requests at epoch+1s; later whole-hour requests coincide exactly with a
	# ticker callback. The expiry itself remains the calendar date, not the
	# observation instant.
	local listing_poll_offset_nano=1000000000
	local schedule interval lead listing_nano expiry observed_listing_nano
	local -A future_first option_first
	for schedule in "3600000000000 7200000000000" "10800000000000 21600000000000" "21600000000000 43200000000000"; do
		read -r interval lead <<<"$schedule"
		listing_nano=$calendar_epoch_nano
		while (( listing_nano <= calendar_end_nano )); do
			expiry=$((listing_nano + lead))
			observed_listing_nano=$listing_nano
			if (( observed_listing_nano == calendar_epoch_nano )); then
				observed_listing_nano=$((observed_listing_nano + listing_poll_offset_nano))
			fi
			if [[ -z ${future_first[$expiry]+set} || $observed_listing_nano -lt ${future_first[$expiry]} ]]; then
				future_first[$expiry]=$observed_listing_nano
			fi
			if [[ -z ${option_first[$expiry]+set} || $observed_listing_nano -lt ${option_first[$expiry]} ]]; then
				option_first[$expiry]=$observed_listing_nano
			fi
			listing_nano=$((listing_nano + interval))
		done
	done
	local -a expiries=()
	while IFS= read -r expiry; do
		[[ -n "$expiry" ]] && expiries+=("$expiry")
	done < <(printf '%s\n' "${!future_first[@]}" | sort -n)
	local output='[' separator=''
	for expiry in "${expiries[@]}"; do
		output+="${separator}{\"expiry_nano\":$expiry,\"future_first_listed_at_nano\":${future_first[$expiry]},\"option_first_listed_at_nano\":${option_first[$expiry]},\"future_contract_count\":1,\"option_contract_count\":10}"
		separator=,
	done
	output+=']'
	printf '%s\n' "$output"
}

# Option chains are price-gated because their strike grid is fixed at first
# listing. The registered one-second automation poll and the observed startup
# price-gating delay are retained, but a finite bound prevents an option from
# being deferred for nearly its entire contractual life while still passing a
# mere "before expiry" check.
v2_r2_calendar_option_listing_max_delay_nano() {
	printf '%s\n' 60000000000
}

v2_r2_calendar_listing_timeline_jq_definition() {
	cat <<'EOF'
def calendar_listing_timeline_matches($actual; $expected):
	if (($actual | type) != "array" or ($actual | length) != ($expected | length)) then
		false
	else
		[range(0; ($expected | length))] |
		all(.[]; . as $index |
			try (
				$actual[$index].expiry_nano == $expected[$index].expiry_nano and
				$actual[$index].future_first_listed_at_nano == $expected[$index].future_first_listed_at_nano and
				$actual[$index].future_contract_count == 1 and
				$actual[$index].option_contract_count == 10 and
					($actual[$index].option_first_listed_at_nano | type) == "number" and
					(if ($actual[$index].option_first_listed_at_nano | type) == "number" then
						$actual[$index].option_first_listed_at_nano >= $expected[$index].option_first_listed_at_nano and
						$actual[$index].option_first_listed_at_nano <= ($expected[$index].option_first_listed_at_nano + $max_option_listing_delay_nano) and
						$actual[$index].option_first_listed_at_nano < $actual[$index].expiry_nano and
					($actual[$index].option_first_listed_at_nano | tostring | test("^[0-9]+000000000$"))
				else false end)
			) catch false)
	end;
EOF
}

v2_r2_require_calendar_listing_timeline() {
	[[ $# -ge 1 && $# -le 2 ]] || return 1
	local calendar_path=$1
	local expected_timeline=${2:-$(v2_r2_expected_calendar_listing_timeline)}
	local max_option_listing_delay_nano
	max_option_listing_delay_nano=$(v2_r2_calendar_option_listing_max_delay_nano)
	local filter
	filter="$(v2_r2_calendar_listing_timeline_jq_definition)"
	filter+='type == "object" and .result.contract == "calendar-audit-v2" and
		 (.result.venues | type) == "array" and (.result.venues | length) > 0 and
		 all(.result.venues[]; calendar_listing_timeline_matches(.listing_timeline; $expected_timeline))'
	jq -e --argjson expected_timeline "$expected_timeline" \
		--argjson max_option_listing_delay_nano "$max_option_listing_delay_nano" \
		"$filter" \
		"$calendar_path" >/dev/null
}

v2_r2_expected_calendar_venue_ids() {
	printf '%s\n' '["central","north","south"]'
}

v2_r2_require_calendar_venue_set() {
	[[ $# -ge 1 && $# -le 2 ]] || return 1
	local calendar_path=$1
	local expected_venues=${2:-$(v2_r2_expected_calendar_venue_ids)}
	jq -e --argjson expected_venues "$expected_venues" \
		'type == "object" and .result.contract == "calendar-audit-v2" and
		 (.result.venues | type) == "array" and
		 (.result.venues | map(.venue_id) | sort) == ($expected_venues | sort) and
		 all(.result.venues[]; (.venue_id | type) == "string" and length > 0)' \
		"$calendar_path" >/dev/null
}

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

v2_r2_resolve_analysis_source_mode() {
	[[ $# -eq 5 ]] || return 1
	local root_dir=$1 metadata_revision=$2 head_revision=$3 analyzer_only_replay=$4 raw_source_revision=$5
	[[ "$metadata_revision" =~ ^[0-9a-f]{40}$ && "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	case "$analyzer_only_replay" in
		true|false) ;;
		*) return 1 ;;
	esac
	if [[ "$metadata_revision" == "$head_revision" ]]; then
		[[ "$analyzer_only_replay" == false && -z "$raw_source_revision" ]] || return 1
		printf '%s\n' same_revision
		return 0
	fi
	[[ "$analyzer_only_replay" == true && "$raw_source_revision" == "$metadata_revision" ]] || return 1
	git -C "$root_dir" merge-base --is-ancestor "$metadata_revision" "$head_revision" || return 1
	printf '%s\n' analyzer_only_replay
}

v2_r2_capacity_attestation_path() {
	printf '%s\n' '/home/vlad/v2-integrated-longrun-r2-binary-capacity-v1.json'
}

# Successor namespaces may bind capacity evidence to each registered
# production configuration and process width. Historical namespaces retain the
# single attestation identity above.
v2_r2_capacity_attestation_path_for_config() {
	[[ $# -eq 2 ]] || return 1
	v2_r2_capacity_attestation_path
}

v2_r2_capacity_probe_cell_for_config() {
	[[ $# -eq 2 ]] || return 1
	printf '%s\n' "${v2_r2_capacity_probe_cell:-treatment-607}"
}

v2_r2_require_binary_capacity_attestation() {
	local binary=$1 source_revision=$2 attestation=${3:-$(v2_r2_capacity_attestation_path)}
	# Successor callers may bind the measured configuration, process width, and
	# minimum-free reserve; historical callers retain the original two-argument
	# source/binary capacity contract.
	local expected_config_sha256=${4:-} expected_gomaxprocs=${5:-} expected_minimum_free_bytes=${6:-}
	local expected_launch_config_sha256=${8:-}
	local expected_memory_limit_bytes=${9:-}
	local expected_measurement_config_sha256=${10:-}
	local expected_activation_provenance_sha256=${11:-}
	local expected_activation_review_attestation_sha256=${12:-}
	local require_live_free_capacity=${7:-true}
	local expected_binary_sha available_kb required_bytes peak_bytes safety_bytes capacity_contract
	local expected_authorized_launch_config_sha256='[]'
	local expected_cpu_affinity="" expected_host_cpu_count=0 expected_allowed_cpu_count=0
	capacity_contract=${v2_r2_sv1_capacity_attestation_contract:-v2-integrated-longrun-r2-binary-capacity-v1}
	[[ -s "$attestation" && ! -L "$attestation" ]] || return 1
	expected_binary_sha=$(sha256sum -- "$binary" | awk '{print $1}') || return 1
	peak_bytes=$(jq -er '.peak_output_bytes' "$attestation") || return 1
	safety_bytes=$(jq -er '.safety_margin_bytes' "$attestation") || return 1
	required_bytes=$(jq -er '.required_free_bytes' "$attestation") || return 1
	[[ "$peak_bytes" =~ ^[0-9]+$ && "$safety_bytes" =~ ^[0-9]+$ && "$required_bytes" =~ ^[0-9]+$ ]] || return 1
	[[ "$safety_bytes" -ge $((2 * 1024 * 1024 * 1024)) ]] || return 1
	[[ "$required_bytes" == $((peak_bytes + safety_bytes)) ]] || return 1
	jq -e --arg source_revision "$source_revision" --arg binary_sha256 "$expected_binary_sha" --arg contract "$capacity_contract" \
		'.schema_version == 1 and .contract == $contract and
			 .measurement == "full_24h_binary_evidence_capacity_probe" and
			 .evidence_format == "evstream_v3" and .source_revision == $source_revision and
			 .binary_sha256 == $binary_sha256 and
				 (.peak_output_bytes | type) == "number" and (.safety_margin_bytes | type) == "number" and
				 (.required_free_bytes | type) == "number" and .required_free_bytes == (.peak_output_bytes + .safety_margin_bytes)' \
			"$attestation" >/dev/null || return 1
	if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
		jq -e --argjson capacity_seed "${v2_r2_sv1_capacity_measurement_seed:-0}" \
			'.capacity_only == true and .measurement_seed == $capacity_seed and .source_config_seed == 643 and
				 .measurement_seed != .source_config_seed' "$attestation" >/dev/null || return 1
	fi
	if [[ -n "$expected_config_sha256" || -n "$expected_launch_config_sha256" ]]; then
		[[ "$require_live_free_capacity" == true || "$require_live_free_capacity" == false ]] || return 1
		[[ "$expected_gomaxprocs" =~ ^[0-9]+$ && "$expected_minimum_free_bytes" =~ ^[0-9]+$ ]] || return 1
		if [[ -n "$expected_launch_config_sha256" ]]; then
			[[ "$expected_launch_config_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
			[[ -z "$expected_config_sha256" || "$expected_config_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
		fi
		if [[ -n "$expected_memory_limit_bytes" ]]; then
			[[ "$expected_memory_limit_bytes" =~ ^[1-9][0-9]*$ ]] || return 1
		fi
		if [[ -n "$expected_measurement_config_sha256" ]]; then
			[[ "$expected_measurement_config_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
		fi
		if [[ -n "$expected_activation_provenance_sha256" ]]; then
			[[ "$expected_activation_provenance_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
		fi
		if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
			[[ -n "$expected_activation_provenance_sha256" && "$expected_activation_provenance_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
			[[ -n "$expected_activation_review_attestation_sha256" && "$expected_activation_review_attestation_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
			local authorized_name authorized_path
			for authorized_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
				[[ "$authorized_name" != */* && "$authorized_name" == *.json ]] || return 1
				authorized_path="$v2_r2_sv1_config_dir/$authorized_name"
				[[ -s "$authorized_path" && ! -L "$authorized_path" ]] || return 1
			done
			expected_authorized_launch_config_sha256=$(for authorized_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
				sha256sum -- "$v2_r2_sv1_config_dir/$authorized_name" | awk '{print $1}'
			done | jq -Rsc 'split("\n") | map(select(length > 0))') || return 1
			IFS=$'\t' read -r expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity < <(v2_r2_sv1b_cpu_policy) || return 1
			command -v taskset >/dev/null 2>&1 || return 1
		fi
		jq -e --arg config_sha256 "$expected_config_sha256" --arg launch_config_sha256 "$expected_launch_config_sha256" \
			--arg measurement_config_sha256 "$expected_measurement_config_sha256" \
			--arg activation_provenance_sha256 "$expected_activation_provenance_sha256" \
			--arg activation_review_attestation_sha256 "$expected_activation_review_attestation_sha256" \
			--argjson expected_authorized_launch_config_sha256 "$expected_authorized_launch_config_sha256" \
			--arg expected_cpu_affinity "$expected_cpu_affinity" --argjson expected_host_cpu_count "$expected_host_cpu_count" \
			--argjson expected_allowed_cpu_count "$expected_allowed_cpu_count" --argjson expected_cpu_limit_percent "${v2_r2_sv1_cpu_limit_percent:-0}" \
			--argjson gomaxprocs "$expected_gomaxprocs" \
			--argjson minimum_free_bytes "$expected_minimum_free_bytes" --arg memory_limit_bytes "$expected_memory_limit_bytes" \
			'($config_sha256 == "" or .config_sha256 == $config_sha256) and
				($measurement_config_sha256 == "" or
					((.measurement_config_sha256 | type) == "string" and
					 (.measurement_config_sha256 | test("^[0-9a-f]{64}$")) and
					 .measurement_config_sha256 == $measurement_config_sha256)) and
					($activation_provenance_sha256 == "" or
						((.activation_provenance_sha256 | type) == "string" and
						 (.activation_provenance_sha256 | test("^[0-9a-f]{64}$")) and
						 .activation_provenance_sha256 == $activation_provenance_sha256)) and
					($activation_review_attestation_sha256 == "" or
						((.activation_review_attestation_sha256 | type) == "string" and
						 (.activation_review_attestation_sha256 | test("^[0-9a-f]{64}$")) and
						 .activation_review_attestation_sha256 == $activation_review_attestation_sha256)) and
					($launch_config_sha256 == "" or
					(.calibration_only == true and
					(.launch_config_sha256 | type) == "string" and
					(.primary_launch_config_sha256 | type) == "string" and
					.primary_launch_config_sha256 == .launch_config_sha256 and
						(.authorized_launch_config_sha256 | type) == "array" and
						all(.authorized_launch_config_sha256[]; type == "string" and test("^[0-9a-f]{64}$")) and
						(($expected_authorized_launch_config_sha256 | length) == 0 or
							.authorized_launch_config_sha256 == $expected_authorized_launch_config_sha256) and
						(.authorized_launch_config_sha256 | index($launch_config_sha256)) != null and
						(.measurement_config_path | type) == "string" and (.launch_config_path | type) == "string")) and
				 (($expected_cpu_affinity == "") or
					(.host_cpu_count == $expected_host_cpu_count and .allowed_cpu_count == $expected_allowed_cpu_count and
					 .cpu_limit_percent == $expected_cpu_limit_percent and .cpu_affinity == $expected_cpu_affinity)) and
				 .gomaxprocs == $gomaxprocs and .minimum_free_bytes == $minimum_free_bytes and
				(if $memory_limit_bytes == "" then true else
					((.memory_limit_bytes | type) == "number" and .memory_limit_bytes == ($memory_limit_bytes | tonumber) and
					 (.gomemlimit_bytes | type) == "number" and .gomemlimit_bytes == (.memory_limit_bytes - 2147483648) and
					 .gomemlimit_bytes > 0 and
					 (.peak_rss_bytes | type) == "number" and .peak_rss_bytes > 0 and .peak_rss_bytes <= .memory_limit_bytes)
			 end) and
			 (.initial_available_free_bytes | type) == "number" and .initial_available_free_bytes >= $minimum_free_bytes and
			 (.available_free_bytes | type) == "number" and .available_free_bytes >= $minimum_free_bytes and
			 .available_free_bytes >= (.peak_output_bytes + .safety_margin_bytes) and
			 (.evidence_manifest_sha256 | type) == "string" and (.evidence_manifest_sha256 | test("^[0-9a-f]{64}$"))' \
			"$attestation" >/dev/null || return 1
			local probe_root probe_cell probe_cell_name actual_manifest_sha256 measurement_config_path measured_log_mode
			local launch_config_path measurement_config_abs launch_config_abs actual_measurement_config_sha256 actual_launch_config_sha256
			local measured_seed measured_start_nano measured_end_nano measured_config_sha256
		probe_root=$(jq -er '.probe_root | select(type == "string")' "$attestation") || return 1
		[[ "$probe_root" == /* && "$probe_root" != */ && "$probe_root" != *$'\n'* && "$probe_root" != *$'\t'* ]] || return 1
		[[ -d "$probe_root" && ! -L "$probe_root" ]] || return 1
		[[ "$(realpath -e -- "$probe_root")" == "$probe_root" ]] || return 1
		measurement_config_path=$(jq -er '.measurement_config_path | select(type == "string" and length > 0)' "$attestation") || return 1
		probe_cell_name=$(v2_r2_capacity_probe_cell_for_config "$measurement_config_path" "$expected_gomaxprocs") || return 1
		[[ "$probe_cell_name" != /* && "$probe_cell_name" != */* && "$probe_cell_name" != *$'\n'* && "$probe_cell_name" != *$'\t'* ]] || return 1
		[[ "$(jq -er '.probe_cell | select(type == "string")' "$attestation")" == "$probe_cell_name" ]] || return 1
		probe_cell="$probe_root/$probe_cell_name"
		[[ -d "$probe_cell" && ! -L "$probe_cell" ]] || return 1
		[[ "$(realpath -e -- "$probe_cell")" == "$probe_cell" ]] || return 1
		if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
			[[ "$measurement_config_path" != /* && "$measurement_config_path" != */ && "$measurement_config_path" != *$'\n'* && "$measurement_config_path" != *$'\t'* ]] || return 1
			measurement_config_abs="$root_dir/$measurement_config_path"
			[[ -f "$measurement_config_abs" && ! -L "$measurement_config_abs" ]] || return 1
			[[ "$(realpath -e -- "$measurement_config_abs")" == "$measurement_config_abs" ]] || return 1
			actual_measurement_config_sha256=$(sha256sum -- "$measurement_config_abs" | awk '{print $1}') || return 1
			[[ "$actual_measurement_config_sha256" == "$(jq -er '.measurement_config_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$attestation")" ]] || return 1
			launch_config_path=$(jq -er '.launch_config_path | select(type == "string" and length > 0)' "$attestation") || return 1
			[[ "$launch_config_path" != /* && "$launch_config_path" != */ && "$launch_config_path" != *$'\n'* && "$launch_config_path" != *$'\t'* ]] || return 1
			v2_r2_capacity_attestation_path_for_config "$launch_config_path" "$expected_gomaxprocs" >/dev/null || return 1
			launch_config_abs="$root_dir/$launch_config_path"
			[[ -f "$launch_config_abs" && ! -L "$launch_config_abs" ]] || return 1
			[[ "$(realpath -e -- "$launch_config_abs")" == "$launch_config_abs" ]] || return 1
			actual_launch_config_sha256=$(sha256sum -- "$launch_config_abs" | awk '{print $1}') || return 1
			[[ "$actual_launch_config_sha256" == "$(jq -er '.launch_config_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$attestation")" ]] || return 1
			[[ "$(jq -er '.primary_launch_config_sha256' "$attestation")" == "$actual_launch_config_sha256" ]] || return 1
			local activation_provenance_path activation_review_attestation_path actual_activation_provenance_sha256 actual_activation_review_sha256
			activation_provenance_path=$(jq -er '.activation_provenance_path | select(type == "string")' "$attestation") || return 1
			[[ "$activation_provenance_path" == /* && "$activation_provenance_path" != */ && "$activation_provenance_path" != *$'\n'* && "$activation_provenance_path" != *$'\t'* ]] || return 1
			[[ -f "$activation_provenance_path" && ! -L "$activation_provenance_path" && "$(realpath -e -- "$activation_provenance_path")" == "$activation_provenance_path" ]] || return 1
			actual_activation_provenance_sha256=$(sha256sum -- "$activation_provenance_path" | awk '{print $1}') || return 1
			[[ "$actual_activation_provenance_sha256" == "$(jq -er '.activation_provenance_sha256' "$attestation")" ]] || return 1
			[[ "$actual_activation_provenance_sha256" == "$expected_activation_provenance_sha256" ]] || return 1
			v2_r2_require_sv1b_activation_provenance "$activation_provenance_path" "$source_revision" "$expected_binary_sha" || return 1
			activation_review_attestation_path=$(jq -er '.activation_review_attestation_path | select(type == "string")' "$attestation") || return 1
			[[ "$activation_review_attestation_path" == /* && "$activation_review_attestation_path" != */ && "$activation_review_attestation_path" != *$'\n'* && "$activation_review_attestation_path" != *$'\t'* ]] || return 1
			[[ -f "$activation_review_attestation_path" && ! -L "$activation_review_attestation_path" && "$(realpath -e -- "$activation_review_attestation_path")" == "$activation_review_attestation_path" ]] || return 1
			actual_activation_review_sha256=$(sha256sum -- "$activation_review_attestation_path" | awk '{print $1}') || return 1
			[[ "$actual_activation_review_sha256" == "$(jq -er '.activation_review_attestation_sha256' "$attestation")" ]] || return 1
			[[ "$actual_activation_review_sha256" == "$expected_activation_review_attestation_sha256" ]] || return 1
			v2_r2_require_sv1b_activation_review_attestation "$activation_review_attestation_path" "$source_revision" "$activation_provenance_path" || return 1
		fi
		measured_log_mode=$(jq -er '.log_mode | select(. == "full" or . == "none")' "$probe_cell/run-config.json") || return 1
		local retained_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl \
			events.evs binary-evidence-attestation.json evidence-manifest.json)
		if [[ "$measured_log_mode" == full ]]; then
			retained_files+=(evidence-only-artifact-hash.json)
		else
			[[ ! -e "$probe_cell/evidence-only-artifact-hash.json" && ! -L "$probe_cell/evidence-only-artifact-hash.json" ]] || return 1
		fi
		if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
			retained_files+=(terminal-outcome.json)
		fi
		for retained in "${retained_files[@]}"; do
			[[ -s "$probe_cell/$retained" && ! -L "$probe_cell/$retained" ]] || return 1
		done
		if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
			measured_seed=$(jq -er '.measurement_seed | select(type == "number")' "$attestation") || return 1
			measured_start_nano=$(jq -er '.simulation_start_nano | select(type == "number")' "$attestation") || return 1
			measured_end_nano=$(jq -er '.simulation_end_nano | select(type == "number")' "$attestation") || return 1
			measured_config_sha256=$(sha256sum -- "$probe_cell/run-config.json" | awk '{print $1}') || return 1
			[[ "$measured_config_sha256" == "$(jq -er '.config_sha256' "$attestation")" ]] || return 1
			jq -e --argjson seed "$measured_seed" --arg log_mode "$measured_log_mode" \
				'.seed == $seed and .log_mode == $log_mode and .evidence_format == "evstream_v3"' \
				"$probe_cell/run-config.json" >/dev/null || return 1
			jq -e --arg revision "$source_revision" --argjson seed "$measured_seed" --arg log_mode "$measured_log_mode" \
				'.schema_version >= 2 and .build.revision == $revision and .build.modified == false and
				 .config.seed == $seed and .config.log_mode == $log_mode and .config.evidence_format == "evstream_v3"' \
				"$probe_cell/manifest.json" >/dev/null || return 1
			jq -e --argjson start "$measured_start_nano" --argjson end "$measured_end_nano" \
				'.schema_version == 7 and .report_status == "complete_terminal_valuation" and
				 .terminal_valuation_available == true and
				 (.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $start)) and
				 (.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $end))' \
				"$probe_cell/greeks.json" >/dev/null || return 1
			if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
				jq -e --argjson start "$measured_start_nano" --argjson end "$measured_end_nano" \
					-f "$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq" "$probe_cell/terminal-outcome.json" >/dev/null || return 1
				[[ "$(jq -er '.status' "$probe_cell/terminal-outcome.json")" == completed ]] || return 1
			fi
			v2_r2_require_checkpoint_stream "$probe_cell/checkpoints.jsonl" "$measured_start_nano" "$measured_end_nano" || return 1
		fi
		actual_manifest_sha256=$(sha256sum -- "$probe_cell/evidence-manifest.json" | awk '{print $1}') || return 1
		[[ "$actual_manifest_sha256" == "$(jq -er '.evidence_manifest_sha256' "$attestation")" ]] || return 1
		v2_r2_verify_evidence_manifest "$probe_cell" || return 1
	fi
	if [[ "$require_live_free_capacity" == true ]]; then
		available_kb=$(df -Pk "$(dirname -- "$attestation")" | awk 'NR == 2 {print $4}') || return 1
		[[ "$available_kb" =~ ^[0-9]+$ ]] || return 1
		[[ $((available_kb * 1024)) -ge "$required_bytes" ]]
	fi
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

# Checkpoint streams are strictly increasing except for the explicit final
# attestation, which may repeat the terminal ordinary checkpoint when an event
# lands exactly on a checkpoint boundary. The repeated row must be identical
# in state and must be marked final; this keeps the terminal attestation
# explicit without allowing an arbitrary duplicate in the ordered stream.
v2_r2_require_checkpoint_stream() {
	[[ $# -eq 3 ]] || return 1
	local checkpoints=$1
	local simulation_start_nano=$2
	local simulation_end_nano=$3
	jq -e -s --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		'. as $checkpoints |
			 ($checkpoints | length) >= 2 and
			 all($checkpoints[]; .domain == "execution_observations" and .ordering == "ordered_stream" and
			(.sim_time | type) == "number" and (.event_count | type) == "number" and
			(.execution_stream_hash | type) == "string" and (.execution_stream_hash | test("^[0-9a-f]{64}$")) and
			.sim_time >= $simulation_start_nano and .sim_time <= $simulation_end_nano and .event_count >= 0) and
			 ($checkpoints | map(select(.final == true)) | length) == 1 and
			 $checkpoints[-1].final == true and
			 all(range(1; (($checkpoints | length) - 1));
				 ($checkpoints[. - 1].final != true and $checkpoints[.].final != true and
				  $checkpoints[. - 1].sim_time < $checkpoints[.].sim_time and
				  $checkpoints[. - 1].event_count < $checkpoints[.].event_count)) and
			 $checkpoints[-2].final != true and
			 $checkpoints[-2].sim_time == $checkpoints[-1].sim_time and
			 $checkpoints[-2].event_count == $checkpoints[-1].event_count and
			 ($checkpoints[-2] | del(.final)) == ($checkpoints[-1] | del(.final)) and
			 $checkpoints[-1].sim_time == $simulation_end_nano and $checkpoints[-1].final == true' \
		"$checkpoints" >/dev/null
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

v2_r2_evidence_format() {
	local cell=$1
	jq -er '.evidence_format // "jsonl"' "$cell/run-config.json"
}

# A typed economic endpoint is accepted as a terminal-failure measurement only
# when its provenance says that the price error arose at the terminal capture
# fixed point. The runner and verifier both call this predicate so a generic or
# scheduled price error cannot enter the diagnostic-negative evidence profile.
v2_r2_terminal_failure_outcome_present() {
	local cell=$1 start end
	[[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]] || return 1
	[[ -s "$cell/run-metadata.json" && -s "$cell/terminal-outcome.json" ]] || return 1
	start=$(jq -er '.simulation_start_nano' "$cell/run-metadata.json") || return 1
	end=$(jq -er '.simulation_end_nano' "$cell/run-metadata.json") || return 1
	config_strict=$(jq -er '.strict_population_accounting' "$cell/run-config.json") || return 1
	jq -e --argjson start "$start" --argjson end "$end" --argjson strict "$config_strict" '
		type == "object" and .schema_version == 2 and .status == "terminal_failure" and
		(.code == "PRICE_UNAVAILABLE" or .code == "PRICE_DOMAIN_ERROR") and
		.phase == "terminal_post_mark" and .simulation_start_nano == $start and
		.simulation_end_nano == $end and .evidence_format == "evstream_v3" and
		(.strict_population_accounting | type) == "boolean" and .strict_population_accounting == $strict and
		(.evidence_sealed | type) == "boolean" and (.terminal_risk_captured | type) == "boolean" and
		(.terminal_population_captured | type) == "boolean" and
		.evidence_sealed == true and .terminal_risk_captured == false and
		.terminal_population_captured == false and
		(.stage == "terminal_risk_capture" or .stage == "terminal_population_capture") and
		(.failure_at_nano | type) == "number" and .failure_at_nano == $end and
		(.failure_venue_id | type) == "string" and (.failure_venue_id | length) > 0 and
		(.failure_symbol | type) == "string" and (.failure_symbol | length) > 0 and
		(.error | type) == "string" and (.error | length) > 0 and
		((.evidence_seal_error // "") == "")' "$cell/terminal-outcome.json" >/dev/null
}

v2_r2_terminal_completed_outcome_present() {
	local cell=$1 start end
	[[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]] || return 1
	[[ -s "$cell/run-metadata.json" && -s "$cell/terminal-outcome.json" ]] || return 1
	start=$(jq -er '.simulation_start_nano' "$cell/run-metadata.json") || return 1
	end=$(jq -er '.simulation_end_nano' "$cell/run-metadata.json") || return 1
	config_strict=$(jq -er '.strict_population_accounting' "$cell/run-config.json") || return 1
	jq -e --argjson start "$start" --argjson end "$end" --argjson strict "$config_strict" '
		type == "object" and .schema_version == 2 and .status == "completed" and
		.code == "COMPLETED" and .phase == "terminal_post_mark" and
		.simulation_start_nano == $start and .simulation_end_nano == $end and
		.evidence_format == "evstream_v3" and .evidence_sealed == true and
		(.strict_population_accounting | type) == "boolean" and .strict_population_accounting == $strict and
		.terminal_risk_captured == true and
		(.strict_population_accounting == false or .terminal_population_captured == true) and
		(has("error") | not) and (has("evidence_seal_error") | not) and
		(has("stage") | not) and (has("failure_at_nano") | not) and
		(has("failure_venue_id") | not) and (has("failure_symbol") | not)' "$cell/terminal-outcome.json" >/dev/null
}

v2_r2_write_evidence_manifest() {
	local cell=$1
	local output="$cell/evidence-manifest.json"
	local log_mode evidence_format contract record_market_data_receipts
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json") || return 1
	evidence_format=$(v2_r2_evidence_format "$cell") || return 1
	record_market_data_receipts=$(jq -r '.record_market_data_receipts // false' "$cell/run-config.json") || return 1
	local fixed_files terminal_required=false terminal_failure=false
	if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
		terminal_required=true
		if v2_r2_terminal_failure_outcome_present "$cell"; then
			terminal_failure=true
		elif ! v2_r2_terminal_completed_outcome_present "$cell"; then
			return 1
		fi
	fi
	case "$evidence_format:$log_mode" in
		jsonl:full)
			contract="v2-integrated-longrun-evidence-manifest-v1"
			fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-artifact-hash.json)
			;;
		jsonl:none)
			contract="v2-integrated-longrun-evidence-manifest-v1"
			fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl)
			;;
		evstream_v3:full)
			contract="v2-integrated-longrun-evidence-manifest-v2"
			fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json)
			;;
		evstream_v3:none)
			contract="v2-integrated-longrun-evidence-manifest-v2"
			fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json)
			;;
		*) return 1 ;;
	 esac
	if [[ "$terminal_required" == true ]]; then
		case "$evidence_format:$log_mode" in
			evstream_v3:full)
				contract="v2-integrated-longrun-evidence-manifest-v3"
				if [[ "$terminal_failure" == true ]]; then
					fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json terminal-outcome.json)
				else
					fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json terminal-outcome.json)
				fi
				;;
			evstream_v3:none)
				contract="v2-integrated-longrun-evidence-manifest-v3"
				if [[ "$terminal_failure" == true ]]; then
					fixed_files=(run-config.json run-metadata.json manifest.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json terminal-outcome.json)
				else
					fixed_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json terminal-outcome.json)
				fi
				;;
			*) return 1 ;;
		 esac
	fi
	if [[ "$evidence_format" == evstream_v3 && "$record_market_data_receipts" == true ]]; then
		local receipt_file
		for receipt_file in market-data-evidence-v2.json market-data-schedules-v2.bin market-data-receipts-v2.bin market-data-decisions-v2.bin market-data-actions-v2.bin; do
			if [[ "$receipt_file" == *.bin ]]; then
				[[ -f "$cell/$receipt_file" && ! -L "$cell/$receipt_file" ]] || return 1
			else
				[[ -s "$cell/$receipt_file" && ! -L "$cell/$receipt_file" ]] || return 1
			fi
			fixed_files+=("$receipt_file")
		done
	fi
	local fixed_records='[]'
	local relative path bytes digest
	for relative in "${fixed_files[@]}"; do
		path="$cell/$relative"
		if [[ "$relative" == market-data-*.bin ]]; then
			[[ -f "$path" && ! -L "$path" ]] || return 1
		else
			[[ -s "$path" && ! -L "$path" ]] || return 1
		fi
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
		--arg contract "$contract" \
		--arg cell "$(basename "$cell")" \
		--arg log_mode "$log_mode" \
		--arg evidence_format "$evidence_format" \
		--arg source_revision "$(jq -er '.git_revision' "$cell/run-metadata.json")" \
		--argjson fixed_files "$fixed_records" --argjson raw_files "$raw_records" \
		--argjson raw_count "$raw_count" --argjson raw_bytes "$raw_bytes" \
		'{schema_version: (if $contract == "v2-integrated-longrun-evidence-manifest-v3" then 3 elif $evidence_format == "evstream_v3" then 2 else 1 end), contract: $contract, cell: $cell, log_mode: $log_mode, evidence_format: $evidence_format, source_revision: $source_revision,
			fixed_files: $fixed_files, raw_jsonl_files: $raw_count, raw_jsonl_bytes: $raw_bytes,
		 raw_files: $raw_files}' >"$temporary" || return 1
	mv -- "$temporary" "$output"
}

v2_r2_verify_evidence_manifest() {
	local cell=$1
	local manifest="$cell/evidence-manifest.json"
	[[ -s "$manifest" ]] || return 1
	local log_mode evidence_format expected_contract expected_schema record_market_data_receipts
	log_mode=$(jq -er '.log_mode' "$cell/run-config.json") || return 1
	evidence_format=$(v2_r2_evidence_format "$cell") || return 1
	record_market_data_receipts=$(jq -r '.record_market_data_receipts // false' "$cell/run-config.json") || return 1
	local terminal_required=false terminal_failure=false
	if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
		terminal_required=true
		if v2_r2_terminal_failure_outcome_present "$cell"; then
			terminal_failure=true
		elif ! v2_r2_terminal_completed_outcome_present "$cell"; then
			return 1
		fi
	fi
	case "$evidence_format" in
		jsonl) expected_contract="v2-integrated-longrun-evidence-manifest-v1"; expected_schema=1 ;;
		evstream_v3)
			if [[ "$terminal_required" == true ]]; then
				expected_contract="v2-integrated-longrun-evidence-manifest-v3"; expected_schema=3
			else
				expected_contract="v2-integrated-longrun-evidence-manifest-v2"; expected_schema=2
			fi
			;;
		*) return 1 ;;
	esac
	local expected_fixed
	case "$evidence_format:$log_mode" in
		jsonl:full) expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-artifact-hash.json | sort) ;;
		jsonl:none) expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl | sort) ;;
		evstream_v3:full)
			if [[ "$terminal_required" == true ]]; then
				expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json terminal-outcome.json | sort)
			else
				expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json | sort)
			fi
			;;
		evstream_v3:none)
			if [[ "$terminal_required" == true ]]; then
				if [[ "$terminal_failure" == true ]]; then
					expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json terminal-outcome.json | sort)
				else
					expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json terminal-outcome.json | sort)
				fi
			else
				expected_fixed=$(printf '%s\n' run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json | sort)
			fi
			;;
		*) return 1 ;;
	esac
	if [[ "$evidence_format" == evstream_v3 && "$record_market_data_receipts" == true ]]; then
		expected_fixed=$(printf '%s\n' "$expected_fixed" market-data-evidence-v2.json market-data-schedules-v2.bin market-data-receipts-v2.bin market-data-decisions-v2.bin market-data-actions-v2.bin | sort)
	fi
	jq -e --arg cell "$(basename "$cell")" --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
		--arg expected_contract "$expected_contract" --argjson expected_schema "$expected_schema" \
		'.schema_version == $expected_schema and .contract == $expected_contract and .cell == $cell and
			.log_mode == $log_mode and .evidence_format == $evidence_format and
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
	local source_revision binary_sha256 config_sha256 prunegate_sha256 evidence_format attestation_contract terminal_outcome_sha256=""
	source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json") || return 1
	evidence_format=$(v2_r2_evidence_format "$cell") || return 1
	if [[ "$evidence_format" == "evstream_v3" ]]; then
		if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
			attestation_contract="v2-integrated-longrun-external-attestation-v3"
		else
			attestation_contract="v2-integrated-longrun-external-attestation-v2"
		fi
	else
		attestation_contract="v2-integrated-longrun-external-attestation-v1"
	fi
	binary_sha256=$(jq -er '.binary_sha256' "$cell/run-metadata.json") || return 1
	config_sha256=$(jq -er '.config_sha256' "$cell/run-metadata.json") || return 1
	prunegate_sha256=$(jq -er '.prunegate_sha256' "$cell/run-metadata.json") || return 1
	if [[ "$attestation_contract" == "v2-integrated-longrun-external-attestation-v3" ]]; then
		terminal_outcome_sha256=$(sha256sum -- "$cell/terminal-outcome.json" | awk '{print $1}') || return 1
	fi
	jq -n --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
		--arg config_sha256 "$config_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
		--arg evidence_format "$evidence_format" --arg attestation_contract "$attestation_contract" \
		--arg terminal_outcome_sha256 "$terminal_outcome_sha256" \
		'{schema_version: (if $attestation_contract == "v2-integrated-longrun-external-attestation-v3" then 3 elif $evidence_format == "evstream_v3" then 2 else 1 end), contract: $attestation_contract, cell: $cell,
		 evidence_manifest_sha256: $manifest_sha, run_status_sha256: $status_sha,
		 source_revision: $source_revision, binary_sha256: $binary_sha256,
		 config_sha256: $config_sha256, prunegate_sha256: $prunegate_sha256,
		 evidence_format: $evidence_format,
		 attestation_scope: "runner-produced evidence manifest and completion status"} |
		 (if $terminal_outcome_sha256 == "" then . else . + {terminal_outcome_sha256: $terminal_outcome_sha256} end)' >"$temporary" || return 1
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
	local source_revision binary_sha256 config_sha256 prunegate_sha256 evidence_format expected_contract expected_schema terminal_outcome_sha256=""
	source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json") || return 1
	evidence_format=$(v2_r2_evidence_format "$cell") || return 1
	if [[ "$evidence_format" == "evstream_v3" ]]; then
		if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
			expected_contract="v2-integrated-longrun-external-attestation-v3"
			expected_schema=3
		else
			expected_contract="v2-integrated-longrun-external-attestation-v2"
			expected_schema=2
		fi
	else
		expected_contract="v2-integrated-longrun-external-attestation-v1"
		expected_schema=1
	fi
	binary_sha256=$(jq -er '.binary_sha256' "$cell/run-metadata.json") || return 1
	config_sha256=$(jq -er '.config_sha256' "$cell/run-metadata.json") || return 1
	prunegate_sha256=$(jq -er '.prunegate_sha256' "$cell/run-metadata.json") || return 1
	if [[ "$expected_contract" == "v2-integrated-longrun-external-attestation-v3" ]]; then
		terminal_outcome_sha256=$(sha256sum -- "$cell/terminal-outcome.json" | awk '{print $1}') || return 1
	fi
	jq -e --arg cell "$cell_name" --arg manifest_sha "$manifest_sha" --arg status_sha "$status_sha" \
		--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
		--arg config_sha256 "$config_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
		--arg evidence_format "$evidence_format" --arg expected_contract "$expected_contract" --argjson expected_schema "$expected_schema" \
		--arg terminal_outcome_sha256 "$terminal_outcome_sha256" \
		'.schema_version == $expected_schema and .contract == $expected_contract and .evidence_format == $evidence_format and
		 .cell == $cell and .evidence_manifest_sha256 == $manifest_sha and .run_status_sha256 == $status_sha and
		 .source_revision == $source_revision and .binary_sha256 == $binary_sha256 and
		 .config_sha256 == $config_sha256 and .prunegate_sha256 == $prunegate_sha256 and
		 (if $expected_contract == "v2-integrated-longrun-external-attestation-v3" then .terminal_outcome_sha256 == $terminal_outcome_sha256 else (has("terminal_outcome_sha256") | not) end)' \
		"$attestation" >/dev/null
}
