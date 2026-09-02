#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner="$root_dir/scripts/run-v2-r2-sv1-activation-probe.sh"
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
temp_root=$(mktemp -d)
trap 'rm -rf -- "$temp_root"' EXIT

assert_rejected() {
	local output_root=$1
	local stdout_log="$temp_root/runner.stdout" stderr_log="$temp_root/runner.stderr"
	if V2_R2_SV1_ACTIVATION_ROOT="$output_root" "$runner" >"$stdout_log" 2>"$stderr_log"; then
		echo "activation runner accepted forbidden output root: $output_root" >&2
		cat "$stderr_log" >&2
		exit 1
	fi
}

assert_rejected "$root_dir/research/probe"
ln -s -- "$root_dir" "$temp_root/repository-link"
assert_rejected "$temp_root/repository-link/probe"

cdf_audit_fixture="$temp_root/cdfliquidity.json"
jq -n ' {
		run: "contract-fixture",
		result: {
			valid: true, supplier_count: 2, decision_count: 4, fill_count: 2,
			trading_supplier_count: 2, pnl_changing_supplier_count: 2,
			inventory_responsive_decision_count: 4, cancel_count: 1, withdraw_count: 1,
			max_borrowed: 0, supplier_volume_share: 0.2, supplier_depth_over_75_share: 0.1,
			venues: [
				{supplier_depth_over_75_fraction: 0.1},
				{supplier_depth_over_75_fraction: 0.1},
				{supplier_depth_over_75_fraction: 0.1}
			],
			suppliers: [
				{valid: true, fill_count: 1, pnl: 1, min_position: 0, max_position: 1,
				 inventory_responsive_decision_count: 2, max_observation_age_ns: 1,
				 max_borrowed: 0, borrow_event_count: 0, max_position: 1,
				 configured_max_position: 2, max_quote_qty: 1, configured_max_quote_qty: 2},
				{valid: true, fill_count: 1, pnl: -1, min_position: -1, max_position: 0,
				 inventory_responsive_decision_count: 2, max_observation_age_ns: 1,
				 max_borrowed: 0, borrow_event_count: 0, max_position: 1,
				 configured_max_position: 2, max_quote_qty: 1, configured_max_quote_qty: 2}
			]
		}
	}' >"$cdf_audit_fixture"
v2_r2_require_cdf_supplier_activation "$cdf_audit_fixture" 2 || {
	echo "valid CDF activity without borrowing was rejected" >&2
	exit 1
}
if jq '.result.suppliers[0].borrow_event_count = 1' "$cdf_audit_fixture" >"$temp_root/borrowed.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/borrowed.json" 2; then
	echo "unregistered CDF borrowing was accepted" >&2
	exit 1
fi
if jq '.result.pnl_changing_supplier_count = 0' "$cdf_audit_fixture" >"$temp_root/no-pnl.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/no-pnl.json" 2; then
	echo "non-PnL CDF activity was accepted" >&2
	exit 1
fi

echo "V2-R2-SV1 activation output boundary contract: pass"
