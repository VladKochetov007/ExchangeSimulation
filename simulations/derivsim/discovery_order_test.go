package derivsim

import "testing"

func TestContractSetOrderedContracts(t *testing.T) {
	set := newContractSet("ABC/USD")
	for _, symbol := range []string{"ABC-2", "ABC-1", "ABC-3"} {
		set.contracts[symbol] = &Contract{Symbol: symbol}
	}
	got := set.orderedContracts()
	for i, want := range []string{"ABC-1", "ABC-2", "ABC-3"} {
		if got[i].Symbol != want {
			t.Fatalf("contract %d = %q, want %q", i, got[i].Symbol, want)
		}
	}
}
