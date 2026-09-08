package analysis

import "testing"

const testCollateralInterestDenominator int64 = 5_256_000_000

func marginInterestAccrualLine(timestamp, clientID, principal, rate, interest, spotInterest, perpInterest, before, after, denominator int64) string {
	return logLine(timestamp, uint64(clientID), "margin_interest_accrual", map[string]any{
		"timestamp": timestamp, "client_id": clientID, "asset": "USD", "interval_seconds": int64(60),
		"principal": principal, "rate_bps": rate, "interest": interest,
		"spot_interest": spotInterest, "perp_interest": perpInterest,
		"remainder_before": before, "remainder_after": after, "denominator": denominator,
	})
}

func marginInterestBorrowLine(timestamp, clientID, amount, rate, collateral int64) string {
	return logLine(timestamp, uint64(clientID), "borrow", map[string]any{
		"timestamp": timestamp, "client_id": clientID, "asset": "USD", "amount": amount,
		"reason": "test", "margin_mode": "cross", "interest_rate_bps": rate, "collateral_used": collateral,
	})
}

func marginInterestRepayLine(timestamp, clientID, principal, remaining int64) string {
	return logLine(timestamp, uint64(clientID), "repay", map[string]any{
		"timestamp": timestamp, "client_id": clientID, "asset": "USD", "principal": principal,
		"interest": int64(0), "remaining_debt": remaining,
	})
}

func marginInterestCloseLine(timestamp, clientID, remainder, debtBefore int64) string {
	return logLine(timestamp, uint64(clientID), "margin_interest_remainder_closed", map[string]any{
		"timestamp": timestamp, "client_id": clientID, "asset": "USD",
		"remainder_before": remainder, "remainder_after": int64(0),
		"denominator": testCollateralInterestDenominator, "debt_before": debtBefore, "debt_after": int64(0),
		"reason": "debt_repaid",
	})
}

func measureDebtTimeline(t *testing.T, terminalRemainders map[string]int64, lines ...string) InterestRemainderAudit {
	t.Helper()
	run, err := Open(writeRun(t, Report{
		TerminalAccounts: []AccountRow{{
			VenueID: "north", ClientID: 1,
			Account: Account{MarginInterestRemainders: terminalRemainders},
		}},
	}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	return conservation.InterestRemainder
}

func TestConservationAuditsDebtStateBehindCollateralInterest(t *testing.T) {
	lines := []string{
		marginInterestBorrowLine(1, 1, 100, 7, 50),
		marginInterestAccrualLine(2, 1, 100, 7, 0, 0, 0, 0, 700, testCollateralInterestDenominator),
		marginInterestRepayLine(3, 1, 100, 0),
		marginInterestCloseLine(3, 1, 700, 100),
	}
	audit := measureDebtTimeline(t, map[string]int64{}, lines...)
	if !audit.Applicable || !audit.Valid || audit.BorrowEvents != 1 || audit.RepayEvents != 1 || audit.DebtStateFailures != 0 || audit.DebtRateFailures != 0 || audit.UnlinkedClosures != 0 || audit.MissingTerminalMaps != 0 {
		t.Fatalf("valid debt-linked interest audit = %+v", audit)
	}
}

func TestConservationRejectsDebtTimelineMismatches(t *testing.T) {
	base := []string{
		marginInterestBorrowLine(1, 1, 100, 7, 50),
		marginInterestAccrualLine(2, 1, 99, 7, 0, 0, 0, 0, 693, testCollateralInterestDenominator),
		marginInterestRepayLine(3, 1, 100, 0),
		marginInterestCloseLine(3, 1, 700, 100),
	}
	forgedDebt := measureDebtTimeline(t, map[string]int64{}, base...)
	if forgedDebt.Valid || forgedDebt.DebtStateFailures == 0 {
		t.Fatalf("forged principal was accepted: %+v", forgedDebt)
	}

	noRepay := measureDebtTimeline(t, map[string]int64{},
		marginInterestBorrowLine(1, 1, 100, 7, 50),
		marginInterestAccrualLine(2, 1, 100, 7, 0, 0, 0, 0, 700, testCollateralInterestDenominator),
		marginInterestCloseLine(3, 1, 700, 100),
	)
	if noRepay.Valid || noRepay.UnlinkedClosures == 0 {
		t.Fatalf("unlinked closure was accepted: %+v", noRepay)
	}

	missingTerminalMap := measureDebtTimeline(t, nil,
		marginInterestBorrowLine(1, 1, 100, 7, 50),
		marginInterestAccrualLine(2, 1, 100, 7, 0, 0, 0, 0, 700, testCollateralInterestDenominator),
	)
	if missingTerminalMap.Valid || missingTerminalMap.MissingTerminalMaps == 0 {
		t.Fatalf("missing terminal remainder map was accepted: %+v", missingTerminalMap)
	}
}

func TestConservationRejectsDebtRateChangeWhileOutstanding(t *testing.T) {
	audit := measureDebtTimeline(t, map[string]int64{},
		marginInterestBorrowLine(1, 1, 100, 7, 50),
		marginInterestBorrowLine(2, 1, 20, 8, 10),
	)
	if audit.Valid || audit.DebtRateFailures == 0 {
		t.Fatalf("outstanding-debt rate change was accepted: %+v", audit)
	}
}

func TestConservationAuditsCollateralInterestRemainderTransition(t *testing.T) {
	finalSequence := uint64(1)
	report := Report{
		TerminalAccounts: []AccountRow{{
			VenueID: "north", ClientID: 1,
			Account: Account{MarginInterestRemainders: map[string]int64{"USD": 0}},
		}},
		VenueLedgers: []VenueLedger{{
			VenueID: "north", FeeRevenue: map[string]int64{"USD": 1}, FinalSequence: &finalSequence,
		}},
	}
	lines := []string{
		marginInterestAccrualLine(1, 1, testCollateralInterestDenominator/2, 1, 0, 0, 0, 0, testCollateralInterestDenominator/2, testCollateralInterestDenominator),
		marginInterestAccrualLine(2, 1, testCollateralInterestDenominator/2, 1, 1, 0, 1, testCollateralInterestDenominator/2, 0, testCollateralInterestDenominator),
		logLine(2, 1, "margin_interest", map[string]any{
			"timestamp": 2, "client_id": 1, "asset": "USD", "wallet": "perp", "amount": 1,
		}),
		changeLine(2, "north", 1, "", "interest_charge", [][3]any{{"USD", int64(10), int64(-1)}}),
		logLine(2, 0, "venue_balance_change", map[string]any{
			"timestamp": 2, "sequence": uint64(1), "bucket": "fee_revenue", "asset": "USD",
			"reason": "margin_interest", "old_balance": int64(0), "new_balance": int64(1), "delta": int64(1),
		}),
	}
	run, err := Open(writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	audit := conservation.InterestRemainder
	if !audit.Applicable || !audit.Valid || audit.Events != 2 || audit.Invalid != 0 || audit.TransitionFailures != 0 || audit.PeriodOrderFailures != 0 || audit.TerminalMismatches != 0 {
		t.Fatalf("valid interest remainder audit = %+v", audit)
	}
}

func TestConservationRejectsCollateralInterestRemainderCorruption(t *testing.T) {
	base := marginInterestAccrualLine(1, 1, 1, 1, 0, 0, 0, 0, 1, testCollateralInterestDenominator)
	measure := func(t *testing.T, line string) InterestRemainderAudit {
		t.Helper()
		run, err := Open(writeRun(t, Report{TerminalAccounts: []AccountRow{{
			VenueID: "north", ClientID: 1,
			Account: Account{MarginInterestRemainders: map[string]int64{"USD": 1}},
		}}}, map[string][]string{"north/derivatives.jsonl": {line}}))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		conservation, err := run.MeasureConservation(ConservationOptions{})
		if err != nil {
			t.Fatalf("measure: %v", err)
		}
		return conservation.InterestRemainder
	}

	valid := measure(t, base)
	if !valid.Valid || valid.Events != 1 {
		t.Fatalf("valid one-tick audit = %+v", valid)
	}
	wrongTransition := marginInterestAccrualLine(1, 1, 1, 1, 0, 0, 0, 5, 5, testCollateralInterestDenominator)
	transitionAudit := measure(t, wrongTransition)
	if transitionAudit.Valid || transitionAudit.Invalid != 1 {
		t.Fatalf("wrong transition audit = %+v", transitionAudit)
	}
	wrongDenominator := marginInterestAccrualLine(1, 1, 1, 1, 0, 0, 0, 0, 1, 10)
	denominatorAudit := measure(t, wrongDenominator)
	if denominatorAudit.Valid || denominatorAudit.Invalid != 1 {
		t.Fatalf("wrong denominator audit = %+v", denominatorAudit)
	}
}

func TestConservationAuditsCollateralInterestRemainderClose(t *testing.T) {
	finalSequence := uint64(0)
	run, err := Open(writeRun(t, Report{
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{MarginInterestRemainders: map[string]int64{"USD": 0}}}},
		VenueLedgers:     []VenueLedger{{VenueID: "north", FeeRevenue: map[string]int64{}, FinalSequence: &finalSequence}},
	}, map[string][]string{"north/derivatives.jsonl": {
		marginInterestAccrualLine(1, 1, testCollateralInterestDenominator/2, 1, 0, 0, 0, 0, testCollateralInterestDenominator/2, testCollateralInterestDenominator),
		logLine(2, 1, "margin_interest_remainder_closed", map[string]any{
			"timestamp": 2, "client_id": 1, "asset": "USD",
			"remainder_before": testCollateralInterestDenominator / 2, "remainder_after": int64(0),
			"denominator": testCollateralInterestDenominator, "reason": "debt_repaid",
		}),
	}}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if !conservation.InterestRemainder.Valid || conservation.InterestRemainder.ClosureEvents != 1 || conservation.InterestRemainder.ClosureStateFailures != 0 {
		t.Fatalf("valid remainder close audit = %+v", conservation.InterestRemainder)
	}
}

func TestConservationOrdersSameTimestampCollateralInterestClose(t *testing.T) {
	finalSequence := uint64(0)
	run, err := Open(writeRun(t, Report{
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{MarginInterestRemainders: map[string]int64{"USD": 0}}}},
		VenueLedgers:     []VenueLedger{{VenueID: "north", FeeRevenue: map[string]int64{}, FinalSequence: &finalSequence}},
	}, map[string][]string{"north/derivatives.jsonl": {
		marginInterestAccrualLine(1, 1, testCollateralInterestDenominator/2, 1, 0, 0, 0, 0, testCollateralInterestDenominator/2, testCollateralInterestDenominator),
		logLine(1, 1, "margin_interest_remainder_closed", map[string]any{
			"timestamp": 1, "client_id": 1, "asset": "USD",
			"remainder_before": testCollateralInterestDenominator / 2, "remainder_after": int64(0),
			"denominator": testCollateralInterestDenominator, "reason": "debt_repaid",
		}),
	}}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if !conservation.InterestRemainder.Valid || conservation.InterestRemainder.PeriodOrderFailures != 0 || conservation.InterestRemainder.ClosureStateFailures != 0 {
		t.Fatalf("same-timestamp close audit = %+v", conservation.InterestRemainder)
	}
}

func TestConservationRejectsUnprovedCollateralInterestRemainderClose(t *testing.T) {
	measure := func(t *testing.T, before int64) InterestRemainderAudit {
		t.Helper()
		finalSequence := uint64(0)
		run, err := Open(writeRun(t, Report{
			TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{MarginInterestRemainders: map[string]int64{"USD": 0}}}},
			VenueLedgers:     []VenueLedger{{VenueID: "north", FeeRevenue: map[string]int64{}, FinalSequence: &finalSequence}},
		}, map[string][]string{"north/derivatives.jsonl": {
			logLine(1, 1, "margin_interest_remainder_closed", map[string]any{
				"timestamp": 1, "client_id": 1, "asset": "USD",
				"remainder_before": before, "remainder_after": int64(0),
				"denominator": testCollateralInterestDenominator, "reason": "debt_repaid",
			}),
		}}))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		conservation, err := run.MeasureConservation(ConservationOptions{})
		if err != nil {
			t.Fatalf("measure: %v", err)
		}
		return conservation.InterestRemainder
	}

	withoutAccrual := measure(t, 1)
	if withoutAccrual.Valid || withoutAccrual.ClosureStateFailures != 1 {
		t.Fatalf("unproved close audit = %+v", withoutAccrual)
	}
	zeroClose := measure(t, 0)
	if zeroClose.Valid || zeroClose.InvalidClosures != 1 {
		t.Fatalf("zero remainder close audit = %+v", zeroClose)
	}
}
