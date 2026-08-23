#!/usr/bin/env bash
# Run one immutable V2-2b cell. It refuses an existing evidence directory so
# a failed or completed cell can never be silently overwritten.
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
	echo "usage: $0 I0R0|I1R0|I0R1|I1R1 101|103 [multivenue-binary]" >&2
	exit 2
fi

arm=$1
seed=$2
case "$arm" in I0R0|I1R0|I0R1|I1R1) ;; *) echo "unknown arm: $arm" >&2; exit 2;; esac
case "$seed" in 101|103) ;; *) echo "unregistered seed: $seed" >&2; exit 2;; esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config="$root_dir/research/configs/v2-2b/$arm-$seed.json"
output="$root_dir/research/artifacts/v2-2b/$arm/seed-$seed"
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
	echo "refusing to overwrite evidence directory: $output" >&2
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
	  experiment_id: ("v2-2b-" + $arm + "-seed-" + ($seed|tostring)),
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
	  raw_log_policy: "retain until V2-2b smoke measurement contract passes"
	}' >"$output/run-metadata.json"

exec "$binary" -config "$config" -duration 5m -logdir "$output" -log-mode full
