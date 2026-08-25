#!/usr/bin/env bash
# Derive the registered same-seed P4 chain and basis event study only after both
# arms have passed their complete cell-level evidence contract.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 107|109|127|131|137" >&2
	exit 2
fi
seed=$1
case "$seed" in 107|109|127|131|137) ;; *) echo "unregistered P4 seed: $seed" >&2; exit 2 ;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
base="$root_dir/research/artifacts/v2-5-p4"
control="$base/A-$seed"
treatment="$base/B-$seed"
output="$base/pair-$seed.json"

for cell in "$control" "$treatment"; do
	if [[ ! -s "$cell/analysis-metadata.json" ]]; then
		echo "P4 pair requires completed extraction: $cell" >&2
		exit 1
	fi
	jq -e '.analysis_contract == "v2-5-p4-funding-carry-v1"' "$cell/analysis-metadata.json" >/dev/null
done

temporary=$(mktemp "${output}.tmp-XXXXXX")
"$analyzer" -metric termcarryp4pair -json "$control" "$treatment" >"$temporary"
mv "$temporary" "$output"
jq -e '
  .result.control_cap_bps == 1 and .result.treatment_cap_bps == 75 and
  .result.control_valid == true and .result.treatment_valid == true
' "$output" >/dev/null

echo "extracted V2-5 P4 pair: seed $seed"
