package executionlab

import (
	"context"
	"reflect"
	"runtime"
	"testing"

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

func TestTWAPRejectsInsufficientDrainHorizon(t *testing.T) {
	cfg := DefaultSimConfig(TWAP)
	cfg.Duration = cfg.Parent.DecisionAfter
	if _, err := NewSim(cfg); err == nil {
		t.Fatal("NewSim accepted a duration that drops scheduled parent children")
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
}
