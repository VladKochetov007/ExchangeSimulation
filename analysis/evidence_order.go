package analysis

// evidenceOrder identifies a persisted record well enough to establish
// causality. Within one file the persisted sequence is authoritative, and a
// backdated record cannot be moved before an earlier sequence position. Across
// files simulated time orders records; records at one timestamp are
// intentionally ambiguous because the logger does not persist a global order.
type evidenceOrder struct {
	timestamp int64
	file      string
	ordinal   int64
}

func evidenceAfter(use, prerequisite evidenceOrder) bool {
	if prerequisite.timestamp > use.timestamp {
		return false
	}
	if use.file == prerequisite.file {
		return prerequisite.ordinal < use.ordinal
	}
	return use.timestamp > prerequisite.timestamp
}

func evidenceBefore(left, right evidenceOrder) bool {
	if left.timestamp != right.timestamp {
		return left.timestamp < right.timestamp
	}
	if left.file != right.file {
		return left.file < right.file
	}
	return left.ordinal < right.ordinal
}

func latestCausalPrerequisite(prerequisites []evidenceOrder, use evidenceOrder) (evidenceOrder, bool) {
	var latest evidenceOrder
	found := false
	for _, prerequisite := range prerequisites {
		if !evidenceAfter(use, prerequisite) {
			continue
		}
		if !found || evidenceBefore(latest, prerequisite) {
			latest = prerequisite
			found = true
		}
	}
	return latest, found
}
