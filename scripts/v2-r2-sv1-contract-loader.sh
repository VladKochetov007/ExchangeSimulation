#!/usr/bin/env bash

# Select only the two committed SV1 contract entry points. The caller may
# choose the historical SV1 namespace or the fresh SV1B namespace, but cannot
# inject an arbitrary shell file into a provenance-sensitive runner.
v2_r2_select_sv1_contract() {
	[[ $# -eq 1 ]] || return 1
	local root_dir=$1 requested=${V2_R2_SV1_CONTRACT_SCRIPT:-} historical successor
	historical="$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
	successor="$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
	if [[ -z "$requested" ]]; then
		requested=$historical
	fi
	[[ "$requested" == "$historical" || "$requested" == "$successor" ]] || return 1
	[[ -f "$requested" && ! -L "$requested" ]] || return 1
	[[ "$(realpath -e -- "$requested")" == "$requested" ]] || return 1
	printf '%s\n' "$requested"
}
