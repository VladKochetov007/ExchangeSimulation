#!/usr/bin/env bash
# Run one immutable V2-4 L1-P2 factorial cell. A partial directory is never
# evidence; final greeks.json and latency.json are the only completion sentinels.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 A|B|C|D 101|103 [multivenue-binary]" >&2
	exit 2
fi

arm=$1
seed=$2
case "$arm" in A|B|C|D) ;; *) echo "unknown L1-P2 arm: $arm" >&2; exit 2 ;; esac
case "$seed" in 101|103) ;; *) echo "unregistered L1-P2 seed: $seed" >&2; exit 2 ;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-4-l1p2/$arm-$seed.json"
output="$root_dir/research/artifacts/v2-4-l1p2/$arm/seed-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

if [[ ! -f "$config" ]]; then
	echo "missing rendered config: $config" >&2
	exit 1
fi
if [[ ! -x "$binary" ]]; then
	echo "missing executable multivenue binary: $binary" >&2
	exit 1
fi
if [[ -e "$output" ]]; then
	echo "refusing to overwrite L1-P2 evidence directory: $output" >&2
	exit 1
fi

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg arm "$arm" \
	--argjson seed "$seed" \
	--argjson liability_phase_nanos "$(jq '.cdf_liability_hedger.decision_phase_offset' "$config")" \
	--argjson noise_phase_nanos "$(jq '.noise_flow_decision_phase_offset' "$config")" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	'{
	  experiment_id: ("v2-4-l1p2-" + $arm + "-seed-" + ($seed|tostring)),
	  arm: $arm,
	  seed: $seed,
	  liability_decision_phase_offset_nanos: $liability_phase_nanos,
	  noise_flow_decision_phase_offset_nanos: $noise_phase_nanos,
	  simulated_horizon: "30m0s",
	  config_sha256: $config_sha256,
	  binary_sha256: $binary_sha256,
	  git_revision: $git_revision,
	  go_version: $go_version,
	  gomaxprocs: $gomaxprocs,
	  output_dir: $output_dir,
	  command: ["multivenue", "-config", "run-config.json", "-duration", "30m", "-log-mode", "full"],
	  raw_log_policy: "retain until every L1-P2 receipt, evidence-artifact, liability-policy/phase, noise-phase, exact-gap, fill-transition, and non-collapse contract passes review"
	}' >"$output/run-metadata.json"

exec "$binary" -config "$config" -duration 30m -logdir "$output" -log-mode full
