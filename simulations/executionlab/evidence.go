package executionlab

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/marketdata"
)

// EvidenceObservation is emitted at a causal boundary, without feeding any
// value back into a participant. Payload must be copied by the receiver before
// the callback returns if it needs to retain it.
type EvidenceObservation struct {
	Timestamp int64
	ClientID  uint64
	Source    string
	Name      string
	Route     string
	Payload   any
}

type evidenceLogger struct {
	observe func(EvidenceObservation)
	route   string
}

func (l evidenceLogger) LogEvent(timestamp int64, clientID uint64, name string, payload any) {
	l.observe(EvidenceObservation{
		Timestamp: timestamp, ClientID: clientID, Source: "exchange",
		Name: name, Route: l.route, Payload: payload,
	})
}

// SetEvidenceObserver must be called before Run. The observer is write-only:
// it may retain evidence, but must not call into the exchange or alter actors.
func (s *Sim) SetEvidenceObserver(observe func(EvidenceObservation)) {
	s.observe = observe
	if observe == nil {
		s.exchange.SetLogger(s.Parent.cfg.Symbol, nil)
		s.exchange.SetLogger("_global", nil)
		s.exchange.MDPublisher.SetPublicationObserver(s.Parent.ID(), nil)
		for _, parent := range s.Parents {
			parent.observe = nil
			parent.SetOrderDecisionObserver(nil)
		}
		return
	}
	s.exchange.SetLogger(s.Parent.cfg.Symbol, evidenceLogger{observe: observe, route: s.Parent.cfg.Symbol})
	s.exchange.SetLogger("_global", evidenceLogger{observe: observe, route: "_global"})
	if s.contract.Config.RecordSnapshotProjectionEvidence {
		s.exchange.MDPublisher.SetPublicationObserver(s.Parent.ID(), func(outcome marketdata.PublicationOutcome) {
			if outcome.Type != exchange.MDSnapshot || outcome.Symbol != s.Parent.cfg.Symbol {
				return
			}
			observe(EvidenceObservation{
				Timestamp: outcome.Timestamp, ClientID: outcome.ClientID,
				Source: "transport", Name: "snapshot_publish_outcome", Route: outcome.Symbol, Payload: outcome,
			})
		})
	}
	for _, parent := range s.Parents {
		parent.observe = observe
		parent.observationTime = s.clock.NowUnixNano
		clientID := parent.ID()
		parent.SetOrderDecisionObserver(func(request exchange.Request) {
			observe(EvidenceObservation{
				Timestamp: s.clock.NowUnixNano(), ClientID: clientID,
				Source: "actor", Name: "order_send", Payload: request,
			})
		})
	}
}

func (a *executionAgent) observeReceipt(event *actor.Event) {
	if a.observe == nil {
		return
	}
	name := ""
	switch event.Type {
	case actor.EventBookSnapshot:
		name = "book_snapshot_receipt"
	case actor.EventOrderAccepted:
		name = "order_accepted_receipt"
	case actor.EventOrderRejected:
		name = "order_rejected_receipt"
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		name = "order_fill_receipt"
	case actor.EventOrderCancelled:
		name = "order_cancelled_receipt"
	}
	if name != "" {
		a.observe(EvidenceObservation{
			Timestamp: a.observationTime(),
			ClientID:  a.ID(), Source: "actor", Name: name, Payload: event.Data,
		})
	}
}

type DecisionTick struct {
	BestBid        int64 `json:"best_bid"`
	BestAsk        int64 `json:"best_ask"`
	AlreadyDecided bool  `json:"already_decided"`
}

type SnapshotProcessingComplete struct {
	SeqNum      uint64 `json:"seq_num"`
	ReceivedAt  int64  `json:"received_at"`
	ProcessedAt int64  `json:"processed_at"`
	BestBid     int64  `json:"best_bid"`
	BestAsk     int64  `json:"best_ask"`
	TwoSided    bool   `json:"two_sided"`
}

type TerminalBook struct {
	Symbol string            `json:"symbol"`
	Bid    exchange.TopLevel `json:"bid"`
	Ask    exchange.TopLevel `json:"ask"`
	Valid  bool              `json:"valid"`
}
