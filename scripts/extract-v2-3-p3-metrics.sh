#!/usr/bin/env bash
# Extract the complete V2-3 P3 screen contract. It never prunes raw JSONL.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
variant=${P3_VARIANT:-v2-3-p3}
artifact_dir="$root_dir/research/artifacts/$variant"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi

write_metric() {
	local output=$1
	shift
	local temp
	temp=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temp"
	mv "$temp" "$output"
}

for arm in A B; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-3 P3 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing P3 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/perpreplenishment.json" "$analyzer" -metric perpreplenishment -json "$cell"
		write_metric "$cell/orderlifecycle.json" "$analyzer" -metric orderlifecycle -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"
		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"

		jq -e '
		  .result.valid == true and
		  .result.decisions > 0 and
		  .result.invalid_decision_records == 0 and
		  .result.invalid_lifecycle_records == 0 and
		  .result.lifecycle_mismatches == 0 and
		  .result.threshold_mismatches == 0 and
		  .result.missing_outcomes == 0 and
		  .result.duplicate_outcomes == 0 and
		  .result.outcome_field_mismatches == 0
		' "$cell/perpreplenishment.json" >/dev/null
		jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: [
			    "evidenceartifacthash.json", "perpreplenishment.json", "orderlifecycle.json",
			    "viability.json", "observationreceipts.json"
			  ],
			  raw_log_policy: "retained; no P3 raw evidence is prunable from this script"
			}' >"$cell/analysis-metadata.json"
		echo "extracted P3 $arm/$seed"
	done
done
