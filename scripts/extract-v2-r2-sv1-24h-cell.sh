#!/usr/bin/env bash
# SV1 adapter for the shared fail-closed extractor. Historical extraction
# remains the default when the variant is not explicitly selected.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
V2_R2_EXTRACTOR_VARIANT=sv1 exec "$root_dir/scripts/extract-v2-integrated-longrun-r2-cell.sh" "$@"
