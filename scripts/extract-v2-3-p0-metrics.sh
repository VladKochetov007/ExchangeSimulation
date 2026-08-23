#!/usr/bin/env bash
# Extract the P0 contract before any raw evidence can be considered prunable.
# Final greeks.json plus latency.json are completion sentinels; process names
# and partial directories are deliberately not evidence of a completed world.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p0"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
roles='spot_maker,fixed_distance_maker,imbalance_maker'

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

for arm in A B C; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-3 P0 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing P0 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/postonly-cdf.json" "$analyzer" -metric postonly -json \
			-post-only-roles "$roles" -post-only-symbols CDF/USD "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"

		# P0 uses direct, raw measures for viability; it does not post-hoc turn a
		# price statistic into a viability score. The resulting rows are exactly
		# the CDF/USD venues/windows, with two-sided share made explicit.
		temp=$(mktemp "$cell/cdf-viability.json.tmp-XXXXXX")
		jq '{
		  windows: [.result.windows[] | select(.symbol == "CDF/USD") | . + {
		    two_sided_share: (if .snapshots == 0 then null else 1 - (.empty_side_snapshots / .snapshots) end)
		  }],
		  summaries: [.result.book_summaries[] | select(.symbol == "CDF/USD")]
		}' "$cell/viability.json" >"$temp"
		jq -e '.windows | length > 0' "$temp" >/dev/null
		mv "$temp" "$cell/cdf-viability.json"

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: ["observationreceipts.json", "evidenceartifacthash.json", "postonly-cdf.json", "viability.json", "cdf-viability.json"]
			}' >"$cell/analysis-metadata.json"
		echo "extracted P0 $arm/$seed"
	done
done
