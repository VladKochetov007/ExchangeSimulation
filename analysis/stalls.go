package analysis

// StallStats describes execution outcomes for a desk that splits parent orders.
//
// A stall is a parent that reaches its horizon having filled nothing. It is not
// a slow execution: it costs a fixed block of the desk's time and crowds out
// the parents that would otherwise have run in it, which is why the horizon
// consumed by stalls predicts the loss in parent throughput.
type StallStats struct {
	Parents        int
	Filled         int
	Stalled        int
	ZeroFill       int
	StallSeconds   float64
	HorizonSeconds float64
	Sides          map[string]int
}

// StallFraction is the share of the desks' combined horizon spent in stalls.
func (s StallStats) StallFraction() float64 {
	if s.HorizonSeconds <= 0 {
		return 0
	}
	return s.StallSeconds / s.HorizonSeconds
}

// StallOptions configures what counts as a stall and how much desk time exists.
type StallOptions struct {
	// HorizonSeconds is the parent duration at which a parent is abandoned.
	HorizonSeconds float64
	// Desks and RunSeconds give the denominator for StallFraction.
	Desks      int
	RunSeconds float64
}

// Stalls summarises the run's recorded parent orders.
func (r *Run) Stalls(opts StallOptions) StallStats {
	horizon := opts.HorizonSeconds
	if horizon <= 0 {
		horizon = 900
	}
	stats := StallStats{Sides: map[string]int{}, HorizonSeconds: float64(opts.Desks) * opts.RunSeconds}
	for _, parent := range r.Report.Metaorders {
		stats.Parents++
		if parent.FilledQty > 0 {
			stats.Filled++
		} else {
			stats.ZeroFill++
		}
		if parent.EndTimestamp == 0 {
			continue
		}
		duration := float64(parent.EndTimestamp-parent.StartTimestamp) / 1e9
		// A parent within a second of the horizon reached it: the abandon check
		// runs on the desk's tick, so the recorded duration lands just under.
		if duration >= horizon-1 {
			stats.Stalled++
			stats.StallSeconds += duration
			stats.Sides[parent.Side]++
		}
	}
	return stats
}

// AdmittedByRole totals admitted requests for a participant class.
func (r *Run) AdmittedByRole(role string) int64 {
	var total int64
	for _, budget := range r.Report.RequestBudgets {
		if RoleGroup(budget.Role) == role {
			total += budget.Admitted
		}
	}
	return total
}
