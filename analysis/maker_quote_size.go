package analysis

import (
	"math/big"
	"sort"
)

// MakerQuoteSizeOptions selects the declared P1 Stoikov-maker families. Empty
// Roles uses the complete P1 spot-maker roster; it never silently includes
// perpetual or derivative maker activity.
type MakerQuoteSizeOptions struct {
	Roles []string
}

// MakerQuoteSizeAudit independently joins pre-send P1 quantity decisions to
// the venue's accepted/rejected requests. It validates the declared integer
// policy rather than inferring inventory appetite from later fills or book
// snapshots.
type MakerQuoteSizeAudit struct {
	Decisions                 int64                  `json:"decisions"`
	DecisionSides             int64                  `json:"decision_sides"`
	ZeroRiskDecisions         int64                  `json:"zero_risk_decisions"`
	NonzeroRiskDecisions      int64                  `json:"nonzero_risk_decisions"`
	ZeroRiskSymmetric         int64                  `json:"zero_risk_symmetric"`
	LongRiskDecisions         int64                  `json:"long_risk_decisions"`
	ShortRiskDecisions        int64                  `json:"short_risk_decisions"`
	LongPositiveSizeSkew      int64                  `json:"long_positive_size_skew"`
	ShortNegativeSizeSkew     int64                  `json:"short_negative_size_skew"`
	NonzeroRiskZeroSizeSkew   int64                  `json:"nonzero_risk_zero_size_skew"`
	WrongDirectionSizeSkew    int64                  `json:"wrong_direction_size_skew"`
	NonzeroAdjustments        int64                  `json:"nonzero_adjustments"`
	Accepted                  int64                  `json:"accepted"`
	Rejected                  int64                  `json:"rejected"`
	HorizonCensoredSides      int64                  `json:"horizon_censored_sides"`
	CensoredOutcomeDeliveries int64                  `json:"censored_outcome_deliveries"`
	MissingOutcomes           int64                  `json:"missing_outcomes"`
	DuplicateOutcomes         int64                  `json:"duplicate_outcomes"`
	DuplicateDecisionSides    int64                  `json:"duplicate_decision_sides"`
	DecisionFieldMismatches   int64                  `json:"decision_field_mismatches"`
	OutcomeFieldMismatches    int64                  `json:"outcome_field_mismatches"`
	InvalidDecisionRecords    int64                  `json:"invalid_decision_records"`
	InvalidCensorRecords      int64                  `json:"invalid_censor_records"`
	SkewBuckets               []QuoteSizeSkewBucket  `json:"skew_buckets"`
	MakerBuckets              []QuoteSizeMakerBucket `json:"maker_buckets"`
	Checks                    []MakerQuoteSizeCheck  `json:"checks,omitempty"`
}

// QuoteSizeSkewBucket keeps control and treatment evidence separate instead of
// averaging the registered coefficients into one activation statistic.
type QuoteSizeSkewBucket struct {
	SizeSkewBps int64 `json:"size_skew_bps"`
	Decisions   int64 `json:"decisions"`
	ZeroRisk    int64 `json:"zero_risk"`
	NonzeroRisk int64 `json:"nonzero_risk"`
	Adjusted    int64 `json:"adjusted"`
}

// QuoteSizeMakerBucket preserves numbered maker identity for P1's per-maker
// viability gate. Role aggregation alone could hide one silent participant.
type QuoteSizeMakerBucket struct {
	Maker                string `json:"maker"`
	Symbol               string `json:"symbol"`
	Decisions            int64  `json:"decisions"`
	DecisionSides        int64  `json:"decision_sides"`
	ZeroRisk             int64  `json:"zero_risk"`
	NonzeroRisk          int64  `json:"nonzero_risk"`
	Adjusted             int64  `json:"adjusted"`
	Accepted             int64  `json:"accepted"`
	Rejected             int64  `json:"rejected"`
	HorizonCensoredSides int64  `json:"horizon_censored_sides"`
}

// MakerQuoteSizeCheck names one evidence or policy-contract failure. Stable
// sorting permits direct comparison of independently replayed artifacts.
type MakerQuoteSizeCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id"`
	Side      string `json:"side"`
	Failure   string `json:"failure"`
}

type makerQuoteSizeDecision struct {
	Maker               string `json:"maker"`
	ClientID            uint64 `json:"client_id"`
	Symbol              string `json:"symbol"`
	DecisionTime        int64  `json:"decision_time"`
	BidRequestID        uint64 `json:"bid_request_id"`
	AskRequestID        uint64 `json:"ask_request_id"`
	BaseVolatilitySize  int64  `json:"base_volatility_size"`
	RiskPosition        int64  `json:"risk_position"`
	InventoryLimit      int64  `json:"inventory_limit"`
	SizeSkewBps         int64  `json:"size_skew_bps"`
	FullAdjustment      int64  `json:"full_adjustment"`
	Adjustment          int64  `json:"adjustment"`
	BidPrice            int64  `json:"bid_price"`
	AskPrice            int64  `json:"ask_price"`
	BidQty              int64  `json:"bid_qty"`
	AskQty              int64  `json:"ask_qty"`
	PostOnly            bool   `json:"post_only"`
	CancelBeforeReplace bool   `json:"cancel_before_replace"`
	OutcomeExpectation  string `json:"outcome_expectation"`
	CensorReason        string `json:"censor_reason"`
}

type makerQuoteSizeOrder struct {
	RequestID   uint64 `json:"request_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	Error       string `json:"error"`
}

type makerQuoteSizeKey struct {
	venueID  string
	clientID uint64
	request  uint64
}

type makerQuoteSizeExpected struct {
	symbol   string
	side     string
	price    int64
	qty      int64
	postOnly bool
	censored bool
	maker    string
}

type makerQuoteSizeOutcome struct {
	accepted bool
	order    makerQuoteSizeOrder
}

// MeasureMakerQuoteSize verifies P1's decision-side activation and the exact
// request relation. Missing evidence remains a scored failure rather than a
// numeric zero quantity or an inferred book update.
func (r *Run) MeasureMakerQuoteSize(options MakerQuoteSizeOptions) (*MakerQuoteSizeAudit, error) {
	roles := selectionSet(options.Roles)
	if len(roles) == 0 {
		roles = selectionSet([]string{"spot_maker", "cdf_spot_maker", "abc_cdf_spot_maker"})
	}
	result := &MakerQuoteSizeAudit{}
	expected := make(map[makerQuoteSizeKey]makerQuoteSizeExpected)
	outcomes := make(map[makerQuoteSizeKey][]makerQuoteSizeOutcome)
	buckets := make(map[int64]*QuoteSizeSkewBucket)
	makers := make(map[string]*QuoteSizeMakerBucket)
	addCheck := func(key makerQuoteSizeKey, side, failure string) {
		result.Checks = append(result.Checks, MakerQuoteSizeCheck{
			VenueID: key.venueID, ClientID: key.clientID, RequestID: key.request,
			Side: side, Failure: failure,
		})
	}

	err := r.Scan(ScanOptions{
		Events:  []string{"maker_quote_size_decision", "OrderAccepted", "OrderRejected"},
		Workers: 1,
	}, func(event Event) {
		if !selected(r.Role(event.VenueID, event.ClientID), roles) {
			return
		}
		switch event.Name {
		case "maker_quote_size_decision":
			var decision makerQuoteSizeDecision
			if event.Decode(&decision) != nil || decision.ClientID == 0 || decision.BidRequestID == 0 || decision.AskRequestID == 0 || decision.BidRequestID == decision.AskRequestID {
				result.InvalidDecisionRecords++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "invalid_decision_record")
				return
			}
			if decision.ClientID != event.ClientID {
				result.InvalidDecisionRecords++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "decision_client_mismatch")
				return
			}
			if decision.Maker == "" || RoleGroup(decision.Maker) != r.Role(event.VenueID, event.ClientID) {
				result.InvalidDecisionRecords++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "decision_maker_mismatch")
				return
			}
			result.Decisions++
			bucket := buckets[decision.SizeSkewBps]
			if bucket == nil {
				bucket = &QuoteSizeSkewBucket{SizeSkewBps: decision.SizeSkewBps}
				buckets[decision.SizeSkewBps] = bucket
			}
			bucket.Decisions++
			maker := makers[decision.Maker]
			if maker == nil {
				maker = &QuoteSizeMakerBucket{Maker: decision.Maker, Symbol: decision.Symbol}
				makers[decision.Maker] = maker
			}
			if maker.Symbol != decision.Symbol {
				result.InvalidDecisionRecords++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "maker_symbol_mismatch")
			}
			maker.Decisions++
			if decision.RiskPosition == 0 {
				result.ZeroRiskDecisions++
				bucket.ZeroRisk++
				maker.ZeroRisk++
				if decision.AskQty == decision.BidQty {
					result.ZeroRiskSymmetric++
				} else {
					result.WrongDirectionSizeSkew++
					addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "zero_risk_asymmetric_size")
				}
			} else {
				result.NonzeroRiskDecisions++
				bucket.NonzeroRisk++
				maker.NonzeroRisk++
				skew := decision.AskQty - decision.BidQty
				if decision.RiskPosition > 0 {
					result.LongRiskDecisions++
					if skew > 0 {
						result.LongPositiveSizeSkew++
					} else if skew == 0 {
						result.NonzeroRiskZeroSizeSkew++
					} else {
						result.WrongDirectionSizeSkew++
						addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "long_risk_wrong_size_direction")
					}
				} else {
					result.ShortRiskDecisions++
					if skew < 0 {
						result.ShortNegativeSizeSkew++
					} else if skew == 0 {
						result.NonzeroRiskZeroSizeSkew++
					} else {
						result.WrongDirectionSizeSkew++
						addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "short_risk_wrong_size_direction")
					}
				}
			}
			if decision.Adjustment > 0 {
				result.NonzeroAdjustments++
				bucket.Adjusted++
				maker.Adjusted++
			}

			if !validMakerQuoteSizeDecision(decision) {
				result.DecisionFieldMismatches++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "decision_policy_mismatch")
			}
			censored, validCensor := validMakerQuoteSizeCensor(decision)
			if !validCensor {
				result.InvalidCensorRecords++
				addCheck(makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID}, "", "invalid_outcome_expectation")
			}
			for _, side := range []struct {
				name      string
				requestID uint64
				price     int64
				qty       int64
			}{
				{name: "BUY", requestID: decision.BidRequestID, price: decision.BidPrice, qty: decision.BidQty},
				{name: "SELL", requestID: decision.AskRequestID, price: decision.AskPrice, qty: decision.AskQty},
			} {
				key := makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID, request: side.requestID}
				result.DecisionSides++
				maker.DecisionSides++
				if _, duplicate := expected[key]; duplicate {
					result.DuplicateDecisionSides++
					addCheck(key, side.name, "duplicate_decision_side")
					continue
				}
				expected[key] = makerQuoteSizeExpected{symbol: decision.Symbol, side: side.name, price: side.price, qty: side.qty, postOnly: decision.PostOnly, censored: censored, maker: decision.Maker}
			}
		case "OrderAccepted", "OrderRejected":
			var order makerQuoteSizeOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				return
			}
			if order.Symbol == "" {
				order.Symbol = event.Symbol
			}
			if order.Symbol == "" {
				order.Symbol = symbolFromSpotFile(event.File)
			}
			key := makerQuoteSizeKey{venueID: event.VenueID, clientID: event.ClientID, request: order.RequestID}
			outcomes[key] = append(outcomes[key], makerQuoteSizeOutcome{accepted: event.Name == "OrderAccepted", order: order})
		}
	})
	if err != nil {
		return nil, err
	}

	for key, want := range expected {
		maker := makers[want.maker]
		got := outcomes[key]
		if want.censored {
			if len(got) == 0 {
				result.HorizonCensoredSides++
				maker.HorizonCensoredSides++
				continue
			}
			result.CensoredOutcomeDeliveries += int64(len(got))
			addCheck(key, want.side, "terminal_censored_request_delivered")
			continue
		}
		switch len(got) {
		case 0:
			result.MissingOutcomes++
			addCheck(key, want.side, "missing_request_outcome")
			continue
		case 1:
		default:
			result.DuplicateOutcomes++
			addCheck(key, want.side, "duplicate_request_outcome")
			continue
		}
		outcome := got[0]
		if outcome.accepted {
			result.Accepted++
			maker.Accepted++
		} else {
			result.Rejected++
			maker.Rejected++
		}
		if outcome.order.Symbol != want.symbol || outcome.order.Side != want.side || outcome.order.Type != "LIMIT" ||
			outcome.order.TimeInForce != "GTC" || outcome.order.PostOnly != want.postOnly || outcome.order.Price != want.price || outcome.order.Qty != want.qty {
			result.OutcomeFieldMismatches++
			addCheck(key, want.side, "request_fields_mismatch")
		}
	}
	for _, bucket := range buckets {
		result.SkewBuckets = append(result.SkewBuckets, *bucket)
	}
	for _, maker := range makers {
		result.MakerBuckets = append(result.MakerBuckets, *maker)
	}
	sort.Slice(result.SkewBuckets, func(i, j int) bool { return result.SkewBuckets[i].SizeSkewBps < result.SkewBuckets[j].SizeSkewBps })
	sort.Slice(result.MakerBuckets, func(i, j int) bool {
		if result.MakerBuckets[i].Maker != result.MakerBuckets[j].Maker {
			return result.MakerBuckets[i].Maker < result.MakerBuckets[j].Maker
		}
		return result.MakerBuckets[i].Symbol < result.MakerBuckets[j].Symbol
	})
	sort.Slice(result.Checks, func(i, j int) bool {
		left, right := result.Checks[i], result.Checks[j]
		if left.VenueID != right.VenueID {
			return left.VenueID < right.VenueID
		}
		if left.ClientID != right.ClientID {
			return left.ClientID < right.ClientID
		}
		if left.RequestID != right.RequestID {
			return left.RequestID < right.RequestID
		}
		if left.Side != right.Side {
			return left.Side < right.Side
		}
		return left.Failure < right.Failure
	})
	return result, nil
}

// validMakerQuoteSizeDecision derives the registered P1 integer quantities
// with independent arbitrary-precision arithmetic. It deliberately does not
// call the actor's checked helper, so a shared arithmetic bug cannot approve
// both generator and detector.
func validMakerQuoteSizeDecision(decision makerQuoteSizeDecision) bool {
	if decision.BaseVolatilitySize <= 0 || decision.InventoryLimit <= 0 || decision.SizeSkewBps < 0 || decision.SizeSkewBps > 5_000 ||
		decision.BidPrice <= 0 || decision.AskPrice <= decision.BidPrice || !decision.PostOnly || !decision.CancelBeforeReplace ||
		decision.BidQty <= 0 || decision.AskQty <= 0 {
		return false
	}
	base := big.NewInt(decision.BaseVolatilitySize)
	bps := big.NewInt(decision.SizeSkewBps)
	denominator := big.NewInt(10_000)
	full := new(big.Int).Mul(base, bps)
	full.Quo(full, denominator)
	if !full.IsInt64() || full.Int64() != decision.FullAdjustment {
		return false
	}
	risk := big.NewInt(decision.RiskPosition)
	risk.Abs(risk)
	limit := big.NewInt(decision.InventoryLimit)
	if risk.Cmp(limit) > 0 {
		risk.Set(limit)
	}
	adjustment := new(big.Int).Mul(full, risk)
	adjustment.Quo(adjustment, limit)
	if !adjustment.IsInt64() || adjustment.Int64() != decision.Adjustment {
		return false
	}
	if decision.RiskPosition == 0 {
		return decision.Adjustment == 0 && decision.BidQty == decision.BaseVolatilitySize && decision.AskQty == decision.BaseVolatilitySize
	}
	if decision.RiskPosition > 0 {
		return decision.BidQty == decision.BaseVolatilitySize-decision.Adjustment && decision.AskQty == decision.BaseVolatilitySize+decision.Adjustment
	}
	return decision.AskQty == decision.BaseVolatilitySize-decision.Adjustment && decision.BidQty == decision.BaseVolatilitySize+decision.Adjustment
}

// validMakerQuoteSizeCensor accepts only the two preregistered evidence
// states. The offline audit intentionally trusts no inferred horizon from
// timestamps: terminal suppression is explicit producer evidence, and an
// observed venue outcome falsifies that claim below.
func validMakerQuoteSizeCensor(decision makerQuoteSizeDecision) (censored bool, valid bool) {
	switch decision.OutcomeExpectation {
	case "VENUE_OUTCOME_REQUIRED":
		return false, decision.CensorReason == ""
	case "SIMULATION_HORIZON_CENSORED":
		return true, decision.CensorReason == "terminal_horizon_before_venue_ingress"
	default:
		return false, false
	}
}
