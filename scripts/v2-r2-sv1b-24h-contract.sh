#!/usr/bin/env bash
# SV1B namespace contract. It reuses the accepted SV1 evidence primitives
# while giving the fresh CDF successor its own identities and storage roots.
set -euo pipefail

source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1b-24h-development-20260902-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1b-24h-development-20260902-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1b-24h-development.lock"
v2_r2_sv1_candidate_id="V2-R2-SV1B-24H-CDF-LIQUIDITY"
v2_r2_sv1_require_candidate_metadata=true
v2_r2_sv1_require_generator_metadata=true
v2_r2_sv1_candidate_contract_version="v2-r2-sv1b-24h-candidate-v3"
v2_r2_sv1_scorer_contract="v2-r2-sv1b-24h-development-scorer-v3"
v2_r2_sv1_survival_contract="v2-r2-sv1b-24h-survival-side-availability-v2"
v2_r2_sv1_paired_effect_contract="v2-r2-sv1b-24h-paired-survival-effect-v1"
v2_r2_sv1_parity_contract="v2-r2-sv1b-24h-parity-v1"
v2_r2_sv1_predecessor_id="V2-R2-SV1"
v2_r2_sv1_runner_contract="v2-r2-sv1b-24h-runner-v3"
v2_r2_sv1_require_terminal_outcome=true
v2_r2_sv1_completion_sentinels='["greeks.json", "latency.json", "terminal-outcome.json"]'
v2_r2_sv1_require_no_replacement_withdrawal=true
v2_r2_sv1_experiment_prefix="v2-r2-sv1b-24h"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1b-24h-config-provenance-v3"
v2_r2_sv1_config_dir="$root_dir/research/configs/v2-r2-sv1b-24h"
v2_r2_sv1_config_provenance_manifest="$root_dir/research/v2-r2-sv1b-24h-config-provenance.json"
v2_r2_sv1_seeds=(643 647 653)
v2_r2_sv1_parity_seed=643
v2_r2_sv1_source_config_names=(dev-607.json dev-607-none.json)
v2_r2_sv1_activation_config="$root_dir/research/configs/v2-r2-sv1b/activation-643.json"
v2_r2_sv1_activation_control_config="$root_dir/research/configs/v2-r2-sv1b/activation-643-control.json"
v2_r2_sv1_activation_seed=643
v2_r2_sv1_run_hypothesis_id="V2-R2-SV1B-24H-CDF-LIQUIDITY"
v2_r2_sv1_activation_hypothesis_prefix="V2-R2-SV1B-CDF-LIQUIDITY"
v2_r2_sv1_activation_contract="v2-r2-sv1b-activation-provenance-v2"
v2_r2_sv1_activation_pair_contract="v2-r2-sv1b-activation-pair-v2"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1b-activation"
v2_r2_sv1_capacity_attestation="/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260902-v1.json"
v2_r2_sv1_capacity_probe_prefix="v2-r2-sv1b-24h-capacity"
v2_r2_sv1_capacity_probe_contract="v2-r2-sv1b-24h-capacity-probe-v1"
v2_r2_capacity_probe_cell="treatment-643"

# Keep SV1B's exact Go toolchain requirement and all fail-closed activity
# predicates from the accepted SV1 contract.
