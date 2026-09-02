#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
filter="$root_dir/scripts/v2-r2-sv1-terminal-measurement.jq"
outcome_filter="$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq"
end_nano=1735776000000000000
fixture_dir=$(mktemp -d "${TMPDIR:-$root_dir}/sv1-terminal-contract.XXXXXX")
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

write_outcome_fixture() {
	local name=$1 status=$2 code=$3 sealed=$4 risk=$5 population=$6 error=${7:-}
	if [[ -n "$error" ]]; then
		jq -n --arg status "$status" --arg code "$code" --arg error "$error" \
			--arg stage "terminal_risk_capture" --arg failure_venue_id "north" --arg failure_symbol "CDF/USD" \
			--argjson sealed "$sealed" --argjson risk "$risk" --argjson population "$population" \
			'{schema_version: 2, status: $status, code: $code, phase: "terminal_post_mark",
			 simulation_start_nano: 10, simulation_end_nano: 20,
			 strict_population_accounting: true, evidence_format: "evstream_v3",
			 evidence_sealed: $sealed, terminal_risk_captured: $risk,
			 terminal_population_captured: $population, stage: $stage, failure_at_nano: 20,
			 failure_venue_id: $failure_venue_id, failure_symbol: $failure_symbol, error: $error}' \
			>"$fixture_dir/$name.json"
		return
	fi
	jq -n --arg status "$status" --arg code "$code" \
		--argjson sealed "$sealed" --argjson risk "$risk" --argjson population "$population" \
			'{schema_version: 2, status: $status, code: $code, phase: "terminal_post_mark",
		 simulation_start_nano: 10, simulation_end_nano: 20,
		 strict_population_accounting: true, evidence_format: "evstream_v3",
		 evidence_sealed: $sealed, terminal_risk_captured: $risk,
		 terminal_population_captured: $population}' \
		>"$fixture_dir/$name.json"
}

outcome_valid() {
	jq -e --argjson start 10 --argjson end 20 -f "$outcome_filter" "$1" >/dev/null
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

write_outcome_fixture completed completed COMPLETED true true true
write_outcome_fixture unavailable terminal_failure PRICE_UNAVAILABLE true false false "no usable price"
write_outcome_fixture unsealed terminal_failure PRICE_UNAVAILABLE false false false "no usable price"
write_outcome_fixture software terminal_failure SIMULATION_FAILURE true false false "software failure"
write_outcome_fixture incomplete completed COMPLETED true false false

outcome_valid "$fixture_dir/completed.json" || fail "completed typed outcome was rejected"
outcome_valid "$fixture_dir/unavailable.json" || fail "sealed economic terminal failure was rejected"
if outcome_valid "$fixture_dir/unsealed.json"; then
	fail "unsealed terminal failure was accepted"
fi
if outcome_valid "$fixture_dir/software.json"; then
	fail "generic software failure was accepted as an economic endpoint"
fi
if outcome_valid "$fixture_dir/incomplete.json"; then
	fail "incomplete successful outcome was accepted"
fi

printf '✓ SV1 terminal measurement separates typed endpoint failure from invalid evidence\n'
