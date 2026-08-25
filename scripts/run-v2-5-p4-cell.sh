#!/usr/bin/env bash
# Run one immutable V2-5 P4 cell. Final greeks.json and latency.json are the
# only completion sentinels; a process exit or partial directory is not evidence.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 A|B 107|109|127|131|137 [multivenue-binary]" >&2
	exit 2
fi

arm=$1
seed=$2
case "$arm" in A|B) ;; *) echo "unknown P4 arm: $arm" >&2; exit 2 ;; esac
case "$seed" in 107|109|127|131|137) ;; *) echo "unregistered P4 seed: $seed" >&2; exit 2 ;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-5-p4/$arm-$seed.json"
output="$root_dir/research/artifacts/v2-5-p4/$arm-$seed"
binary=${3:-"$root_dir/bin/multivenue"}

if [[ ! -f "$config" ]]; then
	echo "missing immutable P4 config: $config" >&2
	exit 1
fi
if [[ ! -x "$binary" ]]; then
	echo "missing executable multivenue binary: $binary" >&2
	exit 1
fi
if [[ -e "$output" ]]; then
	echo "refusing to overwrite P4 evidence directory: $output" >&2
	exit 1
fi
"$root_dir/scripts/check-v2-5-p4-configs.sh" >/dev/null

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg arm "$arm" \
	--argjson seed "$seed" \
	--argjson funding_max_rate_bps "$(jq '.funding_max_rate_bps' "$config")" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	'{
	  experiment_id: ("v2-5-p4-funding-carry-" + $arm + "-seed-" + ($seed|tostring)),
	  hypothesis_id: "V2-5-P4-FUNDING-CARRY",
	  arm: $arm,
	  seed: $seed,
	  funding_max_rate_bps: $funding_max_rate_bps,
	  simulated_horizon: "98h0m0s",
	  config_sha256: $config_sha256,
	  binary_sha256: $binary_sha256,
	  git_revision: $git_revision,
	  go_version: $go_version,
	  gomaxprocs: $gomaxprocs,
	  output_dir: $output_dir,
	  command: ["multivenue", "-config", "run-config.json", "-duration", "98h", "-log-mode", "full"],
	  completion_sentinels: ["greeks.json", "latency.json"],
	  raw_log_policy: "retain until the exact P4 evidence contract and any promoted holdout contract pass"
	}' >"$output/run-metadata.json"

exec "$binary" -config "$config" -duration 98h -logdir "$output" -log-mode full
