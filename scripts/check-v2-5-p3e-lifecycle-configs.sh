#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p3e"
deadline=1736038805000000000

for seed in 107 109; do
	a="$config_dir/lifecycle-A-$seed.json"
	b="$config_dir/lifecycle-B-$seed.json"

	jq -e --argjson seed "$seed" '
	  .seed == $seed and
	  .log_mode == "full" and
	  .record_market_data_receipts == true and
	  .record_term_carry_decisions == true and
	  (.term_carry_allocator | has("passive_exit") | not)
	' "$a" >/dev/null
	jq -e --argjson seed "$seed" --argjson deadline "$deadline" '
	  .seed == $seed and
	  .log_mode == "full" and
	  .record_market_data_receipts == true and
	  .record_term_carry_decisions == true and
	  .term_carry_allocator.passive_exit == {
	    slice_qty: 100000,
	    deadline_at_nano: $deadline
	  }
	' "$b" >/dev/null

	a_economic=$(jq -cS 'del(.experiment_id, .description)' "$a")
	b_without_treatment=$(jq -cS 'del(.experiment_id, .description, .term_carry_allocator.passive_exit)' "$b")
	if [[ "$a_economic" != "$b_without_treatment" ]]; then
		echo "seed $seed has an undeclared paired config delta" >&2
		exit 1
	fi
done

seed_107=$(jq -cS 'del(.experiment_id, .description, .seed, .term_carry_allocator.passive_exit)' "$config_dir/lifecycle-B-107.json")
seed_109=$(jq -cS 'del(.experiment_id, .description, .seed, .term_carry_allocator.passive_exit)' "$config_dir/lifecycle-B-109.json")
if [[ "$seed_107" != "$seed_109" ]]; then
	echo "seed cells differ beyond seed and provenance metadata" >&2
	exit 1
fi

echo "V2-5 P3e lifecycle configs: paired structural delta verified"
