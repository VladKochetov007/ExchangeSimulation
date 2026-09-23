// Package executionlab provides a controlled parent-order experiment. It is
// deliberately separate from the general ecology: execution quality is a
// cost/completion measurement, not account PnL after unrelated future moves.
package executionlab

import (
	"context"
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

type Policy string

const (
	Immediate Policy = "immediate"
	TWAP      Policy = "twap"
)

// ParentOrderConfig defines one execution decision. All monetary values use
// the venue's fixed-point units. SliceCount includes the first child sent at
// DecisionAfter; an immediate parent always sends exactly one child.
type ParentOrderConfig struct {
	Symbol        string
	Side          exchange.Side
	TargetQty     int64
	BasePrecision int64
	QuoteAsset    string
	Policy        Policy
	Instruction   *ChildInstruction `json:"instruction,omitempty"`
	DecisionAfter time.Duration
	SliceInterval time.Duration
	SliceCount    int
	PollInterval  time.Duration
}

type ChildInstruction struct {
	OrderType   exchange.OrderType   `json:"order_type"`
	TimeInForce exchange.TimeInForce `json:"time_in_force"`
	LimitPrice  int64                `json:"limit_price"`
}

func (i ChildInstruction) validate() error {
	if i.TimeInForce != exchange.GTC && i.TimeInForce != exchange.IOC && i.TimeInForce != exchange.FOK {
		return fmt.Errorf("executionlab: unsupported time-in-force %d", i.TimeInForce)
	}
	switch i.OrderType {
	case exchange.Market:
		if i.LimitPrice != 0 {
			return fmt.Errorf("executionlab: market child cannot have a limit price")
		}
	case exchange.LimitOrder:
		if i.LimitPrice <= 0 {
			return fmt.Errorf("executionlab: spot limit child requires a positive price")
		}
	default:
		return fmt.Errorf("executionlab: unsupported order type %d", i.OrderType)
	}
	return nil
}

func (c ParentOrderConfig) validate() error {
	if c.Symbol == "" || c.TargetQty <= 0 || c.BasePrecision <= 0 || c.QuoteAsset == "" {
		return fmt.Errorf("executionlab: symbol, target quantity, base precision, and quote asset are required")
	}
	if c.Policy != Immediate && c.Policy != TWAP {
		return fmt.Errorf("executionlab: unknown execution policy %q", c.Policy)
	}
	if c.DecisionAfter < 0 || c.PollInterval <= 0 {
		return fmt.Errorf("executionlab: decision delay must be non-negative and poll interval positive")
	}
	if c.Policy == TWAP && (c.SliceCount < 2 || c.SliceInterval <= 0) {
		return fmt.Errorf("executionlab: TWAP requires at least two positive-interval slices")
	}
	if c.Instruction != nil {
		if err := c.Instruction.validate(); err != nil {
			return err
		}
	}
	return nil
}

// ChildReport records a child exactly as observed by the agent. SentAt is the
// decision-side transmit time; fill timestamps are exchange match times.
type ChildReport struct {
	RequestID       uint64
	OrderID         uint64
	SentAt          int64
	RequestedQty    int64
	FilledQty       int64
	Notional        int64
	QuoteFee        int64
	FirstFillAt     int64
	LastFillAt      int64
	Rejected        bool
	RejectReason    exchange.RejectReason
	CancelRemaining int64
}

// ExecutionReport separates filled impact from target completion. Shortfall is
// the signed all-in cost of the quantity actually executed against the
// decision mid: buy: paid - reference + fees; sell: reference - proceeds +
// fees. It must never be read without UnfilledQty, because a cheap partial
// execution is not a completed parent order.
//
// TargetShortfall is a separately labelled hypothetical mark-to-complete
// metric. It values UnfilledQty at a contemporaneous two-sided TerminalMid
// without assuming it was filled or charging an invented terminal fee. It is
// valid only when every observed fee was priced in the quote asset. It provides
// an urgency-sensitive comparison when one parent exhausts the book, while
// FilledQty and Shortfall retain the observed execution result.
type ExecutionReport struct {
	Policy               Policy
	Side                 exchange.Side
	TargetQty            int64
	DecisionAt           int64
	DecisionMid          int64
	FirstVenueFillAt     int64
	LastVenueFillAt      int64
	FilledQty            int64
	UnfilledQty          int64
	Notional             int64
	QuoteFees            int64
	UnpricedFeeCount     int
	Shortfall            int64
	ShortfallBps         float64
	TerminalMid          int64
	TerminalMarkSource   string
	TargetShortfall      int64
	TargetShortfallBps   float64
	TargetShortfallValid bool
	SubmittedChildren    int
	RejectedChildren     int
	TerminalCancels      int
	Children             []ChildReport
}

type executionAgent struct {
	*actor.BaseActor
	cfg             ParentOrderConfig
	observe         func(EvidenceObservation)
	observationTime func() int64

	bestBid                  int64
	bestAsk                  int64
	processingDelay          time.Duration
	emitProcessingCompletion bool
	pendingSnapshots         []pendingProcessingSnapshot

	decided      bool
	nextSliceAt  int64
	sentSlices   int
	submittedQty int64
	byRequest    map[uint64]int
	byOrder      map[uint64]int
	report       ExecutionReport
}

type pendingProcessingSnapshot struct {
	sequence   uint64
	receivedAt int64
	readyAt    int64
	bestBid    int64
	bestAsk    int64
	twoSided   bool
}

func newExecutionAgent(id uint64, gateway actor.Gateway, cfg ParentOrderConfig) (*executionAgent, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	a := &executionAgent{
		BaseActor: actor.NewBaseActor(id, gateway),
		cfg:       cfg,
		byRequest: make(map[uint64]int),
		byOrder:   make(map[uint64]int),
		report: ExecutionReport{
			Policy:    cfg.Policy,
			Side:      cfg.Side,
			TargetQty: cfg.TargetQty,
		},
	}
	a.SetHandler(a)
	a.AddTicker(cfg.PollInterval, a.onTick)
	return a, nil
}

func (a *executionAgent) Start(ctx context.Context) error {
	a.Subscribe(a.cfg.Symbol, exchange.MDSnapshot)
	return a.BaseActor.Start(ctx)
}

func (a *executionAgent) HandleEvent(_ context.Context, event *actor.Event) {
	a.observeReceipt(event)
	switch event.Type {
	case actor.EventBookSnapshot:
		snapshot := event.Data.(actor.BookSnapshotEvent)
		if snapshot.Symbol != a.cfg.Symbol || snapshot.Snapshot == nil {
			return
		}
		twoSided := len(snapshot.Snapshot.Bids) != 0 && len(snapshot.Snapshot.Asks) != 0
		if a.processingDelay == 0 {
			if twoSided {
				a.bestBid = snapshot.Snapshot.Bids[0].Price
				a.bestAsk = snapshot.Snapshot.Asks[0].Price
			}
			if a.emitProcessingCompletion && a.observe != nil {
				processed := SnapshotProcessingComplete{SeqNum: snapshot.SeqNum, ReceivedAt: a.observationTime(),
					ProcessedAt: a.observationTime(), TwoSided: twoSided}
				if twoSided {
					processed.BestBid, processed.BestAsk = a.bestBid, a.bestAsk
				}
				a.observe(EvidenceObservation{Timestamp: processed.ProcessedAt, ClientID: a.ID(), Source: "actor",
					Name: "snapshot_processing_complete", Payload: processed})
			}
			return
		}
		receivedAt := a.observationTime()
		pending := pendingProcessingSnapshot{
			sequence: snapshot.SeqNum, receivedAt: receivedAt,
			readyAt: receivedAt + int64(a.processingDelay), twoSided: twoSided,
		}
		if twoSided {
			pending.bestBid = snapshot.Snapshot.Bids[0].Price
			pending.bestAsk = snapshot.Snapshot.Asks[0].Price
		}
		a.pendingSnapshots = append(a.pendingSnapshots, pending)
	case actor.EventOrderAccepted:
		accepted := event.Data.(actor.OrderAcceptedEvent)
		if child, ok := a.byRequest[accepted.RequestID]; ok {
			a.report.Children[child].OrderID = accepted.OrderID
			a.byOrder[accepted.OrderID] = child
		}
	case actor.EventOrderRejected:
		rejected := event.Data.(actor.OrderRejectedEvent)
		if child, ok := a.byRequest[rejected.RequestID]; ok {
			a.report.Children[child].Rejected = true
			a.report.Children[child].RejectReason = rejected.Reason
			a.report.RejectedChildren++
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.recordFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		cancelled := event.Data.(actor.OrderCancelledEvent)
		if child, ok := a.byOrder[cancelled.OrderID]; ok {
			a.report.Children[child].CancelRemaining = cancelled.RemainingQty
			a.report.TerminalCancels++
		}
	}
}

func (a *executionAgent) onTick(t time.Time) {
	now := t.UnixNano()
	a.processDueSnapshots(now)
	if a.observe != nil {
		a.observe(EvidenceObservation{
			Timestamp: now, ClientID: a.ID(), Source: "actor", Name: "decision_tick",
			Payload: DecisionTick{BestBid: a.bestBid, BestAsk: a.bestAsk, AlreadyDecided: a.decided},
		})
	}
	if !a.decided {
		if now < a.cfg.DecisionAfter.Nanoseconds() || a.bestBid <= 0 || a.bestAsk <= 0 {
			return
		}
		a.decided = true
		a.report.DecisionAt = now
		a.report.DecisionMid = a.bestBid + (a.bestAsk-a.bestBid)/2
		a.sendNext(now)
		return
	}
	if a.cfg.Policy == TWAP && a.sentSlices < a.cfg.SliceCount && now >= a.nextSliceAt {
		a.sendNext(now)
	}
}

func (a *executionAgent) processDueSnapshots(now int64) {
	processed := 0
	for processed < len(a.pendingSnapshots) && a.pendingSnapshots[processed].readyAt <= now {
		snapshot := a.pendingSnapshots[processed]
		if snapshot.twoSided {
			a.bestBid, a.bestAsk = snapshot.bestBid, snapshot.bestAsk
		}
		if a.observe != nil {
			a.observe(EvidenceObservation{
				Timestamp: now, ClientID: a.ID(), Source: "actor", Name: "snapshot_processing_complete",
				Payload: SnapshotProcessingComplete{
					SeqNum: snapshot.sequence, ReceivedAt: snapshot.receivedAt, ProcessedAt: now,
					BestBid: snapshot.bestBid, BestAsk: snapshot.bestAsk, TwoSided: snapshot.twoSided,
				},
			})
		}
		processed++
	}
	a.pendingSnapshots = a.pendingSnapshots[processed:]
}

func (a *executionAgent) sendNext(now int64) {
	remaining := a.cfg.TargetQty - a.submittedQty
	if remaining <= 0 {
		return
	}
	slicesLeft := 1
	if a.cfg.Policy == TWAP {
		slicesLeft = a.cfg.SliceCount - a.sentSlices
	}
	qty := remaining / int64(slicesLeft)
	if remaining%int64(slicesLeft) != 0 {
		qty++
	}
	orderType, timeInForce, price := exchange.Market, exchange.GTC, int64(0)
	if a.cfg.Instruction != nil {
		orderType, timeInForce, price = a.cfg.Instruction.OrderType,
			a.cfg.Instruction.TimeInForce, a.cfg.Instruction.LimitPrice
	}
	requestID := a.SubmitOrderWithTimeInForce(a.cfg.Symbol, a.cfg.Side, orderType, price, qty, timeInForce)
	a.report.Children = append(a.report.Children, ChildReport{
		RequestID:    requestID,
		SentAt:       now,
		RequestedQty: qty,
	})
	a.byRequest[requestID] = len(a.report.Children) - 1
	a.submittedQty += qty
	a.sentSlices++
	a.report.SubmittedChildren++
	if a.cfg.Policy == TWAP && a.sentSlices < a.cfg.SliceCount {
		a.nextSliceAt = now + a.cfg.SliceInterval.Nanoseconds()
	}
}

func (a *executionAgent) recordFill(fill actor.OrderFillEvent) {
	child, ok := a.byOrder[fill.OrderID]
	if !ok || fill.Symbol != a.cfg.Symbol {
		return
	}
	notional, ok := types.TryMulDiv(fill.Qty, fill.Price, a.cfg.BasePrecision)
	if !ok {
		panic("executionlab: representable fill produced unrepresentable notional")
	}
	record := &a.report.Children[child]
	record.FilledQty = checkedAdd(record.FilledQty, fill.Qty, "child filled quantity")
	record.Notional = checkedAdd(record.Notional, notional, "child notional")
	if fill.FeeAsset == a.cfg.QuoteAsset {
		record.QuoteFee = checkedAdd(record.QuoteFee, fill.FeeAmount, "child quote fee")
		a.report.QuoteFees = checkedAdd(a.report.QuoteFees, fill.FeeAmount, "parent quote fee")
	} else if fill.FeeAmount != 0 {
		a.report.UnpricedFeeCount++
	}
	if record.FirstFillAt == 0 || fill.Timestamp < record.FirstFillAt {
		record.FirstFillAt = fill.Timestamp
	}
	if fill.Timestamp > record.LastFillAt {
		record.LastFillAt = fill.Timestamp
	}
	if a.report.FirstVenueFillAt == 0 || fill.Timestamp < a.report.FirstVenueFillAt {
		a.report.FirstVenueFillAt = fill.Timestamp
	}
	if fill.Timestamp > a.report.LastVenueFillAt {
		a.report.LastVenueFillAt = fill.Timestamp
	}
	a.report.FilledQty = checkedAdd(a.report.FilledQty, fill.Qty, "parent filled quantity")
	a.report.Notional = checkedAdd(a.report.Notional, notional, "parent notional")
}

func (a *executionAgent) Report() ExecutionReport {
	return a.reportWithTerminalMid(0)
}

func (a *executionAgent) ReportWithTerminalMid(terminalMid int64) ExecutionReport {
	return a.reportWithTerminalMark(terminalMid, "caller_supplied")
}

func (a *executionAgent) reportWithTerminalMid(terminalMid int64) ExecutionReport {
	return a.reportWithTerminalMark(terminalMid, "")
}

func (a *executionAgent) reportWithTerminalMark(terminalMid int64, source string) ExecutionReport {
	report := a.report
	report.Children = append([]ChildReport(nil), a.report.Children...)
	report.UnfilledQty = report.TargetQty - report.FilledQty
	if report.UnfilledQty < 0 {
		panic("executionlab: filled more than parent target")
	}
	if report.FilledQty != 0 && report.DecisionMid > 0 {
		reference, ok := types.TryMulDiv(report.FilledQty, report.DecisionMid, a.cfg.BasePrecision)
		if !ok {
			panic("executionlab: representable report has unrepresentable filled reference notional")
		}
		if report.Side == exchange.Buy {
			report.Shortfall = checkedAdd(checkedSub(report.Notional, reference, "buy price shortfall"), report.QuoteFees, "buy all-in shortfall")
		} else {
			report.Shortfall = checkedAdd(checkedSub(reference, report.Notional, "sell price shortfall"), report.QuoteFees, "sell all-in shortfall")
		}
		if reference != 0 {
			report.ShortfallBps = float64(report.Shortfall) * 10_000 / float64(reference)
		}
	}

	if terminalMid <= 0 || report.DecisionMid <= 0 || report.UnpricedFeeCount != 0 {
		return report
	}
	targetReference, ok := types.TryMulDiv(report.TargetQty, report.DecisionMid, a.cfg.BasePrecision)
	if !ok {
		panic("executionlab: representable report has unrepresentable target reference notional")
	}
	completionNotional := report.Notional
	if report.UnfilledQty > 0 {
		terminalNotional, ok := types.TryMulDiv(report.UnfilledQty, terminalMid, a.cfg.BasePrecision)
		if !ok {
			panic("executionlab: representable report has unrepresentable terminal completion notional")
		}
		completionNotional = checkedAdd(completionNotional, terminalNotional, "terminal completion notional")
	}
	report.TerminalMid = terminalMid
	report.TerminalMarkSource = source
	report.TargetShortfallValid = true
	if report.Side == exchange.Buy {
		report.TargetShortfall = checkedAdd(checkedSub(completionNotional, targetReference, "buy target price shortfall"), report.QuoteFees, "buy target all-in shortfall")
	} else {
		report.TargetShortfall = checkedAdd(checkedSub(targetReference, completionNotional, "sell target price shortfall"), report.QuoteFees, "sell target all-in shortfall")
	}
	if targetReference != 0 {
		report.TargetShortfallBps = float64(report.TargetShortfall) * 10_000 / float64(targetReference)
	}
	return report
}

func checkedAdd(a, b int64, field string) int64 {
	value, ok := types.TryAdd(a, b)
	if !ok {
		panic("executionlab: " + field + " overflows int64")
	}
	return value
}

func checkedSub(a, b int64, field string) int64 {
	value, ok := types.TrySub(a, b)
	if !ok {
		panic("executionlab: " + field + " overflows int64")
	}
	return value
}
