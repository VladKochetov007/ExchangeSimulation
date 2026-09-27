package repeatedspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"exchange_sim/exchange"
	"exchange_sim/experiment/executionpilot"
	"exchange_sim/simulation"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type captureContract struct {
	VenueID                          string `json:"venue_id"`
	RecordSnapshotProjectionEvidence bool   `json:"record_snapshot_projection_evidence"`
	Instrument                       struct {
		Symbol string `json:"symbol"`
	} `json:"instrument"`
	Participants []struct {
		ActorID  uint64 `json:"actor_id"`
		ClientID uint64 `json:"client_id"`
		Role     string `json:"role"`
	} `json:"participants"`
}

// Capture joins the exchange's canonical event order with the existing
// participant-information sidecars. Attach it to a fresh World before Run.
type Capture struct {
	world    *worldspot.World
	recorder *Recorder
	receipts *simulation.MarketDataReceiptRecorder
	symbol   string
	run      bool
}

func NewCapture(world *worldspot.World, stream io.Writer, receipts *simulation.MarketDataReceiptRecorder) (*Capture, error) {
	if world == nil || stream == nil || receipts == nil {
		return nil, errors.New("repeated spot: world, stream and receipt recorder are required")
	}
	contractBytes := world.ContractJSON()
	if err := executionpilot.ValidateStrictJSON(contractBytes); err != nil {
		return nil, fmt.Errorf("repeated spot: invalid world contract: %w", err)
	}
	var contract captureContract
	if err := json.Unmarshal(contractBytes, &contract); err != nil {
		return nil, err
	}
	actors := world.Actors()
	if contract.VenueID == "" || contract.Instrument.Symbol == "" || !contract.RecordSnapshotProjectionEvidence || len(actors) != len(contract.Participants) {
		return nil, errors.New("repeated spot: incomplete world contract or actor roster")
	}
	if world.Venue().Loggers[contract.Instrument.Symbol] != nil || world.Venue().Loggers["_global"] != nil {
		return nil, errors.New("repeated spot: existing venue logger would be overwritten")
	}
	gateways := make([]*simulation.DelayedGateway, len(actors))
	orderObservers := make([]interface{ SetOrderDecisionObserver(func(exchange.Request)) }, len(actors))
	cancelObservers := make([]interface{ SetCancelDecisionObserver(func(exchange.Request)) }, len(actors))
	for index, participant := range contract.Participants {
		if participant.ActorID != actors[index].ID() || participant.ClientID == 0 || participant.Role == "" {
			return nil, fmt.Errorf("repeated spot: actor/contract identity mismatch at slot %d", index)
		}
		gateway, ok := actors[index].Gateway().(*simulation.DelayedGateway)
		if !ok || !gateway.SimulationClockConfigured() || gateway.ID() != participant.ClientID {
			return nil, fmt.Errorf("repeated spot: actor %d has no deterministic delayed gateway", participant.ActorID)
		}
		gateways[index] = gateway
		observer, ok := actors[index].(interface{ SetOrderDecisionObserver(func(exchange.Request)) })
		if !ok {
			return nil, fmt.Errorf("repeated spot: actor %d cannot expose outbound order decisions", participant.ActorID)
		}
		orderObservers[index] = observer
		cancelObserver, ok := actors[index].(interface{ SetCancelDecisionObserver(func(exchange.Request)) })
		if !ok {
			return nil, fmt.Errorf("repeated spot: actor %d cannot expose outbound cancellations", participant.ActorID)
		}
		cancelObservers[index] = cancelObserver
	}

	capture := &Capture{world: world, recorder: NewRecorder(stream), receipts: receipts, symbol: contract.Instrument.Symbol}
	world.Venue().SetLogger(contract.Instrument.Symbol, ExchangeLogger{Recorder: capture.recorder, Route: contract.Instrument.Symbol})
	world.Venue().SetLogger("_global", ExchangeLogger{Recorder: capture.recorder, Route: "_global"})
	for index, participant := range contract.Participants {
		gateway := gateways[index]
		clientID := participant.ClientID
		recordOutbound := func(request exchange.Request) {
			capture.recorder.Record(world.SimulatedNowUnixNano(), clientID, "actor", "order_send", contract.Instrument.Symbol, request)
		}
		orderObservers[index].SetOrderDecisionObserver(recordOutbound)
		cancelObservers[index].SetCancelDecisionObserver(recordOutbound)
		gateway.SetMarketDataReceiptRecorder(receipts, contract.VenueID,
			fmt.Sprintf("%s/client/%d", contract.VenueID, participant.ClientID), participant.Role)
		if maker, ok := actors[index].(*worldspot.RecurringMaker); ok {
			maker.SetDecisionObserver(func(decision worldspot.MakerDecision) {
				capture.recorder.Record(decision.DecisionAt, clientID, "actor", "maker_decision", contract.Instrument.Symbol, decision)
			})
			maker.SetObservationObserver(func(observation worldspot.MakerObservation) {
				capture.recorder.Record(observation.ProcessedAt, clientID, "actor", "maker_observation", contract.Instrument.Symbol, observation)
			})
		}
	}
	return capture, nil
}

// Run retains an explicit failure boundary when the world or a sidecar fails.
// A completed stream is not, by itself, a valid economic result: the replay
// must also accept its identities and accounting before scoring.
func (capture *Capture) Run(ctx context.Context) (EvidenceIdentity, error) {
	if capture.run {
		return EvidenceIdentity{}, errors.New("repeated spot: capture can run only once")
	}
	capture.run = true
	start := capture.world.SimulatedNowUnixNano()
	capture.recorder.Record(start, 0, "control", "world_begin", "", map[string]any{
		"contract_sha256": capture.world.ContractSHA256(),
	})
	capture.world.Venue().LogAllBalances()
	capture.recorder.Record(start, 0, "control", "world_run_start", "", struct{}{})
	runErr := capture.world.Run(ctx)
	terminalAt := capture.world.SimulatedNowUnixNano()
	if runErr == nil {
		capture.recorder.Record(terminalAt, 0, "control", "world_run_end", "", struct{}{})
	} else {
		capture.recorder.Record(terminalAt, 0, "control", "world_run_failed", "", map[string]string{"error": runErr.Error()})
	}
	capture.world.Venue().LogAllBalances()
	book := capture.world.Venue().GetBook(capture.symbol)
	if book == nil {
		capture.recorder.Record(terminalAt, 0, "control", "terminal_book_unavailable", "", map[string]string{"symbol": capture.symbol})
	} else {
		capture.recorder.Record(terminalAt, 0, "control", "terminal_book", "", struct {
			Symbol string                `json:"symbol"`
			Bids   []exchange.PriceLevel `json:"bids"`
			Asks   []exchange.PriceLevel `json:"asks"`
		}{capture.symbol, book.Bids.GetPublicSnapshot(), book.Asks.GetPublicSnapshot()})
	}
	receiptErr := capture.receipts.Finalize(terminalAt)
	if runErr != nil || receiptErr != nil {
		capture.recorder.Record(terminalAt, 0, "control", "world_failed", "", map[string]string{
			"run_error": fmt.Sprint(runErr), "receipt_error": fmt.Sprint(receiptErr),
		})
	} else {
		capture.recorder.Record(terminalAt, 0, "control", "world_end", "", struct{}{})
	}
	identity, evidenceErr := capture.recorder.Finish()
	return identity, errors.Join(runErr, receiptErr, evidenceErr)
}
