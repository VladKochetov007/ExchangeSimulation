#!/usr/bin/env bash
# Recompute every derived development artifact from retained raw evidence and
# compare it with the stored artifact. This is the scorer's independent,
# deterministic guard against post hoc edits to metrics or integrity sidecars.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	printf 'usage: %s CELL_DIR\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
verification_variant=${V2_R2_EXTRACTOR_VARIANT:-historical}
case "$verification_variant" in
	historical) contract_script="$root_dir/scripts/v2-integrated-longrun-r2-contract.sh" ;;
	sv1) contract_script="$root_dir/scripts/v2-r2-sv1-24h-contract.sh" ;;
	*) printf 'integrated long-run verification failure: unsupported verification variant: %s\n' "$verification_variant" >&2; exit 2 ;;
esac
source "$contract_script"
cell=$1
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

fail() {
	printf 'integrated long-run verification failure: %s\n' "$*" >&2
	exit 1
}
v2_r2_acquire_namespace_lock || fail "could not acquire the R2 evidence namespace lock"

v2_r2_require_output_root "$v2_r2_output_root" || fail "R2 output root is not canonical"
v2_r2_require_cell_path "$cell" || fail "cell is outside the canonical R2 evidence root or is symlinked: $cell"
[[ -x "$analyzer" ]] || fail "missing analyzer: $analyzer"

cell=$(realpath -e -- "$cell")
verification_tmp_root=${V2_VERIFY_TMP_ROOT:-"$root_dir/.git"}
analysis_dir=$(mktemp -d "$verification_tmp_root/v2-r2-verify.XXXXXX")
trap 'rm -rf -- "$analysis_dir"' EXIT

if ! V2_ANALYSIS_OUTPUT_DIR="$analysis_dir" MVANALYZE_BIN="$analyzer" \
	V2_R2_EXTRACTOR_VARIANT="$verification_variant" \
	"$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" "$cell" >/dev/null; then
	fail "fresh extraction failed: $(basename "$cell")"
fi

derived_artifacts=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json positions.json
	fillpositions.json orderlifecycle.json lifecycle.json calendar.json settlements.json expiryfills.json
	evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json roleaudit.json ecology.json
	derivatives.json liquidations.json marginchecks.json optionsurface.json optionliabilityp6.json
	optionvaluetakerp6.json vannavolgap6.json exposure.json hedging.json makerrefresh.json makerquotesize.json
	makerrebalance.json postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json analysis-metadata.json
)
if [[ "$verification_variant" == sv1 ]]; then
	derived_artifacts+=(cdfliquidity.json priceunavailable.json)
fi

for artifact in "${derived_artifacts[@]}"; do
	original="$cell/$artifact"
	recomputed="$analysis_dir/$artifact"
	[[ -s "$original" ]] || fail "stored derived artifact is missing: $original"
	[[ -s "$recomputed" ]] || fail "freshly recomputed artifact is missing: $recomputed"
	if ! jq -e 'type == "object"' "$original" >/dev/null; then
		fail "stored derived artifact is not a JSON object: $original"
	fi
	if ! jq -e 'type == "object"' "$recomputed" >/dev/null; then
		fail "freshly recomputed artifact is not a JSON object: $recomputed"
	fi
	if ! cmp -s <(jq -S -c . "$original") <(jq -S -c . "$recomputed"); then
		fail "stored derived artifact differs from fresh raw-evidence recomputation: $artifact"
	fi
done

printf 'integrated long-run derived evidence reverified: %s\n' "$cell"
