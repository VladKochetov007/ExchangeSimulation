package multivenue

import (
	"testing"
	"time"

	"exchange_sim/ratelimit"
)

func kindPlace() ratelimit.RequestKind        { return ratelimit.KindPlaceOrder }
func kindCancel() ratelimit.RequestKind       { return ratelimit.KindCancelOrder }
func kindQueryBalance() ratelimit.RequestKind { return ratelimit.KindQueryBalance }

func TestTiersMatchParticipantRolesByPrefix(t *testing.T) {
	participants := []Participant{
		{ClientID: 1, Role: "spot_maker_1"},
		{ClientID: 2, Role: "spot_maker_2"},
		{ClientID: 3, Role: "noise_flow_1"},
		{ClientID: 4, Role: "option_dealer_1"},
	}
	policy := buildRequestPolicy(map[string]RateLimitTier{
		"makers": {Roles: []string{"spot_maker"}, OrdersPer10s: 1},
		"flow":   {Roles: []string{"noise_flow"}, OrdersPer10s: 2},
	}, participants)
	if policy == nil {
		t.Fatal("no policy built from configured tiers")
	}

	// A maker gets one placement per ten seconds.
	if _, _, ok := policy.Admit(1, kindPlace(), 0); !ok {
		t.Fatal("maker refused its first placement")
	}
	if _, _, ok := policy.Admit(1, kindPlace(), 0); ok {
		t.Fatal("maker admitted beyond its tier")
	}
	// The second maker has its own allowance.
	if _, _, ok := policy.Admit(2, kindPlace(), 0); !ok {
		t.Fatal("a second maker shared the first's budget")
	}
	// Flow gets two.
	if _, _, ok := policy.Admit(3, kindPlace(), 0); !ok {
		t.Fatal("flow refused")
	}
	if _, _, ok := policy.Admit(3, kindPlace(), 0); !ok {
		t.Fatal("flow refused inside its larger budget")
	}
	// A participant matching no tier is unmetered.
	for i := 0; i < 50; i++ {
		if _, _, ok := policy.Admit(4, kindPlace(), 0); !ok {
			t.Fatalf("an unmatched participant was throttled at %d", i)
		}
	}
}

func TestCancelsAreFreeAgainstOrderBudgets(t *testing.T) {
	policy := buildRequestPolicy(map[string]RateLimitTier{
		"makers": {Roles: []string{"spot_maker"}, OrdersPer10s: 1},
	}, []Participant{{ClientID: 1, Role: "spot_maker_1"}})

	policy.Admit(1, kindPlace(), 0)
	if _, _, ok := policy.Admit(1, kindPlace(), 0); ok {
		t.Fatal("order budget did not bind")
	}
	for i := 0; i < 20; i++ {
		if _, _, ok := policy.Admit(1, kindCancel(), 0); !ok {
			t.Fatalf("a throttled maker could not cancel at %d: quotes would be stranded", i)
		}
	}
}

func TestNoTiersMeansNoPolicy(t *testing.T) {
	if policy := buildRequestPolicy(nil, []Participant{{ClientID: 1, Role: "spot_maker_1"}}); policy != nil {
		t.Fatal("an unconfigured scenario built a policy")
	}
}

func TestWeightBudgetsBindAcrossRequestKinds(t *testing.T) {
	policy := buildRequestPolicy(map[string]RateLimitTier{
		"makers": {Roles: []string{"spot_maker"}, WeightPerMinute: 21},
	}, []Participant{{ClientID: 1, Role: "spot_maker_1"}})

	// A balance query costs twenty, a placement one.
	if _, _, ok := policy.Admit(1, kindQueryBalance(), 0); !ok {
		t.Fatal("balance query refused")
	}
	if _, _, ok := policy.Admit(1, kindPlace(), 0); !ok {
		t.Fatal("placement refused inside the weight budget")
	}
	if _, _, ok := policy.Admit(1, kindPlace(), 0); ok {
		t.Fatal("weight budget did not bind")
	}
	// It refreshes with the window.
	if _, _, ok := policy.Admit(1, kindPlace(), int64(time.Minute)); !ok {
		t.Fatal("weight budget did not refresh")
	}
}

// Makers that all requote on one threshold go stale together, which removes the
// dislocations cross-market arbitrage trades. Heterogeneous thresholds are the
// way to test whether that staleness is what the arbitrage lives on.
func TestRequoteThresholdsAreAssignedPerMaker(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		tiers    []int64
		fallback int64
		want     []int64
	}{
		{"no tiers falls back to the single value", nil, 20, []int64{20, 20, 20}},
		{"tiers cycle across makers", []int64{0, 10, 30}, 20, []int64{0, 10, 30}},
		{"more makers than tiers cycles again", []int64{5, 15}, 0, []int64{5, 15, 5}},
	} {
		got := make([]int64, 3)
		for index := range got {
			got[index] = requoteThresholdFor(testCase.tiers, testCase.fallback, index)
		}
		for index, want := range testCase.want {
			if got[index] != want {
				t.Fatalf("%s: maker %d got %d, want %d", testCase.name, index, got[index], want)
			}
		}
	}
}
