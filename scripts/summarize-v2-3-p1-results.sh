#!/usr/bin/env bash
# Compose P1's scored input from its retained evidence contract. This summary
# distinguishes deterministic policy activation from descriptive market rows;
# it never converts a price or viability statistic into an activation pass.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p1"
output="$artifact_dir/p1-summary.json"
scratch=$(mktemp -d -t v2-3-p1-summary-XXXXXX)

required=(
	observationreceipts.json evidenceartifacthash.json makerquotesize.json
	viability.json spot-viability.json maker-net-delta.json
	spot-trade-price-ratio.json analysis-metadata.json
)

for arm in A B; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		for name in "${required[@]}"; do
			if [[ ! -s "$cell/$name" ]]; then
				echo "missing required P1 artifact: $cell/$name" >&2
				exit 1
			fi
		done

		jq -n \
			--arg arm "$arm" --argjson seed "$seed" \
			--slurpfile metadata "$cell/run-metadata.json" \
			--slurpfile config "$cell/run-config.json" \
			--slurpfile receipt "$cell/observationreceipts.json" \
			--slurpfile artifact "$cell/evidenceartifacthash.json" \
			--slurpfile quote_size "$cell/makerquotesize.json" \
			--slurpfile delta "$cell/maker-net-delta.json" \
			--slurpfile viability "$cell/spot-viability.json" \
			--slurpfile price_ratio "$cell/spot-trade-price-ratio.json" \
			'
			def spot_books:
			  [ $viability[0].windows[] ] | group_by(.symbol) |
			  map({
			    symbol: .[0].symbol,
			    venues: (map(.venue_id) | unique),
			    snapshots: (map(.snapshots) | add),
			    empty_side_snapshots: (map(.empty_side_snapshots) | add),
			    two_sided_share: (1 - ((map(.empty_side_snapshots) | add) / (map(.snapshots) | add))),
			    trades: (map(.trades) | add),
			    volume: (map(.volume) | add)
			  }) | sort_by(.symbol);
			def integrity($q):
			  ($q.missing_outcomes == 0 and
			   $q.duplicate_outcomes == 0 and
			   $q.duplicate_decision_sides == 0 and
			   $q.decision_field_mismatches == 0 and
			   $q.outcome_field_mismatches == 0 and
			   $q.invalid_decision_records == 0 and
			   $q.invalid_censor_records == 0 and
			   $q.censored_outcome_deliveries == 0 and
			   $q.wrong_direction_size_skew == 0);
			def per_maker_active($q):
			  ($q.maker_buckets | length == 24 and
			   ([ $q.maker_buckets[] | (.decisions > 0 and ((.accepted + .rejected) > 0)) ] | all));
			def spot_viable($books):
			  ($books | length == 3 and
			   ([ $books[] | (.venues == ["central", "north", "south"] and .snapshots > 0 and .two_sided_share > 0 and .trades > 0 and .volume > 0) ] | all));
			($quote_size[0].result) as $q |
			(spot_books) as $books |
			{
			  arm: $arm,
			  seed: $seed,
			  provenance: {
			    config_sha256: $metadata[0].config_sha256,
			    binary_sha256: $metadata[0].binary_sha256,
			    git_revision: $metadata[0].git_revision,
			    gomaxprocs: $metadata[0].gomaxprocs,
			    evidence: {events: $artifact[0].result.events, digest: $artifact[0].result.digest}
			  },
			  policy: {
			    size_skew_bps: $config[0].spot_stoikov_inventory_size_skew_bps,
			    post_only: $config[0].spot_passive_maker_post_only,
			    cancel_before_replace: $config[0].spot_passive_maker_cancel_before_replace
			  },
			  information_boundary: {
			    valid: $receipt[0].result.valid,
			    schedules: $receipt[0].result.schedules,
			    receipts: $receipt[0].result.receipts,
			    decisions: $receipt[0].result.decisions,
			    future_decision_use: $receipt[0].result.future_decision_use,
			    bad_decision_frontier: $receipt[0].result.bad_decision_frontier
			  },
			  activation: $q,
			  maker_net_delta: $delta[0].aggregate,
			  spot_books: $books,
			  executed_trade_price_ratios: $price_ratio[0].rows,
			  gates: {
			    information_boundary_valid: $receipt[0].result.valid,
			    decision_request_integrity: integrity($q),
			    every_scoped_venue_maker_active: per_maker_active($q),
			    scoped_spot_viable: spot_viable($books),
			    p0c_passive_refresh_preserved: ($config[0].spot_passive_maker_post_only == true and $config[0].spot_passive_maker_cancel_before_replace == true),
			    terminal_censoring_explicit: ($q.horizon_censored_sides > 0 and $q.censored_outcome_deliveries == 0)
			  }
			}' >"$scratch/$arm-$seed.json"
	done
done

jq -s '
  def row($arm; $seed): .[] | select(.arm == $arm and .seed == $seed);
  def book($row; $symbol): $row.spot_books[] | select(.symbol == $symbol);
  def delta($to; $from): {
    decisions: ($to.activation.decisions - $from.activation.decisions),
    nonzero_risk_decisions: ($to.activation.nonzero_risk_decisions - $from.activation.nonzero_risk_decisions),
    nonzero_adjustments: ($to.activation.nonzero_adjustments - $from.activation.nonzero_adjustments),
    accepted: ($to.activation.accepted - $from.activation.accepted),
    rejected: ($to.activation.rejected - $from.activation.rejected),
    mean_abs_maker_net_delta: ($to.maker_net_delta.mean_abs_net_delta - $from.maker_net_delta.mean_abs_net_delta),
    mean_lag1_maker_net_delta: ($to.maker_net_delta.mean_lag1_autocorrelation - $from.maker_net_delta.mean_lag1_autocorrelation),
    spot_books: ["ABC/USD", "CDF/USD", "ABC/CDF"] | map(. as $symbol |
      (book($to; $symbol)) as $t | (book($from; $symbol)) as $f |
      {symbol: $symbol, two_sided_share: ($t.two_sided_share - $f.two_sided_share), trades: ($t.trades - $f.trades), volume: ($t.volume - $f.volume)}
    )
  };
  . as $rows |
  {
    schema_version: 1,
    experiment: "V2-3-P1",
    comparison_contract: {
      b_minus_a: "inventory-asymmetric displayed quote size with P0-R1 C passive refresh held fixed",
      primary: "decision-time signed size association; market rows descriptive only",
      seeds: [101, 103]
    },
    rows: $rows,
    paired_deltas: [101, 103] | map(. as $seed |
      ($rows | row("A"; $seed)) as $a |
      ($rows | row("B"; $seed)) as $b |
      {seed: $seed, b_minus_a: delta($b; $a)}
    ),
    gates: {
      all_cells_complete: ([ $rows[].gates | to_entries[] | select(.value != true) ] | length == 0),
      common_binary: ([ $rows[].provenance.binary_sha256 ] | unique | length == 1),
      common_simulator_revision: ([ $rows[].provenance.git_revision ] | unique | length == 1)
    }
  }
' "$scratch"/*.json >"$output.tmp"

if ! jq -e '.gates.all_cells_complete and .gates.common_binary and .gates.common_simulator_revision' "$output.tmp" >/dev/null; then
	echo "P1 required gate failed; diagnostic summary retained" >&2
	mv "$output.tmp" "$output"
	exit 1
fi
mv "$output.tmp" "$output"
printf 'wrote %s\n' "$output"
