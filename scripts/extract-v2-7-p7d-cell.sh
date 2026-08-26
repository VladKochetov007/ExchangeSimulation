#!/usr/bin/env bash
# Fail-closed P7d extraction. The shared extractor retains the historical
# P7a/P7b/P7c contracts; this wrapper selects the immutable P7d contract.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 CELL_DIR" >&2
  exit 2
fi
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
P7_DISTRESS_PROTOCOL=p7d exec "$root_dir/scripts/extract-v2-7-p7a-cell.sh" "$1"
