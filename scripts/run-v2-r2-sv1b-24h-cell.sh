#!/usr/bin/env bash
# Run one registered SV1B cell through the shared fail-closed runner.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
exec "$root_dir/scripts/run-v2-r2-sv1-24h-cell.sh" "$@"
