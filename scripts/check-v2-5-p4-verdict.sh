#!/usr/bin/env bash
# Reconstruct the registered P4 classification predicate from paired artifacts.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/artifacts/v2-5-p4"
verdict="$base/p4-verdict.json"

jq -e '
  .verdict == "FALSIFIED" and
  .links.delivered_funding == true and
  .links.independent_expected_funding == true and
  .links.exact_net_carry == true and
  .links.target_inventory_response == true and
  .links.ordinary_spot_perp_execution == true and
  .links.basis_response == false and
  .holdout_seeds_consumed == []
' "$verdict" >/dev/null

for seed in 107 109; do
	pair="$base/pair-$seed.json"
	jq -e '
    .result.control_valid == true and .result.treatment_valid == true and
    .result.activation_valid == true and .result.execution_valid == true and
    .result.basis_measurable == true and .result.valid == true and
    .result.seed_statistic_exact == "0" and .result.seed_statistic_sign == 0 and
    (.result.venues | length) == 2 and
    (.result.venues | all(
      .local_inputs_comparable == true and
      .funding_changed_as_predicted == true and
      .exact_carry_crossed == true and
      .target_changed_as_predicted == true and
      .execution_qualified == true and
      .control_basis.measurable == true and .treatment_basis.measurable == true and
      .control_basis.pre_mean_oriented_premium_bps_exact == "20000/9999" and
      .control_basis.post_mean_oriented_premium_bps_exact == "20000/9999" and
      .treatment_basis.pre_mean_oriented_premium_bps_exact == "20000/9999" and
      .treatment_basis.post_mean_oriented_premium_bps_exact == "20000/9999" and
      .paired_convergence_exact == "0" and .paired_convergence_sign == 0
    ))
  ' "$pair" >/dev/null
done

for seed in 127 131 137; do
	if [[ -e "$base/A-$seed" || -e "$base/B-$seed" ]]; then
		echo "P4 holdout seed was consumed: $seed" >&2
		exit 1
	fi
done

echo "V2-5 P4 verdict: FALSIFIED with complete links 1-5 and exact-zero link 6"
