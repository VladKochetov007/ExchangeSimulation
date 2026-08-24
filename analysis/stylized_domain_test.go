package analysis

import "testing"

func TestLogReturnSeriesRetainsSignedDomainExclusions(t *testing.T) {
	tape := &TradeTape{
		Timestamps: []int64{0, int64(1e9), int64(2e9), int64(3e9), int64(4e9)},
		Prices:     []int64{100, 0, 100, -10, 100},
	}
	tests := []struct {
		name   string
		series LogReturnSeries
		pairs  int
		bad    int
	}{
		{name: "trade time", series: tape.LogReturnSeries(), pairs: 4, bad: 4},
		{name: "one second", series: tape.TimeSampledLogReturnSeries(int64(1e9)), pairs: 4, bad: 4},
		{name: "stride one", series: tape.StridedLogReturnSeries(1), pairs: 4, bad: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.series.CandidatePairs != test.pairs || test.series.UndefinedDomainPairs != test.bad || len(test.series.Returns) != 0 {
				t.Fatalf("series = %+v", test.series)
			}
		})
	}

	facts := tape.Facts(5)
	if facts.LogReturnPairs != 4 || facts.LogReturnUndefinedDomainPairs != 4 || facts.Sec1ReturnUndefinedDomainPairs != 4 || facts.Stride20UndefinedDomainPairs != 0 {
		t.Fatalf("facts domain accounting = %+v", facts)
	}
}
