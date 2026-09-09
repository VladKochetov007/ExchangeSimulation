#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
contract="$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"
checker="$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh"
generator="$root_dir/scripts/render-v2-r2-sv1d-activation-configs.sh"
status_writer="$root_dir/scripts/v2-r2-sv1-activation-status.sh"

runner="$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh"
scorer="$root_dir/scripts/score-v2-r2-sv1d-activation.sh"
for file in "$contract" "$checker" "$generator" "$runner" "$scorer"; do
	[[ -f "$file" && -x "$file" ]] || { echo "SV1D activation contract script is not executable: $file" >&2; exit 1; }
done
[[ -f "$status_writer" && ! -L "$status_writer" ]] || { echo "SV1D activation status writer is missing or symlinked" >&2; exit 1; }

rg -F 'activation-659-treatment.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'activation-659-mode-off.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'activation-659-no-roster.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'quote_on_one_sided_local_book' "$generator" "$checker" >/dev/null
rg -F 'minimum_qualifying_qty' "$generator" "$checker" >/dev/null
rg -F 'registered_minimum_executable_qty' "$generator" "$checker" >/dev/null
rg -F 'holdouts_consumed: false' "$generator" >/dev/null
rg -F 'elastic_supplier_count == 8' "$checker" >/dev/null
rg -F 'treatment_mode_filter' "$checker" >/dev/null
rg -F 'v2_r2_sv1d_require_mode_pair_comparison' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null
rg -F 'v2_r2_sv1d_require_no_roster_diagnostic' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'process_group_rss_bytes' "$runner" >/dev/null
rg -F 'activation_analyzer_max_wall_seconds' "$contract" "$runner" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'simulator_stdout_sha256' "$status_writer" "$contract" "$runner" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'max_inventory_utilization' "$contract" "$root_dir/analysis/cdf_liquidity.go" >/dev/null
rg -F 'mode-off' "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'status --porcelain --untracked-files=all' "$generator" "$checker" >/dev/null
rg -F 'v2-r2-sv1-activation-status.sh' "$contract" "$generator" "$checker" >/dev/null
rg -F 'ask-only/positive-inventory' "$root_dir/research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-09.md" >/dev/null

if rg -F 'v2_r2_require_cdf_supplier_comparison "$comparison_record_path"' "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null; then
	echo "SV1D runner still uses the historical zero-roster comparison predicate" >&2
	exit 1
fi

if rg -F 'holdout-619' "$generator" "$checker" "$contract" >/dev/null; then
	echo "SV1D activation package names a holdout" >&2
	exit 1
fi
if rg -F 'capacity' "$generator" >/dev/null; then
	echo "SV1D activation config generator must not silently perform capacity work" >&2
	exit 1
fi

echo "SV1D activation config contract: PASS"
