#!/usr/bin/env bash
# Score the preregistered P4b development pairs after every cell has passed
# its independent evidence contract. This reports the registered basis sign;
# it does not inspect holdouts or substitute secondary endpoints.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
base="$root_dir/research/artifacts/v2-5-p4b/full"
score="$root_dir/research/artifacts/v2-5-p4b/p4b-development-score.json"
seeds=(401 409)

test -x "$analyzer"
for seed in "${seeds[@]}"; do
	for arm in A B; do
		cell="$base/$arm-$seed"
		test -s "$cell/analysis-metadata.json"
		jq -e --arg arm "$arm" --argjson seed "$seed" '
			.analysis_contract == "v2-5-p4b-independent-perp-flow-v1" and
			.arm == $arm and .seed == $seed and
			.completion_sentinels == ["greeks.json", "latency.json"]
		' "$cell/analysis-metadata.json" >/dev/null
	done

	pair="$root_dir/research/artifacts/v2-5-p4b/pair-$seed.json"
	temporary=$(mktemp "${pair}.tmp-XXXXXX")
	if ! "$analyzer" -metric termcarryp4pair -json \
		"$base/A-$seed" "$base/B-$seed" >"$temporary" 2>"$pair.err"; then
		rm -f "$temporary"
		echo "P4b pair analyzer failed for development seed $seed" >&2
		exit 1
	fi
	mv "$temporary" "$pair"
done

for seed in "${seeds[@]}"; do
	jq -e '
		.result.control_cap_bps == 1 and .result.treatment_cap_bps == 75 and
		(.result.control_valid | type) == "boolean" and
		(.result.treatment_valid | type) == "boolean" and
		(.result.activation_valid | type) == "boolean" and
		(.result.execution_valid | type) == "boolean" and
		(.result.basis_measurable | type) == "boolean" and
		(.result.seed_statistic_sign | type) == "number"
	' "$root_dir/research/artifacts/v2-5-p4b/pair-$seed.json" >/dev/null
done

metadata_tmp=$(mktemp "${score}.tmp-XXXXXX")
jq -n \
	--arg contract "v2-5-p4b-independent-perp-flow-v1" \
	--arg source_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--slurpfile p401 "$root_dir/research/artifacts/v2-5-p4b/pair-401.json" \
	--slurpfile p409 "$root_dir/research/artifacts/v2-5-p4b/pair-409.json" \
	--slurpfile a401 "$base/A-401/analysis-metadata.json" \
	--slurpfile a409 "$base/A-409/analysis-metadata.json" \
	--slurpfile b401 "$base/B-401/analysis-metadata.json" \
	--slurpfile b409 "$base/B-409/analysis-metadata.json" \
	--slurpfile ea401 "$base/A-401/perpexposurehedger.json" \
	--slurpfile ea409 "$base/A-409/perpexposurehedger.json" \
	--slurpfile eb401 "$base/B-401/perpexposurehedger.json" \
	--slurpfile eb409 "$base/B-409/perpexposurehedger.json" \
	'
	  def complete($r):
		$r.control_valid and $r.treatment_valid and
		$r.activation_valid and $r.execution_valid and $r.basis_measurable and
		$r.valid;
	  def cellmeta($m): {
		analysis_revision: $m.analysis_revision,
		analyzer_sha256: $m.analyzer_sha256,
		arm: $m.arm,
		seed: $m.seed,
		runtime_evidence_artifact: $m.runtime_evidence_artifact
	  };
	  def exposure($e): {
		valid: $e.result.valid,
		receipt_audit_valid: $e.result.receipt_audit_valid,
		decisions: $e.result.decisions,
		submitted: $e.result.submitted,
		fills: $e.result.fills,
		filled_qty: $e.result.filled_qty,
		venues: $e.result.venues
	  };
	  ($p401[0].result) as $r401 |
	  ($p409[0].result) as $r409 |
	  [$r401, $r409] as $pairs |
	  (if ($pairs | all(complete(.))) then
		(if ($pairs | all(.seed_statistic_sign > 0)) then
			"SUPPORTED (screening)"
		 else "FALSIFIED" end)
	   else "NOT IDENTIFIED" end) as $verdict |
	  {
		contract: $contract,
		verdict: $verdict,
		verdict_rule: "both complete paired seed statistics strictly positive => SUPPORTED (screening); any complete non-positive statistic => FALSIFIED; incomplete/invalid chain => NOT IDENTIFIED",
		primary_endpoint: "exact P4 paired oriented-premium change",
		development_seeds: [401, 409],
		untouched_holdouts: [419, 421, 431],
		source_revision: $source_revision,
		analyzer_sha256: $analyzer_sha256,
		simulator_revisions: [cellmeta($a401[0]), cellmeta($a409[0]), cellmeta($b401[0]), cellmeta($b409[0])],
		paired_results: [
			{seed: 401, result: $r401, complete: complete($r401)},
			{seed: 409, result: $r409, complete: complete($r409)}
		],
		exposure_activation: [
			{cell: "A-401", result: exposure($ea401[0])},
			{cell: "A-409", result: exposure($ea409[0])},
			{cell: "B-401", result: exposure($eb401[0])},
			{cell: "B-409", result: exposure($eb409[0])}
		],
		interpretation_fence: [
			"No profitability, wealth, stability, liquidity, or realism claim is licensed.",
			"A positive development result would still require the registered exposure-off/on interaction factorial and holdout replication.",
			"An exact-zero or negative complete pair is a registered FALSIFIED basis endpoint, not a reason to retune."
		]
	}' >"$metadata_tmp"
mv "$metadata_tmp" "$score"

echo "scored V2-5 P4b development pairs: $score"
