#!/usr/bin/env bash
# Run exactly one V2-3 P0 replacement cell. Attempt 0 is retained separately
# as invalidated evidence and is never overwritten in place.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 A|B|C 101|103 [multivenue-binary]" >&2
	exit 2
fi

arm=$1
seed=$2
case "$arm" in A|B|C) ;; *) echo "unknown P0 arm: $arm" >&2; exit 2;; esac
case "$seed" in 101|103) ;; *) echo "unregistered P0 seed: $seed" >&2; exit 2;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-3-p0-r1/$arm-$seed.json"
output="$root_dir/research/artifacts/v2-3-p0-r1/$arm/seed-$seed"
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
	echo "refusing to overwrite P0 evidence directory: $output" >&2
	exit 1
fi

mkdir -p "$output"
cp "$config" "$output/run-config.json"
jq -n \
	--arg arm "$arm" \
	--argjson seed "$seed" \
	--arg config_sha256 "$(sha256sum "$config" | awk '{print $1}')" \
	--arg binary_sha256 "$(sha256sum "$binary" | awk '{print $1}')" \
	--arg git_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg go_version "$(go version)" \
	--arg gomaxprocs "${GOMAXPROCS:-default}" \
	--arg output_dir "$output" \
	'{
	  experiment_id: ("v2-3-p0-r1-" + $arm + "-seed-" + ($seed|tostring)),
	  arm: $arm,
	  seed: $seed,
	  simulated_horizon: "5m0s",
	  config_sha256: $config_sha256,
	  binary_sha256: $binary_sha256,
	  git_revision: $git_revision,
	  go_version: $go_version,
	  gomaxprocs: $gomaxprocs,
	  output_dir: $output_dir,
	  command: ["multivenue", "-config", "run-config.json", "-duration", "5m", "-log-mode", "full"],
		  raw_log_policy: "retain until every P0-r1 activation and viability artifact passes review"
	}' >"$output/run-metadata.json"

exec "$binary" -config "$config" -duration 5m -logdir "$output" -log-mode full
