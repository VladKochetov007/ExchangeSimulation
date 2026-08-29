#!/usr/bin/env bash
# Apply the precommitted integrated V2 development qualification rule. The
# scorer consumes only the three registered full development cells and the
# seed-607 parity attestation; it never reads holdout directories.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r5-contract.sh"
output_root=${1:-"$v2_r5_output_root"}
score="$output_root/development-score.json"
parity="$output_root/parity-attestation.json"
contract_version="v2-integrated-longrun-scorer-v4"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

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
v2_r5_require_output_root "$output_root" || fail "scorer root is not the canonical r5 evidence root"
[[ ! -e "$score" ]] || fail "refusing to overwrite precommitted score: $score"
[[ -x "$analyzer" ]] || fail "missing analyzer: $analyzer"
for holdout in holdout-619 holdout-631 holdout-641; do
	[[ ! -e "$output_root/$holdout" ]] || fail "reserved holdout output exists before freeze authorization: $holdout"
done
"$root_dir/scripts/check-v2-integrated-longrun-configs.sh" >/dev/null
"$root_dir/scripts/check-v2-integrated-longrun-parity.sh" "$output_root" >/dev/null
require_file "$parity"
require_object "$parity"
jq -e '.contract == "v2-integrated-longrun-parity-v3" and
	(.simulator_binary_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulator_binary_go_version | startswith("go1.27")) and
	(.prunegate_sha256 | test("^[0-9a-f]{64}$")) and
	(.prunegate_go_version | startswith("go1.27")) and
	(.prunegate_revision | test("^[0-9a-f]{40}$")) and
	(.predicates | keys) == ["deterministic_sidecars_equal", "full_evidence_equal", "no_log_evidence_absent", "ordered_checkpoints_equal", "source_and_build_identity_equal"] and
	all(.predicates | to_entries[]; .value == true)' "$parity" >/dev/null || fail "invalid parity attestation"

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
	v2_r5_require_cell_path "$cell_dir" || fail "scorer cell is outside the canonical r5 root or is symlinked: $cell"
	MVANALYZE_BIN="$analyzer" "$root_dir/scripts/verify-v2-integrated-longrun-cell.sh" "$cell_dir" >/dev/null ||
		fail "fresh raw-evidence derivation did not match stored artifacts: $cell"
	for file in analysis-metadata.json integrity.json activation.json; do
		require_file "$cell_dir/$file"
	done
	require_object "$cell_dir/analysis-metadata.json"
	require_object "$cell_dir/integrity.json"
	require_object "$cell_dir/activation.json"
	jq -e --arg cell "$cell" --argjson required_artifacts "$required_json" \
		'.cell == $cell and .seed == (.cell | split("-")[-1] | tonumber) and
		.analysis_contract == "v2-integrated-longrun-candidate-v5" and
		.integrity_contract == "v2-integrated-longrun-candidate-v5" and
		.activation_contract == "v2-integrated-longrun-candidate-v5" and
		.analyzer_trimpath == true and .analyzer_cgo_enabled == "0" and
		.simulator_trimpath == true and .simulator_cgo_enabled == "0" and
		(.analyzer_go_version | startswith("go1.27")) and (.simulator_go_version | startswith("go1.27")) and
		(.prunegate_revision | test("^[0-9a-f]{40}$")) and (.prunegate_sha256 | test("^[0-9a-f]{64}$")) and
		.prunegate_trimpath == true and .prunegate_cgo_enabled == "0" and
		(.prunegate_go_version | startswith("go1.27")) and
		(.analyzer_go_version | type) == "string" and (.simulator_go_version | type) == "string" and
		.analyzer_vcs_modified == false and .required_artifacts == $required_artifacts and
		.require_exact_replay == true and
		(.artifact_sha256 | keys) == ($required_artifacts | sort) and
		all(.artifact_sha256 | to_entries[]; (.value | test("^[0-9a-f]{64}$")))' \
		"$cell_dir/analysis-metadata.json" >/dev/null || fail "invalid analysis metadata: $cell"
	for artifact in "${required[@]}"; do
		require_file "$cell_dir/$artifact"
		require_object "$cell_dir/$artifact"
		actual_sha256=$(sha256sum "$cell_dir/$artifact" | awk '{print $1}')
		declared_sha256=$(jq -er --arg artifact "$artifact" '.artifact_sha256[$artifact]' "$cell_dir/analysis-metadata.json")
		[[ "$actual_sha256" == "$declared_sha256" ]] || fail "artifact hash mismatch: $cell/$artifact"
	done
	jq -e '.schema_version == 1 and .contract == "v2-integrated-longrun-candidate-v5" and
		(.predicates | keys) == ["activation", "conservation", "derivatives", "expiry", "exposure", "fill_positions", "frontier_vectors", "hedging", "late_path", "liability_hedger", "liquidations", "maker_quote_size", "maker_rebalance", "maker_refresh", "margin", "mechanical", "observation_receipts", "option_liability", "option_surface", "option_value_taker", "order_lifecycle", "position_rounding", "positions", "post_only", "settlement", "vanna_volga"] and
		all(.predicates | to_entries[]; .value == true)' "$cell_dir/integrity.json" >/dev/null || fail "integrity predicate failed: $cell"
	jq -e '.schema_version == 1 and .result.contract == "v2-integrated-longrun-candidate-v5" and
		(.result.predicates | length) == 2 and
		(.result.predicates | keys) == ["cdf_collateral_borrowing_observed", "zero_price_unavailable_order_rejections"] and
		all(.result.predicates | to_entries[]; .value == true)' "$cell_dir/activation.json" >/dev/null || fail "activation predicate failed: $cell"
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
analyzer_trimpath=$(jq -er '.analyzer_trimpath' "$output_root/dev-607/analysis-metadata.json")
analyzer_cgo_enabled=$(jq -er '.analyzer_cgo_enabled' "$output_root/dev-607/analysis-metadata.json")
analyzer_go_version=$(jq -er '.analyzer_go_version' "$output_root/dev-607/analysis-metadata.json")
simulator_trimpath=$(jq -er '.simulator_trimpath' "$output_root/dev-607/analysis-metadata.json")
simulator_cgo_enabled=$(jq -er '.simulator_cgo_enabled' "$output_root/dev-607/analysis-metadata.json")
simulator_go_version=$(jq -er '.simulator_go_version' "$output_root/dev-607/analysis-metadata.json")
parity_source_revision=$(jq -er '.source_revision' "$parity")
[[ "$parity_source_revision" == "$simulator_revision" ]] || fail "parity simulator revision differs from development simulator revision"
parity_simulator_sha256=$(jq -er '.simulator_binary_sha256' "$parity")
parity_simulator_go_version=$(jq -er '.simulator_binary_go_version' "$parity")
[[ "$parity_simulator_sha256" == "$simulator_sha256" ]] || fail "parity simulator binary differs from development simulator binary"
[[ "$parity_simulator_go_version" == "$simulator_go_version" ]] || fail "parity simulator Go toolchain differs from development simulator"
prunegate_revision=$(jq -er '.prunegate_revision' "$output_root/dev-607/analysis-metadata.json")
prunegate_sha256=$(jq -er '.prunegate_sha256' "$output_root/dev-607/analysis-metadata.json")
prunegate_go_version=$(jq -er '.prunegate_go_version' "$output_root/dev-607/analysis-metadata.json")
parity_prunegate_revision=$(jq -er '.prunegate_revision' "$parity")
parity_prunegate_sha256=$(jq -er '.prunegate_sha256' "$parity")
parity_prunegate_go_version=$(jq -er '.prunegate_go_version' "$parity")
[[ "$parity_prunegate_revision" == "$prunegate_revision" ]] || fail "parity prunegate revision differs from development provenance"
[[ "$parity_prunegate_sha256" == "$prunegate_sha256" ]] || fail "parity prunegate binary differs from development provenance"
[[ "$parity_prunegate_go_version" == "$prunegate_go_version" ]] || fail "parity prunegate Go toolchain differs from development provenance"
for cell in dev-607 dev-613 dev-617; do
	jq -e --arg source_revision "$source_revision" --arg analyzer_revision "$analyzer_revision" \
		--arg simulator_revision "$simulator_revision" --arg analyzer_sha256 "$analyzer_sha256" \
		--arg simulator_sha256 "$simulator_sha256" --argjson analyzer_trimpath "$analyzer_trimpath" \
		--arg analyzer_cgo_enabled "$analyzer_cgo_enabled" --arg analyzer_go_version "$analyzer_go_version" \
		--argjson simulator_trimpath "$simulator_trimpath" --arg simulator_cgo_enabled "$simulator_cgo_enabled" \
		--arg simulator_go_version "$simulator_go_version" --arg prunegate_revision "$prunegate_revision" \
		--arg prunegate_sha256 "$prunegate_sha256" --arg prunegate_go_version "$prunegate_go_version" \
		'.analysis_revision == $source_revision and .analyzer_revision == $analyzer_revision and
		.simulator_revision == $simulator_revision and .analyzer_sha256 == $analyzer_sha256 and
		.simulator_sha256 == $simulator_sha256 and .analyzer_vcs_modified == false and
		.prunegate_revision == $prunegate_revision and .prunegate_sha256 == $prunegate_sha256 and
		.prunegate_go_version == $prunegate_go_version and
		.analyzer_trimpath == $analyzer_trimpath and .analyzer_cgo_enabled == $analyzer_cgo_enabled and
		.analyzer_go_version == $analyzer_go_version and .simulator_trimpath == $simulator_trimpath and
		.simulator_cgo_enabled == $simulator_cgo_enabled and .simulator_go_version == $simulator_go_version' \
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
	--argjson analyzer_trimpath "$analyzer_trimpath" --arg analyzer_cgo_enabled "$analyzer_cgo_enabled" \
	--arg analyzer_go_version "$analyzer_go_version" --argjson simulator_trimpath "$simulator_trimpath" \
	--arg simulator_cgo_enabled "$simulator_cgo_enabled" --arg simulator_go_version "$simulator_go_version" \
	--arg prunegate_revision "$prunegate_revision" --arg prunegate_sha256 "$prunegate_sha256" \
	--arg prunegate_go_version "$prunegate_go_version" \
	--arg parity_sha256 "$parity_sha256" \
	--argjson cells "$cells_json" \
	--slurpfile parity_report "$parity" \
	'def all_true($objects): (($objects | length) > 0 and all($objects[]; ((. | type) == "object" and (length > 0) and all(to_entries[]; .value == true))));
	 {
		schema_version: 1, contract: $contract,
		status: (if (all_true([$cells[].integrity]) and all_true([$cells[].activation]) and
			all($parity_report[0].predicates | to_entries[]; .value == true) and
			$parity_report[0].source_revision == $simulator_revision and
			$parity_report[0].simulator_binary_sha256 == $simulator_sha256 and
			$parity_report[0].simulator_binary_go_version == $simulator_go_version and
			$parity_report[0].prunegate_revision == $prunegate_revision and
			$parity_report[0].prunegate_sha256 == $prunegate_sha256 and
			$parity_report[0].prunegate_go_version == $prunegate_go_version) then "QUALIFIED" else "BLOCKED" end),
		claim_scope: "integrated deterministic/evidence/lifecycle candidate gate only; no market realism claim",
		source_revision: $source_revision, analyzer_revision: $analyzer_revision,
		analyzer_sha256: $analyzer_sha256, simulator_revision: $simulator_revision,
		simulator_sha256: $simulator_sha256, analyzer_trimpath: $analyzer_trimpath,
		analyzer_cgo_enabled: $analyzer_cgo_enabled, analyzer_go_version: $analyzer_go_version,
		simulator_trimpath: $simulator_trimpath, simulator_cgo_enabled: $simulator_cgo_enabled,
		simulator_go_version: $simulator_go_version, prunegate_revision: $prunegate_revision,
		prunegate_sha256: $prunegate_sha256, prunegate_go_version: $prunegate_go_version,
		parity_attestation_sha256: $parity_sha256,
		development_seeds: [607, 613, 617], reserved_holdout_seeds: [619, 631, 641],
		holdout_status: "RESERVED_AND_NOT_READ_BY_DEVELOPMENT_SCORER",
		predicates: {
			all_development_integrity: all_true([$cells[].integrity]),
			all_development_activation: all_true([$cells[].activation]),
			parity_attested: (all($parity_report[0].predicates | to_entries[]; .value == true) and
				$parity_report[0].source_revision == $simulator_revision and
				$parity_report[0].simulator_binary_sha256 == $simulator_sha256 and
				$parity_report[0].simulator_binary_go_version == $simulator_go_version and
				$parity_report[0].prunegate_revision == $prunegate_revision and
				$parity_report[0].prunegate_sha256 == $prunegate_sha256 and
				$parity_report[0].prunegate_go_version == $prunegate_go_version),
			provenance_consistent: (($cells | map(.source_revision) | unique | length) == 1 and
				($cells | map(.analyzer_revision) | unique | length) == 1 and
				($cells | map(.simulator_revision) | unique | length) == 1),
			holdout_outputs_absent: true,
			holdout_access_policy_enforced: true
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
