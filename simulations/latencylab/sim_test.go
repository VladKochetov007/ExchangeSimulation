package latencylab

import (
	"context"
	"reflect"
	"runtime"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func runRace(t *testing.T, cfg Config) Result {
	t.Helper()
	sim, err := NewSim(cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	result, err := sim.Run(context.Background())
	if err != nil {
		for id, gateway := range sim.ex.Gateways {
			t.Logf("gateway %d req=%d resp=%d md=%d idle=%v", id, len(gateway.RequestCh), len(gateway.ResponseCh), len(gateway.MarketData), gateway.Idle())
		}
		t.Fatalf("Run: %v", err)
	}
	return result
}

func TestHighClientIDFastActorWins(t *testing.T) {
	result := runRace(t, DefaultConfig())
	if result.Winner() != alphaName {
		t.Fatalf("winner = %q, want fast high-ID alpha: %#v", result.Winner(), result)
	}
	if !result.Alpha.PairComplete || result.Alpha.LockedCashflow != 2 || result.Alpha.AccountUSDDelta != 2 || result.Alpha.AccountABCDelta != 0 {
		t.Fatalf("fast actor report = %#v", result.Alpha)
	}
	if result.Beta.PairComplete || result.Beta.ObservedCashflow != 0 || result.Beta.AccountUSDDelta != 0 || result.Beta.AccountABCDelta != 0 {
		t.Fatalf("slow actor should have two clean FOK rejects: %#v", result.Beta)
	}
	for _, leg := range result.Beta.Legs {
		if !leg.Rejected || leg.RejectReason != exchange.RejectFOKNotFilled || leg.FilledQty != 0 {
			t.Fatalf("slow FOK leg = %#v", leg)
		}
	}
	if result.Alpha.ReactionLatency >= result.Beta.ReactionLatency {
		t.Fatalf("signal observation did not preserve latency assignment: alpha=%d beta=%d", result.Alpha.ReactionLatency, result.Beta.ReactionLatency)
	}
}

func TestLatencyLabelSwapFlipsWinner(t *testing.T) {
	fastAlpha := runRace(t, DefaultConfig())
	swapped := DefaultConfig()
	swapped.AlphaLatency, swapped.BetaLatency = swapped.BetaLatency, swapped.AlphaLatency
	fastBeta := runRace(t, swapped)
	if fastAlpha.Winner() != alphaName || fastBeta.Winner() != betaName {
		t.Fatalf("label swap did not flip physical winner: alpha-fast=%#v beta-fast=%#v", fastAlpha, fastBeta)
	}
}

func TestWinnerDoesNotDependOnActorRegistrationOrder(t *testing.T) {
	normal := runRace(t, DefaultConfig())
	reversed := DefaultConfig()
	reversed.ReverseActors = true
	reversedResult := runRace(t, reversed)
	if normal.Winner() != alphaName || reversedResult.Winner() != alphaName {
		t.Fatalf("actor registration changed winner: normal=%#v reversed=%#v", normal, reversedResult)
	}
}

func TestLatencyLabRejectsEqualLatency(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BetaLatency = cfg.AlphaLatency
	if _, err := NewSim(cfg); err == nil {
		t.Fatal("NewSim accepted an equal-latency race whose winner would be client-ID ordered")
	}
}

func TestLatencyRaceDeterministicAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) Result {
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		return runRace(t, DefaultConfig())
	}
	one := run(1)
	many := run(14)
	if !reflect.DeepEqual(one, many) {
		t.Fatalf("latency reports differ by GOMAXPROCS:\n1: %#v\n14: %#v", one, many)
	}
}

func TestLatencyLabRequiresSufficientHorizon(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Duration = 39 * time.Millisecond
	if _, err := NewSim(cfg); err == nil {
		t.Fatal("NewSim accepted a horizon that can terminate before the slow response")
	}
}
