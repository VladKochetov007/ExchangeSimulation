#!/usr/bin/env bash
# Fail-closed mechanics-only check for completed P7c 15-minute preflights.
# This intentionally does not run or score margin/liquidation outcomes.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 PREFLIGHT_ROOT" >&2
	exit 2
fi
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base=$(CDPATH= cd -- "$1" && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
[[ -x "$analyzer" ]] || { echo "missing analyzer: $analyzer" >&2; exit 1; }

for id in C-367 C-371 T-367 T-371; do
	cell="$base/$id"
	[[ -d "$cell" ]] || { echo "missing P7c preflight: $cell" >&2; exit 1; }
	[[ -s "$cell/greeks.json" && -s "$cell/latency.json" ]] || {
		echo "preflight lacks final sentinels: $id" >&2; exit 1;
	}
	jq -e --arg cell "${id%%-*}" --argjson seed "${id##*-}" \
		'.hypothesis_id == "V2-7-P7C-DISTRESS" and .cell == $cell and
		 .seed == $seed and .preflight == true and .simulated_horizon == "15m" and
		 .completion_sentinels == ["greeks.json", "latency.json"]' \
		"$cell/run-metadata.json" >/dev/null

	for metric in perpexposurehedger observationreceipts evidenceartifacthash streamhash orderlifecycle fillpositions positions; do
		out="$cell/$metric.json"
		if [[ ! -s "$out" ]]; then
			tmp=$(mktemp "$out.tmp-XXXXXX")
			if ! "$analyzer" -metric "$metric" -json "$cell" >"$tmp" 2>"$out.err"; then
				rm -f "$tmp"
				echo "P7c preflight metric failed: $id/$metric" >&2
				exit 1
			fi
			mv "$tmp" "$out"
		fi
	done

	jq -e '.result.valid == true and .result.receipt_audit_valid == true and
		.result.future_receipt_use == 0 and .result.decision_mismatches == 0 and
		.result.outcome_mismatches == 0 and .result.missing_outcomes == 0 and
		.result.duplicate_outcomes == 0 and .result.fill_quantity_mismatches == 0' \
		"$cell/perpexposurehedger.json" >/dev/null
	jq -e '.result.valid == true and .result.future_decision_use == 0 and
		.result.bad_global_event_order == 0 and .result.bad_receipt_ordinal == 0 and
		.result.bad_schedule_ordinal == 0 and .result.duplicate_source_identity == 0 and
		.result.receipt_without_schedule == 0 and .result.schedule_receipt_mismatch == 0 and
		.result.bad_decision_frontier == 0' "$cell/observationreceipts.json" >/dev/null
	jq -e '.result.events > 0 and (.result.digest|type) == "string" and
		(.result.digest|length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
	runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
	jq -e --argjson events "$runtime_events" --arg digest "$runtime_digest" \
		'.result.events == $events and .result.digest == $digest' \
		"$cell/evidenceartifacthash.json" >/dev/null
	jq -e '.result.events > 0 and (.result.digest|length) == 64' "$cell/streamhash.json" >/dev/null
	if [[ "${id%%-*}" == C ]]; then
		jq -e '.result.enabled_decisions == 0 and .result.submitted == 0 and
			.result.fills == 0 and .result.filled_qty == 0' \
			"$cell/perpexposurehedger.json" >/dev/null
	else
		jq -e '.result.enabled_decisions > 0 and .result.submitted > 0 and
			.result.accepted > 0 and .result.fills > 0 and .result.filled_qty > 0' \
			"$cell/perpexposurehedger.json" >/dev/null
	fi
done

echo "P7c preflight mechanics/evidence contract passed (risk outcomes not scored)"
