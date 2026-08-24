#!/usr/bin/env bash
# Extract a P3 R1 cell contract without mixing it with invalid attempt-0 files.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec env P3_VARIANT=v2-3-p3-r1 "$root_dir/scripts/extract-v2-3-p3-metrics.sh" "$@"
