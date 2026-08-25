#!/usr/bin/env bash
# Derive one preregistered same-seed P5 pair only after both cell contracts pass.
set -euo pipefail
if [[ $# -ne 1 ]]; then echo "usage: $0 117|119|139|149|151" >&2; exit 2; fi
seed=$1
case "$seed" in 117|119|139|149|151) ;; *) echo "unregistered P5 seed: $seed" >&2; exit 2 ;; esac
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
base="$root_dir/research/artifacts/v2-5-p5"
control="$base/A-$seed"
treatment="$base/B-$seed"
output="$base/pair-$seed.json"
for cell in "$control" "$treatment"; do
	if [[ ! -s "$cell/analysis-metadata.json" ]]; then echo "P5 pair requires completed extraction: $cell" >&2; exit 1; fi
	jq -e '.analysis_contract == "v2-5-p5-dated-carry-v1"' "$cell/analysis-metadata.json" >/dev/null
done
temporary=$(mktemp "${output}.tmp-XXXXXX")
"$analyzer" -metric datedcarryp5pair -json "$control" "$treatment" >"$temporary"
mv "$temporary" "$output"
echo "extracted P5 pair: seed $seed"
