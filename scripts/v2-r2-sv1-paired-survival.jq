def aggregate_empty_side_share:
	([.venues[] | .snapshots] | add) as $snapshots |
	([.venues[] | .empty_side_snapshots] | add) as $empty |
	if ($snapshots | type) == "number" and $snapshots > 0
	then ($empty / $snapshots)
	else null
	end;

def maximum_window_empty_side_share:
	([.window_metrics[] | .empty_side_share] | map(select(type == "number"))) as $shares |
	if ($shares | length) > 0 then ($shares | max) else null end;

($treatment[0]) as $treatment_summary |
($control[0]) as $control_summary |
($treatment_summary | aggregate_empty_side_share) as $treatment_share |
($control_summary | aggregate_empty_side_share) as $control_share |
($treatment_summary | maximum_window_empty_side_share) as $treatment_max_window_share |
($control_summary | maximum_window_empty_side_share) as $control_max_window_share |
{
	schema_version: 1,
	contract: $contract,
	seed: $seed,
	measurement: {
		treatment_valid: ($treatment_summary.schema_version == 1 and ($treatment_summary.predicates | type) == "object" and ($treatment_share | type) == "number"),
		control_valid: ($control_summary.schema_version == 1 and ($control_summary.predicates | type) == "object" and ($control_share | type) == "number")
	},
	treatment: {
		aggregate_empty_side_share: $treatment_share,
		maximum_window_empty_side_share: $treatment_max_window_share,
		survival_predicate: ($treatment_summary.predicates.aggregate_two_sided_98pct == true)
	},
	control: {
		aggregate_empty_side_share: $control_share,
		maximum_window_empty_side_share: $control_max_window_share,
		survival_predicate: ($control_summary.predicates.aggregate_two_sided_98pct == true)
	},
	effect: {
		aggregate_empty_side_share_reduction: ($control_share - $treatment_share),
		maximum_window_empty_side_share_reduction: ($control_max_window_share - $treatment_max_window_share)
	},
	predicates: {
		matched_measurements_valid: ($treatment_summary.schema_version == 1 and $control_summary.schema_version == 1 and ($treatment_share | type) == "number" and ($control_share | type) == "number"),
		treatment_not_worse: (($treatment_share | type) == "number" and ($control_share | type) == "number" and $treatment_share <= $control_share),
		strict_aggregate_reduction: (($treatment_share | type) == "number" and ($control_share | type) == "number" and $treatment_share < $control_share)
	}
}
