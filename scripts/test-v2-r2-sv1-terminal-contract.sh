#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
filter="$root_dir/scripts/v2-r2-sv1-terminal-measurement.jq"
end_nano=1735776000000000000
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/sv1-terminal-contract.XXXXXX")
trap 'rm -rf -- "$fixture_dir"' EXIT

fail() {
	printf 'SV1 terminal contract test failure: %s\n' "$*" >&2
	exit 1
}

write_fixture() {
	local name=$1 cdf=$2 usd=$3 phase=$4 timestamp=$5 source=$6
	jq -n \
		--arg phase "$phase" --arg source "$source" \
		--argjson cdf "$cdf" --argjson usd "$usd" --argjson timestamp "$timestamp" \
		'{terminal_accounts: [{phase: $phase, account: {timestamp: $timestamp}, mark_source: $source, marks: {CDF: $cdf, USD: $usd}}]}' \
		>"$fixture_dir/$name.json"
}

measurement_valid() {
	jq -e --argjson end "$end_nano" -f "$filter" "$1" >/dev/null
}

strict_mark_valid() {
	measurement_valid "$1" && jq -e '
		all(.terminal_accounts[];
			.mark_source == "two_sided_ABC_USD_and_CDF_USD_mid" and
			.marks.CDF > 0 and .marks.USD > 0)' "$1" >/dev/null
}

write_fixture positive 3000 100 "terminal_post_mark" "$end_nano" "two_sided_ABC_USD_and_CDF_USD_mid"
write_fixture zero-cdf 0 100 "terminal_post_mark" "$end_nano" "two_sided_ABC_USD_and_CDF_USD_mid"
write_fixture wrong-phase 3000 100 "terminal_pre_mark" "$end_nano" "two_sided_ABC_USD_and_CDF_USD_mid"
write_fixture wrong-time 3000 100 "terminal_post_mark" 1735775999999999999 "two_sided_ABC_USD_and_CDF_USD_mid"

measurement_valid "$fixture_dir/positive.json" || fail "positive numeric endpoint was rejected"
strict_mark_valid "$fixture_dir/positive.json" || fail "positive strict endpoint was rejected"
measurement_valid "$fixture_dir/zero-cdf.json" || fail "typed zero endpoint was treated as missing measurement"
if strict_mark_valid "$fixture_dir/zero-cdf.json"; then
	fail "zero CDF endpoint was accepted as strict valuation"
fi
if measurement_valid "$fixture_dir/wrong-phase.json"; then
	fail "pre-mark endpoint was accepted as terminal measurement"
fi
if measurement_valid "$fixture_dir/wrong-time.json"; then
	fail "wrong-time endpoint was accepted as terminal measurement"
fi

printf '✓ SV1 terminal measurement separates typed endpoint failure from invalid evidence\n'
