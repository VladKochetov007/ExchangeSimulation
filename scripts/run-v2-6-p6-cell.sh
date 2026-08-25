#!/usr/bin/env bash
# Run one immutable V2-6 P6 staged-options cell. Only final greeks.json and
# latency.json make a completed world; a process exit or partial directory is
# never evidence.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 O0|O1|O2|O3|O4 211|213|223|227|229 [multivenue-binary]" >&2
	exit 2
fi
stage=$1
seed=$2
case "$stage" in O0|O1|O2|O3|O4) ;; *) echo "unknown P6 stage: $stage" >&2; exit 2 ;; esac
case "$seed" in 211|213|223|227|229) ;; *) echo "unregistered P6 seed: $seed" >&2; exit 2 ;; esac
if [[ "$seed" == 223 || "$seed" == 227 || "$seed" == 229 ]]; then
	if [[ "${P6_ALLOW_HOLDOUT:-}" != 1 ]]; then
		echo "refusing P6 holdout before development promotion: set P6_ALLOW_HOLDOUT=1" >&2
		exit 1
	fi
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-6-p6/$stage-$seed.json"
output="$root_dir/research/artifacts/v2-6-p6/$stage-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

if [[ ! -s "$config" || ! -x "$binary" ]]; then
	echo "missing P6 config or executable: $config / $binary" >&2
	exit 1
fi
if [[ -e "$output" ]]; then
	echo "refusing to overwrite P6 evidence directory: $output" >&2
	exit 1
fi
"$root_dir/scripts/check-v2-6-p6-configs.sh" >/dev/null

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg stage "$stage" \
	--argjson seed "$seed" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	'{
	  experiment_id: ("v2-6-p6-options-" + $stage + "-seed-" + ($seed|tostring)),
	  hypothesis_id: "V2-6-P6-OPTIONS",
	  stage: $stage, seed: $seed, simulated_horizon: "8h0m0s",
	  config_sha256: $config_sha256, binary_sha256: $binary_sha256,
	  git_revision: $git_revision, go_version: $go_version,
	  gomaxprocs: $gomaxprocs, output_dir: $output,
	  command: ["multivenue", "-config", "run-config.json", "-duration", "8h", "-log-mode", "full"],
	  completion_sentinels: ["greeks.json", "latency.json"],
	  raw_log_policy: "retain until every registered P6 evidence contract passes"
	}' >"$output/run-metadata.json"

"$binary" -config "$config" -duration 8h -logdir "$output" -log-mode full

if [[ ! -s "$output/greeks.json" || ! -s "$output/latency.json" ]]; then
	echo "P6 process exited without both completion sentinels: $output" >&2
	exit 1
fi
echo "completed P6 cell: $output"

