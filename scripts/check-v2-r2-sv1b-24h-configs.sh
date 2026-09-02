#!/usr/bin/env bash
# Validate only the immutable SV1B configuration namespace.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
exec "$root_dir/scripts/check-v2-r2-sv1-24h-configs.sh" "$@"
