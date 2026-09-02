#!/usr/bin/env bash
# Extract one SV1B cell under its own candidate contract.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
V2_R2_EXTRACTOR_VARIANT=sv1 exec "$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" "$@"
