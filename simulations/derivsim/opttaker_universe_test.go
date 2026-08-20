package derivsim

import "testing"

// A taker drawing uniformly from a mixed universe gives each book flow in
// proportion to how many contracts of its kind are listed, so a dated ladder
// beside a thirty-strike chain sees almost nothing. Restricting the universe
// is how a participant becomes dedicated to one kind of contract.
func TestOptionTakerUniverseCanBeRestrictedByContractType(t *testing.T) {
	defaults := &OptionTaker{cfg: OptionTakerConfig{}}
	if !defaults.tradesType("OPTION") {
		t.Error("the default universe excludes options")
	}
	if defaults.tradesType("FUTURE") {
		t.Error("the default universe includes futures without being asked")
	}

	withFutures := &OptionTaker{cfg: OptionTakerConfig{IncludeFutures: true}}
	if !withFutures.tradesType("FUTURE") || !withFutures.tradesType("OPTION") {
		t.Error("IncludeFutures did not widen the universe")
	}

	futuresOnly := &OptionTaker{cfg: OptionTakerConfig{ContractTypes: []string{"FUTURE"}}}
	if futuresOnly.tradesType("OPTION") {
		t.Error("an explicit type list did not exclude options")
	}
	if !futuresOnly.tradesType("FUTURE") {
		t.Error("an explicit type list excluded the type it named")
	}
	if futuresOnly.tradesType("PERPETUAL") {
		t.Error("an explicit type list admitted a type it did not name")
	}
}
