(
	($all_cells_valid and $all_measurements_valid and
		$all_treatment_terminal_valid and $all_treatment_survival_valid and
		$all_cdf_contract_valid and $all_anticheating_valid and
		$all_paired_effect_valid and $all_paired_effect_identified) as $viable |
	($all_cells_valid and $all_measurements_valid and
		$all_cdf_contract_valid and $all_anticheating_valid and
		$all_paired_effect_valid) as $evidence_valid |
	{
		viable: $viable,
		evidence_valid: $evidence_valid,
		status: (if $viable then "VIABLE_DEVELOPMENT_CANDIDATE"
			elif $evidence_valid then "NON-VIABLE_AT_24H_MARKET_SURVIVAL_GATE"
			else "INVALID_DEVELOPMENT_EVIDENCE"
			end)
	}
)
