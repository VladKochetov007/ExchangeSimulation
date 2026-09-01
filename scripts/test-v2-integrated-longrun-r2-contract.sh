#!/usr/bin/env bash
# Cheap control-plane tests for the R2 successor. These tests never launch a
# simulation and never inspect a reserved holdout artifact.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
sv1_lock_path=/home/vlad/v2-r2-sv1-24h-development.lock
sv1_lock_created=false
sv1_lock_fd=""
cleanup_sv1_lock() {
	if [[ "$sv1_lock_created" == true ]]; then
		flock -u "$sv1_lock_fd" 2>/dev/null || true
		unlink -- "$sv1_lock_path" 2>/dev/null || true
	fi
}
trap 'cleanup_sv1_lock; rm -rf -- "$tmp_root"' EXIT

fail() {
	printf 'integrated long-run R2 contract test failure: %s\n' "$*" >&2
	exit 1
}

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		fail "command unexpectedly succeeded: $*"
	fi
}

"$root_dir/scripts/check-v2-integrated-longrun-r2-configs.sh" >/dev/null
source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

for sv1_script in \
	check-v2-r2-sv1-24h-parity.sh \
	extract-v2-r2-sv1-24h-cell.sh \
	score-v2-r2-sv1-24h-development.sh \
	archive-v2-r2-sv1-capacity-probe.sh \
	verify-v2-r2-sv1-24h-cell.sh; do
	[[ -x "$root_dir/scripts/$sv1_script" ]] || fail "SV1 script is not executable: $sv1_script"
	bash -n "$root_dir/scripts/$sv1_script" || fail "SV1 script has invalid shell syntax: $sv1_script"
done
[[ -f "$root_dir/scripts/v2-r2-evidence-input-contract.sh" ]] || fail "binary evidence input contract is missing"
bash -n "$root_dir/scripts/v2-r2-evidence-input-contract.sh" || fail "binary evidence input contract has invalid shell syntax"
source "$root_dir/scripts/v2-r2-evidence-input-contract.sh"
binary_input_fixture="$tmp_root/events.evs"
json_input_fixture="$tmp_root/sidecar.json"
printf '\000binary-event-stream\n' >"$binary_input_fixture"
printf '{}\n' >"$json_input_fixture"
v2_r2_require_evidence_input_file binary "$binary_input_fixture" || fail "binary evidence input was rejected"
v2_r2_require_evidence_input_file json "$json_input_fixture" || fail "JSON sidecar input was rejected"
expect_failure v2_r2_require_evidence_input_file json "$binary_input_fixture"
ln -s -- "$binary_input_fixture" "$tmp_root/events-symlink.evs"
expect_failure v2_r2_require_evidence_input_file binary "$tmp_root/events-symlink.evs"
expect_failure v2_r2_require_evidence_input_file binary "$tmp_root/missing.evs"
expect_failure "$root_dir/scripts/archive-v2-r2-sv1-capacity-probe.sh" \
	"$tmp_root/missing-capacity-attestation.json" /bin/true "$tmp_root/capacity-archive.tar.zst"
for required_capacity_fragment in \
	'v2-r2-sv1-capacity-archive-retention-v1' \
	'refusing to compact a capacity probe for the current source revision' \
	'git -C "$root_dir" merge-base --is-ancestor' \
	'registered_config=' \
	'new_temporary_file' \
	'publish_new_file' \
	'ln --no-target-directory' \
	'require_published_sidecar' \
	'findmnt -n -l -r -o TARGET' \
	'find -P "$probe_root" -xdev' \
	'deletion_root='; do
	rg -F "$required_capacity_fragment" "$root_dir/scripts/archive-v2-r2-sv1-capacity-probe.sh" >/dev/null ||
		fail "capacity archive script is missing required fragment: $required_capacity_fragment"
done
for required_sv1_fragment in \
	'V2_R2_EXTRACTOR_VARIANT=sv1' \
	'control_full_no_log_normalized_equal' \
	'execution_stream_hash' \
	'v2_r2_require_evidence_input_file' \
	'RESERVED_AND_NOT_READ_BY_DEVELOPMENT_SCORER' \
	'holdout-619'; do
	rg -F "$required_sv1_fragment" \
		"$root_dir/scripts/check-v2-r2-sv1-24h-parity.sh" \
		"$root_dir/scripts/score-v2-r2-sv1-24h-development.sh" \
		"$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" \
		"$root_dir/scripts/verify-v2-integrated-longrun-r2-cell.sh" >/dev/null ||
		fail "SV1 control-plane scripts are missing required fragment: $required_sv1_fragment"
done

left_manifest_dir="$tmp_root/ordered-left"
right_manifest_dir="$tmp_root/ordered-right"
mkdir -p "$left_manifest_dir" "$right_manifest_dir"
printf '%s\n' '{"raw_files":[{"path":"venues/a.jsonl","bytes":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"path":"venues/b.jsonl","bytes":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}' >"$left_manifest_dir/evidence-manifest.json"
cp "$left_manifest_dir/evidence-manifest.json" "$right_manifest_dir/evidence-manifest.json"
v2_r2_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir" || fail "identical ordered manifests did not compare equal"
jq '.raw_files |= reverse' "$right_manifest_dir/evidence-manifest.json" >"$right_manifest_dir/permuted.json"
mv "$right_manifest_dir/permuted.json" "$right_manifest_dir/evidence-manifest.json"
expect_failure v2_r2_compare_ordered_raw_manifests "$left_manifest_dir" "$right_manifest_dir"

matching_revision=0123456789abcdef0123456789abcdef01234567
v2_r2_require_matching_revision "$matching_revision" "$matching_revision" ||
	fail "matching revision was rejected"
expect_failure v2_r2_require_matching_revision "$matching_revision" \
	"fedcba9876543210fedcba9876543210fedcba98"
v2_r2_require_current_source_revision "$matching_revision" "$matching_revision" "$matching_revision" ||
	fail "matching current source revision was rejected"
expect_failure v2_r2_require_current_source_revision "$matching_revision" \
	"fedcba9876543210fedcba9876543210fedcba98" "$matching_revision"
expect_failure v2_r2_require_current_source_revision "$matching_revision" "$matching_revision" \
	"fedcba9876543210fedcba9876543210fedcba98"

checkpoint_fixture="$tmp_root/exact-terminal-checkpoints.jsonl"
checkpoint_hash_a=$(printf 'a%.0s' {1..64})
checkpoint_hash_b=$(printf 'b%.0s' {1..64})
printf '%s\n' \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":5,\"event_count\":1,\"execution_stream_hash\":\"$checkpoint_hash_a\"}" \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":10,\"event_count\":2,\"execution_stream_hash\":\"$checkpoint_hash_b\"}" \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":10,\"event_count\":2,\"execution_stream_hash\":\"$checkpoint_hash_b\",\"final\":true}" \
	>"$checkpoint_fixture"
v2_r2_require_checkpoint_stream "$checkpoint_fixture" 0 10 || fail "exact-terminal final checkpoint was rejected"
jq '.final = true | .execution_stream_hash = "tampered"' "$checkpoint_fixture" >"$tmp_root/tampered-checkpoints.jsonl"
expect_failure v2_r2_require_checkpoint_stream "$tmp_root/tampered-checkpoints.jsonl" 0 10
jq '.execution_stream_hash = 1' "$checkpoint_fixture" >"$tmp_root/non-string-hash-checkpoints.jsonl"
expect_failure v2_r2_require_checkpoint_stream "$tmp_root/non-string-hash-checkpoints.jsonl" 0 10
printf '%s\n' \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":5,\"event_count\":1,\"execution_stream_hash\":\"$checkpoint_hash_a\"}" \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":10,\"event_count\":2,\"execution_stream_hash\":\"$checkpoint_hash_b\"}" \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":10,\"event_count\":3,\"execution_stream_hash\":\"$checkpoint_hash_b\"}" \
	"{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":10,\"event_count\":3,\"execution_stream_hash\":\"$checkpoint_hash_b\",\"final\":true}" \
	>"$tmp_root/repeated-terminal-checkpoints.jsonl"
expect_failure v2_r2_require_checkpoint_stream "$tmp_root/repeated-terminal-checkpoints.jsonl" 0 10
printf '%s\n' \
	'{"domain":"execution_observations","ordering":"ordered_stream","sim_time":5,"event_count":1,"execution_stream_hash":"aaa"}' \
	'{"domain":"execution_observations","ordering":"ordered_stream","sim_time":5,"event_count":1,"execution_stream_hash":"aaa","final":true}' \
	'{"domain":"execution_observations","ordering":"ordered_stream","sim_time":10,"event_count":2,"execution_stream_hash":"bbb"}' \
	'{"domain":"execution_observations","ordering":"ordered_stream","sim_time":10,"event_count":2,"execution_stream_hash":"bbb","final":true}' \
	>"$tmp_root/intermediate-final-checkpoints.jsonl"
expect_failure v2_r2_require_checkpoint_stream "$tmp_root/intermediate-final-checkpoints.jsonl" 0 10

capacity_probe_binary="$tmp_root/capacity-probe"
cp -- /bin/true "$capacity_probe_binary"
capacity_probe_attestation="$tmp_root/binary-capacity.json"
capacity_probe_sha256=$(sha256sum -- "$capacity_probe_binary" | awk '{print $1}')
jq -n --arg source_revision "$matching_revision" --arg binary_sha256 "$capacity_probe_sha256" \
	'{schema_version: 1, contract: "v2-integrated-longrun-r2-binary-capacity-v1",
	 measurement: "full_24h_binary_evidence_capacity_probe", evidence_format: "evstream_v3",
	 source_revision: $source_revision, binary_sha256: $binary_sha256,
	 config_sha256: "config-for-contract-test", gomaxprocs: 4, minimum_free_bytes: 4294967296,
	 peak_output_bytes: 1, safety_margin_bytes: 2147483648,
	 required_free_bytes: 2147483649}' >"$capacity_probe_attestation"
v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" "$capacity_probe_attestation" ||
	fail "a valid binary capacity attestation was rejected"

capacity_retained_cell="$tmp_root/retained-capacity/treatment-607"
mkdir -p "$capacity_retained_cell/venues"
jq -n '{log_mode: "full", evidence_format: "evstream_v3", record_market_data_receipts: false}' \
	>"$capacity_retained_cell/run-config.json"
jq -n --arg revision "$matching_revision" '{git_revision: $revision}' >"$capacity_retained_cell/run-metadata.json"
for retained_json in manifest.json greeks.json latency.json binary-evidence-attestation.json evidence-only-artifact-hash.json; do
	jq -n '{}' >"$capacity_retained_cell/$retained_json"
done
printf '%s\n' '{}' >"$capacity_retained_cell/checkpoints.jsonl"
printf '%s\n' 'binary-evidence' >"$capacity_retained_cell/events.evs"
printf '%s\n' '{}' >"$capacity_retained_cell/venues/a.jsonl"
v2_r2_write_evidence_manifest "$capacity_retained_cell" || fail "could not create retained capacity manifest fixture"
retained_manifest_sha256=$(sha256sum "$capacity_retained_cell/evidence-manifest.json" | awk '{print $1}')
jq --arg probe_root "$(dirname -- "$capacity_retained_cell")" --arg evidence_manifest_sha256 "$retained_manifest_sha256" \
	--argjson initial_available_free_bytes 5000000000 --argjson available_free_bytes 5000000000 \
	--argjson minimum_free_bytes 4294967296 \
	'.probe_root = $probe_root | .evidence_manifest_sha256 = $evidence_manifest_sha256 |
	 .initial_available_free_bytes = $initial_available_free_bytes |
	 .available_free_bytes = $available_free_bytes | .minimum_free_bytes = $minimum_free_bytes' \
	"$capacity_probe_attestation" >"$tmp_root/capacity-attestation-bound.json"
capacity_probe_attestation="$tmp_root/capacity-attestation-bound.json"
v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$capacity_probe_attestation" "config-for-contract-test" 4 4294967296 ||
	fail "a config/process-bound binary capacity attestation was rejected"
jq '.peak_output_bytes = 3000000000 | .required_free_bytes = 5147483648' \
	"$capacity_probe_attestation" >"$tmp_root/capacity-attestation-insufficient-free-space.json"
expect_failure v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$tmp_root/capacity-attestation-insufficient-free-space.json" "config-for-contract-test" 4 4294967296
expect_failure v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$capacity_probe_attestation" "wrong-config" 4
expect_failure v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$capacity_probe_attestation" "config-for-contract-test" 8
expect_failure v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$capacity_probe_attestation" "config-for-contract-test" 4 8589934592
mv "$capacity_retained_cell/evidence-manifest.json" "$tmp_root/evidence-manifest.saved"
expect_failure v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" \
	"$capacity_probe_attestation" "config-for-contract-test" 4 4294967296
mv "$tmp_root/evidence-manifest.saved" "$capacity_retained_cell/evidence-manifest.json"

sv1_capacity_probe="$root_dir/scripts/run-v2-r2-sv1-24h-capacity-probe.sh"
ln -s -- "$root_dir" "$tmp_root/repository-link"
expect_failure env GOMAXPROCS=4 V2_R2_SV1_CAPACITY_ROOT="$root_dir/research/forbidden-capacity-probe" \
	"$sv1_capacity_probe" /bin/true
expect_failure env GOMAXPROCS=4 V2_R2_SV1_CAPACITY_ROOT="$tmp_root/repository-link/forbidden-capacity-probe" \
	"$sv1_capacity_probe" /bin/true
if [[ -e "$sv1_lock_path" || -L "$sv1_lock_path" ]]; then
	[[ -f "$sv1_lock_path" && ! -L "$sv1_lock_path" ]] || fail "SV1 capacity namespace lock is not a regular file"
else
	sv1_lock_created=true
fi
exec {sv1_lock_fd}>>"$sv1_lock_path"
flock -n "$sv1_lock_fd" || fail "could not hold SV1 capacity namespace lock for contention test"
expect_failure env GOMAXPROCS=4 V2_R2_SV1_CAPACITY_ROOT="$tmp_root/contended-capacity-probe" \
	"$sv1_capacity_probe" /bin/true
flock -u "$sv1_lock_fd"
if [[ "$sv1_lock_created" == true ]]; then
	unlink -- "$sv1_lock_path"
	sv1_lock_created=false
fi
capacity_preflight_error="$tmp_root/capacity-preflight-error"
if env GOMAXPROCS=4 V2_R2_SV1_CAPACITY_ROOT="$tmp_root/external-capacity-probe" \
	"$sv1_capacity_probe" /bin/true >"$tmp_root/capacity-preflight-output" 2>"$capacity_preflight_error"; then
	fail "capacity probe unexpectedly accepted a non-Go binary"
fi
if rg -n 'unbound variable|probe_dir' "$capacity_preflight_error" >/dev/null; then
	fail "capacity probe failed before its control-plane checks"
fi
for required_capacity_guard in \
	'trap cleanup_simulator EXIT' \
	'kill -KILL' \
	'retained_output_bytes=$(capacity_directory_bytes' \
	'v2_r2_require_binary_capacity_attestation "$binary" "$head_revision" "$attestation_tmp"'; do
	rg -F "$required_capacity_guard" "$sv1_capacity_probe" >/dev/null ||
		fail "capacity probe is missing required guard: $required_capacity_guard"
done
if rg -n 'historical_full_tree_bytes|35341880370|required_free_bytes=' \
	"$root_dir/scripts/run-v2-integrated-longrun-r2-cell.sh" >/dev/null; then
	fail "registered launcher still contains the obsolete JSON capacity floor"
fi

v2_r2_acquire_namespace_lock || fail "could not acquire the R2 namespace lock for contention test"
expect_failure env -u V2_R2_NAMESPACE_LOCK_FD V2_R2_NAMESPACE_LOCK_HELD=true bash -c \
	"source '$root_dir/scripts/v2-integrated-longrun-r2-contract.sh'; v2_r2_acquire_namespace_lock"

expected_calendar_timeline=$(cat <<'EOF'
[
  {"expiry_nano":1735696800000000000,"future_first_listed_at_nano":1735689601000000000,"option_first_listed_at_nano":1735689601000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735700400000000000,"future_first_listed_at_nano":1735693200000000000,"option_first_listed_at_nano":1735693200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735704000000000000,"future_first_listed_at_nano":1735696800000000000,"option_first_listed_at_nano":1735696800000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735707600000000000,"future_first_listed_at_nano":1735700400000000000,"option_first_listed_at_nano":1735700400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735711200000000000,"future_first_listed_at_nano":1735689601000000000,"option_first_listed_at_nano":1735689601000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735714800000000000,"future_first_listed_at_nano":1735707600000000000,"option_first_listed_at_nano":1735707600000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735718400000000000,"future_first_listed_at_nano":1735711200000000000,"option_first_listed_at_nano":1735711200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735722000000000000,"future_first_listed_at_nano":1735700400000000000,"option_first_listed_at_nano":1735700400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735725600000000000,"future_first_listed_at_nano":1735718400000000000,"option_first_listed_at_nano":1735718400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735729200000000000,"future_first_listed_at_nano":1735722000000000000,"option_first_listed_at_nano":1735722000000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735732800000000000,"future_first_listed_at_nano":1735689601000000000,"option_first_listed_at_nano":1735689601000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735736400000000000,"future_first_listed_at_nano":1735729200000000000,"option_first_listed_at_nano":1735729200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735740000000000000,"future_first_listed_at_nano":1735732800000000000,"option_first_listed_at_nano":1735732800000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735743600000000000,"future_first_listed_at_nano":1735722000000000000,"option_first_listed_at_nano":1735722000000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735747200000000000,"future_first_listed_at_nano":1735740000000000000,"option_first_listed_at_nano":1735740000000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735750800000000000,"future_first_listed_at_nano":1735743600000000000,"option_first_listed_at_nano":1735743600000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735754400000000000,"future_first_listed_at_nano":1735711200000000000,"option_first_listed_at_nano":1735711200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735758000000000000,"future_first_listed_at_nano":1735750800000000000,"option_first_listed_at_nano":1735750800000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735761600000000000,"future_first_listed_at_nano":1735754400000000000,"option_first_listed_at_nano":1735754400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735765200000000000,"future_first_listed_at_nano":1735743600000000000,"option_first_listed_at_nano":1735743600000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735768800000000000,"future_first_listed_at_nano":1735761600000000000,"option_first_listed_at_nano":1735761600000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735772400000000000,"future_first_listed_at_nano":1735765200000000000,"option_first_listed_at_nano":1735765200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735776000000000000,"future_first_listed_at_nano":1735732800000000000,"option_first_listed_at_nano":1735732800000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735779600000000000,"future_first_listed_at_nano":1735772400000000000,"option_first_listed_at_nano":1735772400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735783200000000000,"future_first_listed_at_nano":1735776000000000000,"option_first_listed_at_nano":1735776000000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735786800000000000,"future_first_listed_at_nano":1735765200000000000,"option_first_listed_at_nano":1735765200000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735797600000000000,"future_first_listed_at_nano":1735754400000000000,"option_first_listed_at_nano":1735754400000000000,"future_contract_count":1,"option_contract_count":10},
  {"expiry_nano":1735819200000000000,"future_first_listed_at_nano":1735776000000000000,"option_first_listed_at_nano":1735776000000000000,"future_contract_count":1,"option_contract_count":10}
]
EOF
)
helper_calendar_timeline=$(v2_r2_expected_calendar_listing_timeline)
if ! diff -u \
	<(printf '%s\n' "$expected_calendar_timeline" | jq -S -c .) \
	<(printf '%s\n' "$helper_calendar_timeline" | jq -S -c .); then
	fail "literal calendar fixture drifted from the independently maintained policy helper"
fi
calendar_fixture="$tmp_root/calendar.json"
jq -n --argjson timeline "$expected_calendar_timeline" \
	'($timeline | map(.expiry_nano)) as $expiries |
	 {result: {contract: "calendar-audit-v2", futures_expiry_nanos: $expiries,
		option_expiry_nanos: $expiries, shared_expiry_nanos: $expiries,
		venues: [{listing_timeline: $timeline, futures_listed: 28,
			options_listed: 280, futures_settled: 23, options_settled: 230}]}}' \
	>"$calendar_fixture"
v2_r2_require_calendar_listing_timeline "$calendar_fixture" ||
	fail "registered first-listing timeline was rejected"
calendar_venues_fixture="$tmp_root/calendar-venues.json"
jq -n --argjson timeline "$expected_calendar_timeline" \
	'{result: {contract: "calendar-audit-v2", venues: [
		{venue_id: "central", listing_timeline: $timeline},
		{venue_id: "north", listing_timeline: $timeline},
		{venue_id: "south", listing_timeline: $timeline}]}}' \
	>"$calendar_venues_fixture"
v2_r2_require_calendar_venue_set "$calendar_venues_fixture" ||
	fail "registered venue set was rejected"
jq '.result.venues[1].venue_id = "renamed"' "$calendar_venues_fixture" >"$tmp_root/calendar-renamed-venue.json"
expect_failure v2_r2_require_calendar_venue_set "$tmp_root/calendar-renamed-venue.json"
jq '.result.venues[1].venue_id = ""' "$calendar_venues_fixture" >"$tmp_root/calendar-empty-venue.json"
expect_failure v2_r2_require_calendar_venue_set "$tmp_root/calendar-empty-venue.json"
jq '.result.venues[0].listing_timeline |= map(.future_first_listed_at_nano = 0 | .option_first_listed_at_nano = 0)' \
	"$calendar_fixture" >"$tmp_root/calendar-all-at-zero.json"
expect_failure v2_r2_require_calendar_listing_timeline "$tmp_root/calendar-all-at-zero.json"

expect_failure env GOMAXPROCS=4 "$root_dir/scripts/run-v2-integrated-longrun-r2-cell.sh" holdout-619 /bin/true
expect_failure "$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/verify-v2-integrated-longrun-r2-cell.sh" \
	"$root_dir/research/artifacts/v2-freeze-candidate/smoke-vcs/run-g4"
expect_failure "$root_dir/scripts/archive-v2-integrated-longrun-r2-cell.sh" \
	"$tmp_root/dev-607"
expect_failure "$root_dir/scripts/check-v2-integrated-longrun-r2-parity.sh" "$tmp_root/parity"
expect_failure "$root_dir/scripts/score-v2-integrated-longrun-r2-development.sh" "$tmp_root/score"

printf 'integrated long-run R2 contract tests: pass\n'
