#!/usr/bin/env bash
# Cheap control-plane tests for the R2 successor. These tests never launch a
# simulation and never inspect a reserved holdout artifact.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf -- "$tmp_root"' EXIT

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

capacity_probe_binary="$tmp_root/capacity-probe"
cp -- /bin/true "$capacity_probe_binary"
capacity_probe_attestation="$tmp_root/binary-capacity.json"
capacity_probe_sha256=$(sha256sum -- "$capacity_probe_binary" | awk '{print $1}')
jq -n --arg source_revision "$matching_revision" --arg binary_sha256 "$capacity_probe_sha256" \
	'{schema_version: 1, contract: "v2-integrated-longrun-r2-binary-capacity-v1",
	 measurement: "full_24h_binary_evidence_capacity_probe", evidence_format: "evstream_v3",
	 source_revision: $source_revision, binary_sha256: $binary_sha256,
	 peak_output_bytes: 1, safety_margin_bytes: 2147483648,
	 required_free_bytes: 2147483649}' >"$capacity_probe_attestation"
v2_r2_require_binary_capacity_attestation "$capacity_probe_binary" "$matching_revision" "$capacity_probe_attestation" ||
	fail "a valid binary capacity attestation was rejected"
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
