package analysis

import "testing"

func TestMeasureEcology(t *testing.T) {
	makeRow := func(venue string, id uint64, role string, equity int64) AccountRow {
		return AccountRow{VenueID: venue, ClientID: id, Role: role, Account: Account{Equity: equity}}
	}
	tests := []struct {
		name    string
		run     Run
		wantErr bool
		wantHHI float64
		want    []EcologyRole
	}{
		{
			name: "aggregates numbered roles and shares",
			run: Run{Report: Report{
				InitialAccounts:  []AccountRow{makeRow("north", 1, "maker_1", 100), makeRow("south", 1, "maker_2", 100), makeRow("north", 2, "taker_1", 200)},
				TerminalAccounts: []AccountRow{makeRow("north", 1, "maker_1", 150), makeRow("south", 1, "maker_2", 50), makeRow("north", 2, "taker_1", 200)},
			}},
			wantHHI: 0.5,
			want: []EcologyRole{
				{Role: "maker", Accounts: 2, InitialEquity: 200, TerminalEquity: 200, EquityReturn: 0, InitialWealthShare: 0.5, TerminalWealthShare: 0.5},
				{Role: "taker", Accounts: 1, InitialEquity: 200, TerminalEquity: 200, EquityReturn: 0, InitialWealthShare: 0.5, TerminalWealthShare: 0.5},
			},
		},
		{
			name: "rejects changed account population",
			run: Run{Report: Report{
				InitialAccounts:  []AccountRow{makeRow("north", 1, "maker_1", 100)},
				TerminalAccounts: []AccountRow{makeRow("north", 1, "maker_1", 100), makeRow("north", 2, "taker_1", 100)},
			}},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.run.MeasureEcology()
			if test.wantErr {
				if err == nil {
					t.Fatal("MeasureEcology succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("MeasureEcology: %v", err)
			}
			if got.InitialConcentrationHHI != test.wantHHI || got.TerminalConcentrationHHI != test.wantHHI {
				t.Fatalf("HHI = %g/%g, want %g", got.InitialConcentrationHHI, got.TerminalConcentrationHHI, test.wantHHI)
			}
			if len(got.Roles) != len(test.want) {
				t.Fatalf("roles = %d, want %d", len(got.Roles), len(test.want))
			}
			for i, want := range test.want {
				actual := got.Roles[i]
				if actual != want {
					t.Errorf("role %d = %+v, want %+v", i, actual, want)
				}
			}
		})
	}
}
