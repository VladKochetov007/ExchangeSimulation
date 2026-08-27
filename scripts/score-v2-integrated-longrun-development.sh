#!/usr/bin/env bash
# Apply the precommitted integrated V2 development qualification rule. The
# scorer consumes only the three registered full development cells and the
# seed-607 parity attestation; it never reads holdout directories.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_root=${1:-"$root_dir/research/artifacts/v2-integrated-longrun/candidate"}
score="$output_root/development-score.json"
parity="$output_root/parity-attestation.json"
contract_version="v2-integrated-longrun-scorer-v1"

fail() {
	printf 'integrated long-run scorer failure: %s\n' "$*" >&2
	exit 1
}
require_file() {
	[[ -s "$1" ]] || fail "missing scorer input: $1"
}
require_object() {
	jq -e 'type == "object"' "$1" >/dev/null || fail "malformed scorer JSON: $1"
}
[[ ! -e "$score" ]] || fail "refusing to overwrite precommitted score: $score"
"$root_dir/scripts/check-v2-integrated-longrun-configs.sh" >/dev/null
"$root_dir/scripts/check-v2-integrated-longrun-parity.sh" "$output_root" >/dev/null
require_file "$parity"
require_object "$parity"

required=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json
	positions.json fillpositions.json orderlifecycle.json lifecycle.json settlements.json
	expiryfills.json evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json
	roleaudit.json ecology.json derivatives.json liquidations.json marginchecks.json
	optionsurface.json optionliabilityp6.json optionvaluetakerp6.json vannavolgap6.json
	exposure.json hedging.json makerrefresh.json makerquotesize.json makerrebalance.json
	postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json
)
required_json=$(printf '%s\n' "${required[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
cell_records=()
for cell in dev-607 dev-613 dev-617; do
	cell_dir="$output_root/$cell"
	for file in analysis-metadata.json integrity.json activation.json; do
		require_file "$cell_dir/$file"
	done
	require_object "$cell_dir/analysis-metadata.json"
	require_object "$cell_dir/integrity.json"
	require_object "$cell_dir/activation.json"
	jq -e --arg cell "$cell" --argjson required_artifacts "$required_json" \
		'.cell == $cell and .seed == (.cell | split("-")[-1] | tonumber) and
		.analysis_contract == "v2-integrated-longrun-candidate-v2" and
		.analyzer_vcs_modified == false and .required_artifacts == $required_artifacts' \
		"$cell_dir/analysis-metadata.json" >/dev/null || fail "invalid analysis metadata: $cell"
	jq -e 'all(.predicates | to_entries[]; .value == true)' "$cell_dir/integrity.json" >/dev/null || fail "integrity predicate failed: $cell"
	jq -e 'all(.result.predicates | to_entries[]; .value == true)' "$cell_dir/activation.json" >/dev/null || fail "activation predicate failed: $cell"
	for inactive in fundingcarry termcarry datedcarryp5 datedmandatep5 perpreplenishment; do
		jq -e --arg metric "$inactive" \
			'.result.status == "OUT_OF_SCOPE" and .result.classification == "RECORDER_NOT_ENABLED" and .result.metric == $metric' \
			"$cell_dir/$inactive.json" >/dev/null || fail "inactive recorder contract failed: $cell/$inactive"
	done
done

source_revision=$(jq -er '.analysis_revision' "$output_root/dev-607/analysis-metadata.json")
analyzer_revision=$(jq -er '.analyzer_revision' "$output_root/dev-607/analysis-metadata.json")
simulator_revision=$(jq -er '.simulator_revision' "$output_root/dev-607/analysis-metadata.json")
analyzer_sha256=$(jq -er '.analyzer_sha256' "$output_root/dev-607/analysis-metadata.json")
simulator_sha256=$(jq -er '.simulator_sha256' "$output_root/dev-607/analysis-metadata.json")
for cell in dev-607 dev-613 dev-617; do
	jq -e --arg source_revision "$source_revision" --arg analyzer_revision "$analyzer_revision" \
		--arg simulator_revision "$simulator_revision" --arg analyzer_sha256 "$analyzer_sha256" \
		--arg simulator_sha256 "$simulator_sha256" \
		'.analysis_revision == $source_revision and .analyzer_revision == $analyzer_revision and
		.simulator_revision == $simulator_revision and .analyzer_sha256 == $analyzer_sha256 and
		.simulator_sha256 == $simulator_sha256 and .analyzer_vcs_modified == false' \
		"$output_root/$cell/analysis-metadata.json" >/dev/null || fail "provenance differs: $cell"
done

cells_json=$(jq -n \
	--slurpfile m607 "$output_root/dev-607/analysis-metadata.json" \
	--slurpfile i607 "$output_root/dev-607/integrity.json" \
	--slurpfile a607 "$output_root/dev-607/activation.json" \
	--slurpfile m613 "$output_root/dev-613/analysis-metadata.json" \
	--slurpfile i613 "$output_root/dev-613/integrity.json" \
	--slurpfile a613 "$output_root/dev-613/activation.json" \
	--slurpfile m617 "$output_root/dev-617/analysis-metadata.json" \
	--slurpfile i617 "$output_root/dev-617/integrity.json" \
	--slurpfile a617 "$output_root/dev-617/activation.json" \
	'[
		{cell: $m607[0].cell, seed: $m607[0].seed, source_revision: $m607[0].analysis_revision,
			analyzer_revision: $m607[0].analyzer_revision, simulator_revision: $m607[0].simulator_revision,
			integrity: $i607[0].predicates, activation: $a607[0].result.predicates},
		{cell: $m613[0].cell, seed: $m613[0].seed, source_revision: $m613[0].analysis_revision,
			analyzer_revision: $m613[0].analyzer_revision, simulator_revision: $m613[0].simulator_revision,
			integrity: $i613[0].predicates, activation: $a613[0].result.predicates},
		{cell: $m617[0].cell, seed: $m617[0].seed, source_revision: $m617[0].analysis_revision,
			analyzer_revision: $m617[0].analyzer_revision, simulator_revision: $m617[0].simulator_revision,
			integrity: $i617[0].predicates, activation: $a617[0].result.predicates}
	]')

parity_sha256=$(sha256sum "$parity" | awk '{print $1}')
score_tmp=$(mktemp "$score.tmp-XXXXXX")
jq -n \
	--arg contract "$contract_version" \
	--arg source_revision "$source_revision" \
	--arg analyzer_revision "$analyzer_revision" \
	--arg simulator_revision "$simulator_revision" \
	--arg analyzer_sha256 "$analyzer_sha256" \
	--arg simulator_sha256 "$simulator_sha256" \
	--arg parity_sha256 "$parity_sha256" \
	--argjson cells "$cells_json" \
	'def all_true($objects): all($objects[]; all(to_entries[]; .value == true));
	 {
		schema_version: 1, contract: $contract,
		status: (if (all_true([$cells[].integrity]) and all_true([$cells[].activation])) then "QUALIFIED" else "BLOCKED" end),
		claim_scope: "integrated deterministic/evidence/lifecycle candidate gate only; no market realism claim",
		source_revision: $source_revision, analyzer_revision: $analyzer_revision,
		analyzer_sha256: $analyzer_sha256, simulator_revision: $simulator_revision,
		simulator_sha256: $simulator_sha256, parity_attestation_sha256: $parity_sha256,
		development_seeds: [607, 613, 617], reserved_holdout_seeds: [619, 631, 641],
		holdouts_consumed: false,
		predicates: {
			all_development_integrity: all_true([$cells[].integrity]),
			all_development_activation: all_true([$cells[].activation]),
			parity_attested: true,
			provenance_consistent: (($cells | map(.source_revision) | unique | length) == 1 and
				($cells | map(.analyzer_revision) | unique | length) == 1 and
				($cells | map(.simulator_revision) | unique | length) == 1),
			holdouts_unconsumed: true
		},
		cells: $cells,
		interpretation: [
			"QUALIFIED licenses an immutable freeze bundle; it is not a realism score.",
			"P4/P5 carry mechanisms and P3 replenishment remain OUT_OF_SCOPE when their recorders are disabled.",
			"Funding anchoring, basis convergence, executable price discovery, liquidation reachability, endogenous option shape, and ecology realism remain separate claims."
		]
	}' >"$score_tmp"
mv "$score_tmp" "$score"
require_object "$score"
jq -e '(.status == "QUALIFIED" or .status == "BLOCKED") and
	(if .status == "QUALIFIED" then all(.predicates | to_entries[]; .value == true) else true end)' \
	"$score" >/dev/null || fail "score self-check failed"
printf 'integrated long-run development score written: %s\n' "$score"
