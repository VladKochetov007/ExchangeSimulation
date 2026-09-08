#!/usr/bin/env bash

# Select only the committed SV1 contract entry points. The caller may choose
# the historical SV1 namespace or one of the explicitly registered successor
# namespaces, but cannot inject an arbitrary shell file into a
# provenance-sensitive runner.
v2_r2_select_sv1_contract() {
	[[ $# -eq 1 ]] || return 1
	local root_dir=$1 requested=${V2_R2_SV1_CONTRACT_SCRIPT:-} historical successor_cdf successor_strict
	historical="$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
	successor_cdf="$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
	successor_strict="$root_dir/scripts/v2-r2-sv1c-24h-contract.sh"
	if [[ -z "$requested" ]]; then
		requested=$historical
	fi
	[[ "$requested" == "$historical" || "$requested" == "$successor_cdf" || "$requested" == "$successor_strict" ]] || return 1
	[[ -f "$requested" && ! -L "$requested" ]] || return 1
	[[ "$(realpath -e -- "$requested")" == "$requested" ]] || return 1
	printf '%s\n' "$requested"
}

v2_r2_is_successor_candidate() {
	case "${v2_r2_sv1_candidate_id:-}" in
		V2-R2-SV1B-*|V2-R2-SV1C-*) return 0 ;;
		*) return 1 ;;
	esac
}
