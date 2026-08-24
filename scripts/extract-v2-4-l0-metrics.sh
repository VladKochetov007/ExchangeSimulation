#!/usr/bin/env bash
# Extract V2-4 L0's complete preregistered evidence contract. This script never
# prunes raw evidence or promotes a completion sentinel into an audit result.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-4-l0"
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
			echo "incomplete V2-4 L0 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing L0 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/liabilityhedger.json" "$analyzer" -metric liabilityhedger -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"

		jq -e '.result.valid == true and .result.decisions > 0 and .result.state_updates > 0 and (.result.hedgers | length) == 3 and all(.result.hedgers[]; .state_updates >= 20)' "$cell/liabilityhedger.json" >/dev/null
		jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
		jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) > 0' "$cell/evidenceartifacthash.json" >/dev/null

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: [
			    "observationreceipts.json", "evidenceartifacthash.json", "liabilityhedger.json", "viability.json"
			  ],
			  raw_log_policy: "retained; no L0 raw evidence is prunable from this script"
			}' >"$cell/analysis-metadata.json"
		echo "extracted L0 $arm/$seed"
	done
done
