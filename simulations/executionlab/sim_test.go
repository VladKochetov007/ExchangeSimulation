package executionlab

import (
	"context"
	"reflect"
	"runtime"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestImmediateAndTWAPParentLifecycle(t *testing.T) {
	for _, policy := range []Policy{Immediate, TWAP} {
		t.Run(string(policy), func(t *testing.T) {
			cfg := DefaultSimConfig(policy)
			sim, err := NewSim(cfg)
			if err != nil {
				t.Fatalf("NewSim: %v", err)
			}
			report, err := sim.Run(context.Background())
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if report.DecisionMid <= 0 || report.DecisionAt == 0 {
				t.Fatalf("parent never made a two-sided-book decision: %#v", report)
			}
			wantChildren := 1
			if policy == TWAP {
				wantChildren = cfg.Parent.SliceCount
			}
			if report.SubmittedChildren != wantChildren || len(report.Children) != wantChildren {
				t.Fatalf("children = %d/%d, want %d: %#v", report.SubmittedChildren, len(report.Children), wantChildren, report)
			}
			if report.FilledQty <= 0 || report.FilledQty > report.TargetQty {
				t.Fatalf("invalid filled quantity %d for target %d: %#v", report.FilledQty, report.TargetQty, report)
			}
			if report.Notional <= 0 || report.FirstVenueFillAt < report.DecisionAt {
				t.Fatalf("missing execution accounting: %#v", report)
			}
			if !report.TargetShortfallValid || report.TerminalMid <= 0 || report.TerminalMarkSource != "two_sided_book_mid" {
				t.Fatalf("missing terminal mark-to-complete accounting: %#v", report)
			}
		})
	}
}

func TestTWAPReportDeterministicAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) ExecutionReport {
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(DefaultSimConfig(TWAP))
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		report, err := sim.Run(context.Background())
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return report
	}
	one := run(1)
	many := run(14)
	if !reflect.DeepEqual(one, many) {
		t.Fatalf("TWAP reports differ by GOMAXPROCS:\n1:  %#v\n14: %#v", one, many)
	}
}

func TestExplicitDeploymentKeepsLegacyZeroProcessingEconomics(t *testing.T) {
	legacyConfig := DefaultSimConfig(Immediate)
	legacy, err := NewSim(legacyConfig)
	if err != nil {
		t.Fatal(err)
	}
	legacyReport, err := legacy.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directedConfig := legacyConfig
	directedConfig.ExecutionLatency = 0
	directedConfig.ParentDeployment = &ParentDeployment{
		MarketDataLatency: time.Millisecond, RequestLatency: time.Millisecond,
		ResponseLatency: time.Millisecond,
	}
	directed, err := NewSim(directedConfig)
	if err != nil {
		t.Fatal(err)
	}
	directedReport, err := directed.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyReport, directedReport) {
		t.Fatalf("explicit zero-processing deployment changed old economics:\nlegacy=%#v\ndirected=%#v", legacyReport, directedReport)
	}
}

func TestRetainedC0SeedEconomicsSurviveExplicitDeployment(t *testing.T) {
	legacyConfig := DefaultSimConfig(Immediate)
	legacyConfig.Seed = 1009
	legacyConfig.Parent.TargetQty = 500_000_000
	legacy, err := NewSim(legacyConfig)
	if err != nil {
		t.Fatal(err)
	}
	legacyReport, err := legacy.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directedConfig := legacyConfig
	directedConfig.ExecutionLatency = 0
	directedConfig.ParentDeployment = &ParentDeployment{MarketDataLatency: time.Millisecond,
		RequestLatency: time.Millisecond, ResponseLatency: time.Millisecond}
	directed, err := NewSim(directedConfig)
	if err != nil {
		t.Fatal(err)
	}
	directedReport, err := directed.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacyReport, directedReport) {
		t.Fatalf("retained C0 seed changed under equivalent deployment:\nlegacy=%#v\ndirected=%#v", legacyReport, directedReport)
	}
}

func TestExplicitDeploymentRejectsConflictingOrNegativeLatencies(t *testing.T) {
	for _, deployment := range []ParentDeployment{
		{MarketDataLatency: -time.Millisecond}, {RequestLatency: -time.Millisecond},
		{ResponseLatency: -time.Millisecond}, {ProcessingDelay: -time.Millisecond},
	} {
		config := DefaultSimConfig(Immediate)
		config.ExecutionLatency = 0
		config.ParentDeployment = &deployment
		if _, err := NewSim(config); err == nil {
			t.Fatalf("accepted negative deployment %#v", deployment)
		}
	}
	config := DefaultSimConfig(Immediate)
	config.ParentDeployment = &ParentDeployment{MarketDataLatency: time.Millisecond}
	if _, err := NewSim(config); err == nil {
		t.Fatal("accepted ambiguous legacy and explicit focal latency")
	}
}

func TestFocalDeploymentFollowsAssignmentNotClientID(t *testing.T) {
	for _, delay := range []time.Duration{time.Millisecond, 90 * time.Millisecond} {
		var reports []ExecutionReport
		for _, clientID := range []uint64{13, 14} {
			config := DefaultSimConfig(Immediate)
			config.Seed = 42
			config.ParentClientID = clientID
			config.ExecutionLatency = 0
			config.ParentDeployment = &ParentDeployment{
				MarketDataLatency: delay, RequestLatency: delay, ResponseLatency: delay,
				ProcessingDelay: 120 * time.Millisecond,
			}
			world, err := NewSim(config)
			if err != nil {
				t.Fatal(err)
			}
			if world.Parent.ID() != clientID {
				t.Fatalf("assigned ID=%d got %d", clientID, world.Parent.ID())
			}
			report, err := world.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			reports = append(reports, report)
		}
		if !reflect.DeepEqual(reports[0], reports[1]) {
			t.Fatalf("identical policy/deployment differs when client ID changes at %s: %#v versus %#v", delay, reports[0], reports[1])
		}
	}
}

func TestStaggeredParentReportsAreDeterministicAndAlternating(t *testing.T) {
	config := DefaultSimConfig(TWAP)
	config.ParentCount = 4
	config.ParentInterval = time.Second
	config.Duration = 7 * time.Second

	run := func(procs int) []ExecutionReport {
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(config)
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		reports, err := sim.RunMany(context.Background())
		if err != nil {
			t.Fatalf("RunMany: %v", err)
		}
		return reports
	}

	one := run(1)
	many := run(14)
	if !reflect.DeepEqual(one, many) {
		t.Fatalf("staggered parent reports differ by GOMAXPROCS:\n1:  %#v\n14: %#v", one, many)
	}
	if len(one) != config.ParentCount {
		t.Fatalf("report count = %d, want %d", len(one), config.ParentCount)
	}
	for i, report := range one {
		if report.DecisionAt == 0 || report.DecisionMid <= 0 || !report.TargetShortfallValid {
			t.Fatalf("parent %d lacks a valid decision/terminal report: %#v", i, report)
		}
		wantSide := exchange.Buy
		if i%2 == 1 {
			wantSide = exchange.Sell
		}
		if report.Side != wantSide {
			t.Fatalf("parent %d side = %s, want %s", i, report.Side, wantSide)
		}
		if i > 0 && report.DecisionAt <= one[i-1].DecisionAt {
			t.Fatalf("parent decisions not strictly staggered: %d then %d", one[i-1].DecisionAt, report.DecisionAt)
		}
	}
}

func TestTWAPRejectsInsufficientDrainHorizon(t *testing.T) {
	cfg := DefaultSimConfig(TWAP)
	cfg.Duration = cfg.Parent.DecisionAfter
	if _, err := NewSim(cfg); err == nil {
		t.Fatal("NewSim accepted a duration that drops scheduled parent children")
	}
}

func TestWorldContractCapturesConstructedParticipants(t *testing.T) {
	for _, counts := range [][2]int{{2, 10}, {4, 8}, {6, 6}} {
		cfg := DefaultSimConfig(Immediate)
		cfg.MMCount, cfg.NoiseTraderCount = counts[0], counts[1]
		sim, err := NewSim(cfg)
		if err != nil {
			t.Fatalf("NewSim(%v): %v", counts, err)
		}
		contract := sim.WorldContract()
		if contract.Config.MMCount != counts[0] || contract.Config.NoiseTraderCount != counts[1] ||
			len(contract.Accounts) != 13 || len(contract.Makers) != counts[0] ||
			len(contract.Noise) != counts[1] || len(contract.Parents) != 1 {
			t.Fatalf("wrong roster for %v: %#v", counts, contract)
		}
		if contract.Parents[0].ClientID != 13 || contract.Parents[0].Config.TargetQty != cfg.Parent.TargetQty ||
			contract.Instrument.Symbol != "ABC/USD" || contract.Runner.Iterations != 4000 {
			t.Fatalf("wrong parent/instrument/runner for %v: %#v", counts, contract)
		}
		for i, maker := range contract.Makers {
			if maker.ClientID != uint64(i+1) || len(maker.RealizedLevelCadences) != 5 ||
				maker.RealizedLevelCadences[0] != time.Duration(10+i)*time.Millisecond {
				t.Fatalf("wrong maker cadence for %v maker %d: %#v", counts, i, maker)
			}
		}
		for i, noise := range contract.Noise {
			if noise.ClientID != uint64(counts[0]+i+1) || noise.Seed != cfg.Seed+int64(i)+1 ||
				noise.Latency != cfg.BackgroundLatency {
				t.Fatalf("wrong taker for %v index %d: %#v", counts, i, noise)
			}
		}
		if contract.Accounts[12].InitialBalances["ABC"] != 100_000*basePrecision ||
			contract.Accounts[12].Fee.TakerBps != 5 || !contract.Accounts[12].Fee.InQuote {
			t.Fatalf("wrong focal account for %v: %#v", counts, contract.Accounts[12])
		}
		contract.Accounts[0].InitialBalances["ABC"] = 1
		contract.Makers[0].RealizedLevelCadences[0] = 1
		again := sim.WorldContract()
		if again.Accounts[0].InitialBalances["ABC"] != 100_000*basePrecision ||
			again.Makers[0].RealizedLevelCadences[0] == 1 {
			t.Fatal("caller mutated retained contract")
		}
	}
}

func TestNewSimRejectsNegativeRoster(t *testing.T) {
	for _, cfg := range []SimConfig{
		func() SimConfig { c := DefaultSimConfig(Immediate); c.MMCount = -1; return c }(),
		func() SimConfig { c := DefaultSimConfig(Immediate); c.NoiseTraderCount = -1; return c }(),
	} {
		if _, err := NewSim(cfg); err == nil {
			t.Fatalf("negative roster accepted: %#v", cfg)
		}
	}
}

func TestExecutionShortfallUsesFilledReferenceAndQuoteFees(t *testing.T) {
	cfg := DefaultSimConfig(Immediate).Parent
	gateway := exchange.NewClientGateway(1)
	a, err := newExecutionAgent(1, gateway, cfg)
	if err != nil {
		t.Fatalf("newExecutionAgent: %v", err)
	}
	a.report.DecisionMid = 100 * quotePrecision
	a.report.Children = []ChildReport{{OrderID: 7}}
	a.byOrder[7] = 0
	a.recordFill(actor.OrderFillEvent{
		OrderID: 7, Symbol: cfg.Symbol, Qty: basePrecision,
		Price: 101 * quotePrecision, FeeAsset: "USD", FeeAmount: 2,
		Timestamp: 10,
	})
	a.recordFill(actor.OrderFillEvent{
		OrderID: 7, Symbol: cfg.Symbol, Qty: basePrecision,
		Price: 102 * quotePrecision, FeeAsset: "USD", FeeAmount: 3,
		Timestamp: 11,
	})

	report := a.Report()
	want := int64(3*quotePrecision + 5) // (101+102-2*100) USD plus quote fees.
	if report.Shortfall != want {
		t.Fatalf("shortfall = %d, want %d", report.Shortfall, want)
	}
	if report.UnfilledQty != 0 || report.FirstVenueFillAt != 10 || report.LastVenueFillAt != 11 {
		t.Fatalf("execution timing/completion = %#v", report)
	}
	if report.TargetShortfallValid {
		t.Fatalf("Report unexpectedly priced a target without a terminal mark: %#v", report)
	}
}

func TestExecutionTargetShortfallMarksUnfilledResidual(t *testing.T) {
	cfg := DefaultSimConfig(Immediate).Parent
	gateway := exchange.NewClientGateway(1)
	a, err := newExecutionAgent(1, gateway, cfg)
	if err != nil {
		t.Fatalf("newExecutionAgent: %v", err)
	}
	a.report.DecisionMid = 100 * quotePrecision
	a.report.Children = []ChildReport{{OrderID: 7}}
	a.byOrder[7] = 0
	a.recordFill(actor.OrderFillEvent{
		OrderID: 7, Symbol: cfg.Symbol, Qty: basePrecision,
		Price: 101 * quotePrecision, FeeAsset: "USD", FeeAmount: 2,
		Timestamp: 10,
	})

	report := a.ReportWithTerminalMid(102 * quotePrecision)
	want := int64(3*quotePrecision + 2) // 101 observed + 102 marked - 2*100, plus observed fee.
	if !report.TargetShortfallValid || report.TerminalMid != 102*quotePrecision {
		t.Fatalf("missing terminal mark: %#v", report)
	}
	if report.FilledQty != basePrecision || report.UnfilledQty != basePrecision || report.TargetShortfall != want {
		t.Fatalf("target shortfall = %#v, want %d", report, want)
	}
	if report.Shortfall != quotePrecision+2 {
		t.Fatalf("filled-only shortfall changed = %d, want %d", report.Shortfall, quotePrecision+2)
	}
}

func TestExecutionTargetShortfallRejectsUnpricedForeignFees(t *testing.T) {
	cfg := DefaultSimConfig(Immediate).Parent
	gateway := exchange.NewClientGateway(1)
	a, err := newExecutionAgent(1, gateway, cfg)
	if err != nil {
		t.Fatalf("newExecutionAgent: %v", err)
	}
	a.report.DecisionMid = 100 * quotePrecision
	a.report.Children = []ChildReport{{OrderID: 7}}
	a.byOrder[7] = 0
	a.recordFill(actor.OrderFillEvent{
		OrderID: 7, Symbol: cfg.Symbol, Qty: basePrecision,
		Price: 101 * quotePrecision, FeeAsset: "BNB", FeeAmount: 1,
		Timestamp: 10,
	})

	report := a.ReportWithTerminalMid(102 * quotePrecision)
	if report.UnpricedFeeCount != 1 || report.TargetShortfallValid || report.TerminalMid != 0 {
		t.Fatalf("foreign-fee target metric claimed validity: %#v", report)
	}
}
