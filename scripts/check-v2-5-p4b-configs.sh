#!/usr/bin/env bash
# Verify the immutable P4b config family without reading outcomes.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p4b"
development_seeds=(401 409)
holdout_seeds=(419 421 431)
all_seeds=("${development_seeds[@]}" "${holdout_seeds[@]}")

declare -A expected_sha256=(
	[A-401]=cd9b377fa3efe696ed816e8448eab7e6df7c5b36614fdf03c7f901fb28df19f8
	[A-409]=a9e887e20c9ee860e41796c05e180ebaa22fb4cb7ba96d83a5851decc0ad3778
	[A-419]=69d7c0c3f49fbc2cb28b87f9c9815720becd11e39be13a5bcc2923b7befbcfb8
	[A-421]=ec17c1a3b63af1009072d70aba0173660bf125db13cdf1fb5bbec065adc340a1
	[A-431]=e740edec6ceff03a2a36d1d9ac4ac313a13a5bae39dec69adf1864cd8c85ede3
	[B-401]=a39d721741a047a9e1f3866ba75089b5ddb8012bda7b150c4c5077db598849f0
	[B-409]=6cb26da55db930cca89900eb2ce1656f219a93ad10be34a2900d03193c648804
	[B-419]=e8a7dc1c1a369895319fce752850cbd59f282c6e461554daa31d2cc9c6b23a2d
	[B-421]=b45b9db022d641bcd40a95ef57c81a46e7559e9bcb4a7b4b47cc4c75c63222ee
	[B-431]=1c917cc9f4dc61a777841baecdc9ea13d3b3eb6149ee49a8d59f098af8a379b5
)

normalize_pair() {
	jq -S 'del(.experiment_id, .description, .funding_max_rate_bps)' "$1"
}

normalize_seed() {
	jq -S 'del(.experiment_id, .description, .seed)' "$1"
}

for seed in "${all_seeds[@]}"; do
	for arm in A B; do
		cell="$arm-$seed"
		config="$config_dir/$cell.json"
		test -s "$config"
		actual=$(sha256sum "$config" | awk '{print $1}')
		test "$actual" = "${expected_sha256[$cell]}"
	done

	jq -e --argjson seed "$seed" '
		.seed == $seed and .funding_max_rate_bps == 1 and
		.log_mode == "full" and .strict_population_accounting == true and
		.record_market_data_receipts == true and
		.record_perp_exposure_hedger_decisions == true and
		.record_term_carry_decisions == true and
		.market_data_receipt_roles == ["term_carry_allocator", "perp_exposure_hedger"] and
		.perp_exposure_hedger == {
			enabled: true, symbol: "ABC-PERP", decision_interval: 2000000000,
			exposure_interval: 10000000000, exposure_step_qty: 10000000,
			max_abs_exposure: 100000000, max_request_qty: 10000000,
			tick_size: 1000000, initial_quote_balance: 20000000000000,
			initial_margin: 10000000000000
		} and .latency_profiles.perp_exposure_hedger == {
			model: "constant", delay: 20000000, market_data_scale: 2
		}
	' "$config_dir/A-$seed.json" >/dev/null
	jq -e --argjson seed "$seed" '
		.seed == $seed and .funding_max_rate_bps == 75 and
		.log_mode == "full" and .strict_population_accounting == true and
		.record_market_data_receipts == true and
		.record_perp_exposure_hedger_decisions == true and
		.record_term_carry_decisions == true and
		.market_data_receipt_roles == ["term_carry_allocator", "perp_exposure_hedger"]
	' "$config_dir/B-$seed.json" >/dev/null

	diff -u <(normalize_pair "$config_dir/A-$seed.json") <(normalize_pair "$config_dir/B-$seed.json")
done

for arm in A B; do
	base="$config_dir/$arm-401.json"
	for seed in 409 419 421 431; do
		diff -u <(normalize_seed "$base") <(normalize_seed "$config_dir/$arm-$seed.json")
	done
done

printf 'V2-5 P4b configs: exact funding-cap pair and fixed exposure condition verified\n'
