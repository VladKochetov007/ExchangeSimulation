#!/usr/bin/env bash
# SV1 adapter for deterministic raw-evidence re-extraction and comparison.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
V2_R2_EXTRACTOR_VARIANT=sv1 exec "$root_dir/scripts/verify-v2-integrated-longrun-r2-cell.sh" "$@"
