#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
: "${P3E_CELL:?set P3E_CELL to one registered lifecycle cell}"

P3E_EXPERIMENT_MODE=lifecycle exec "$root_dir/scripts/extract-v2-5-p3e-metrics.sh"
