package exchange

import (
	"slices"

	etypes "exchange_sim/types"
)

// sortedAssetNames orders a per-asset map so that applying its entries has the
// same effect every run. Several of the loops below mutate balances and stop at
// the first failure, so the iteration order decides both what state is left
// behind and whether the plan is admitted at all.
func sortedAssetNames[V any](table map[string]V) []string {
	names := make([]string, 0, len(table))
	for asset := range table {
		names = append(names, asset)
	}
	slices.Sort(names)
	return names
}

// spotExecutionPlan freezes the fee quotes for a cloned match. Matching engines
// mutate before settlement, so validating fees after Match is too late to stop
// a multi-fill (especially iceberg) order from spending more than it owns.
//
// The plan is intentionally scoped to instruments that take the exchange's
// spot settlement path. Instruments with their own Settleable ledger also
// change position margin and must use a ledger-aware preflight rather than
// borrowing spot accounting assumptions.
type spotExecutionPlan struct {
	fills       []plannedSpotExecution
	fullyFilled bool
}

type plannedSpotExecution struct {
	fingerprint executionFingerprint
	takerFee    Fee
	makerFee    Fee
}

type executionFingerprint struct {
	takerOrderID  uint64
	makerOrderID  uint64
	takerClientID uint64
	makerClientID uint64
	price         int64
	qty           int64
	takerFilled   int64
	makerFilled   int64
	makerTotal    int64
	makerSide     Side
	makerPosSide  PositionSide
}

func newExecutionFingerprint(exec *Execution) executionFingerprint {
	return executionFingerprint{
		takerOrderID:  exec.TakerOrderID,
		makerOrderID:  exec.MakerOrderID,
		takerClientID: exec.TakerClientID,
		makerClientID: exec.MakerClientID,
		price:         exec.Price,
		qty:           exec.Qty,
		takerFilled:   exec.TakerFilledQty,
		makerFilled:   exec.MakerFilledQty,
		makerTotal:    exec.MakerTotalQty,
		makerSide:     exec.MakerSide,
		makerPosSide:  exec.MakerPosSide,
	}
}

func (f executionFingerprint) matches(exec *Execution) bool {
	return exec != nil && f == newExecutionFingerprint(exec)
}

func (p *spotExecutionPlan) matches(executions []*Execution) bool {
	if p == nil || len(p.fills) != len(executions) {
		return false
	}
	for i, fill := range p.fills {
		if !fill.fingerprint.matches(executions[i]) {
			return false
		}
	}
	return true
}

type spotPlanFailure struct {
	// makerOrderID is set only when a resting maker is unable to fund the
	// exact next batch. The caller can remove that stale maker and re-plan.
	// A zero value means the incoming taker or an invariant failed, so the
	// incoming order must be rejected without touching the live book.
	makerOrderID uint64
	// err is reserved for an action that could not be preflighted because an
	// explicitly configured price-dependent fee source was unavailable. It is
	// not an insufficiency result: the caller must surface it as such before
	// allowing the matcher to mutate the live book.
	err error
}

// prepareFeeExecutionPlan freezes the exact fee quote for every execution in
// a detached match. It deliberately contains no spot-ledger assumptions, so
// margined and settleable instruments use the same pre-match fee boundary.
//
// Every live execution must have a member of this plan. In particular, a
// configured OptionFee source is consulted before Match, never after a fill
// has already changed balances or positions.
func (e *DefaultExchange) prepareFeeExecutionPlan(book *OrderBook, takerOrder *Order, excluded map[uint64]struct{}) (*spotExecutionPlan, *spotPlanFailure) {
	result, ok := e.previewMatchExcluding(book, takerOrder, excluded)
	if !ok {
		return nil, &spotPlanFailure{}
	}
	defer releasePreviewExecutions(result.Executions)

	plan := &spotExecutionPlan{
		fills:       make([]plannedSpotExecution, 0, len(result.Executions)),
		fullyFilled: result.FullyFilled,
	}
	for _, exec := range result.Executions {
		if exec == nil || exec.TakerOrderID != takerOrder.ID {
			return nil, &spotPlanFailure{}
		}
		makerOrder := book.FindOrder(exec.MakerOrderID)
		if makerOrder == nil || makerOrder.ClientID != exec.MakerClientID || makerOrder.Side != exec.MakerSide || makerOrder.PositionSide != exec.MakerPosSide || exec.MakerTotalQty != makerOrder.Qty {
			return nil, &spotPlanFailure{}
		}
		takerFee, err := calcClientFee(e.Clients[exec.TakerClientID], FillContext{
			Exec: exec, IsMaker: false, BaseAsset: book.Instrument.BaseAsset(), QuoteAsset: book.Instrument.QuoteAsset(), Precision: book.Instrument.BasePrecision(),
		})
		if err != nil {
			return nil, &spotPlanFailure{err: err}
		}
		makerFee, err := calcClientFee(e.Clients[exec.MakerClientID], FillContext{
			Exec: exec, IsMaker: true, BaseAsset: book.Instrument.BaseAsset(), QuoteAsset: book.Instrument.QuoteAsset(), Precision: book.Instrument.BasePrecision(),
		})
		if err != nil {
			return nil, &spotPlanFailure{err: err}
		}
		plan.fills = append(plan.fills, plannedSpotExecution{
			fingerprint: newExecutionFingerprint(exec),
			takerFee:    takerFee,
			makerFee:    makerFee,
		})
	}
	return plan, nil
}

// prepareSpotExecutionPlan runs the configured matcher on a detached book,
// quotes every fee exactly once, and simulates the real reservation releases
// and settlement cash flows. The simulation starts from available balances,
// then only unlocks reservations owned by an order when that order's first
// planned execution reaches settlement. This prevents one order from spending
// funds locked by another order belonging to the same client.
func (e *DefaultExchange) prepareSpotExecutionPlan(book *OrderBook, takerOrder *Order, excluded map[uint64]struct{}, plan *spotExecutionPlan) (*spotExecutionPlan, *spotPlanFailure) {
	if plan == nil {
		var failure *spotPlanFailure
		plan, failure = e.prepareFeeExecutionPlan(book, takerOrder, excluded)
		if failure != nil {
			return nil, failure
		}
	}
	result, ok := e.previewMatchExcluding(book, takerOrder, excluded)
	if !ok {
		return nil, &spotPlanFailure{}
	}
	defer releasePreviewExecutions(result.Executions)
	if !plan.matches(result.Executions) || plan.fullyFilled != result.FullyFilled {
		return nil, &spotPlanFailure{}
	}
	orders := map[uint64]*Order{takerOrder.ID: takerOrder}
	finalFilled := map[uint64]int64{takerOrder.ID: takerOrder.FilledQty}
	makerFilled := make(map[uint64]int64)
	for i, exec := range result.Executions {
		if exec == nil || exec.TakerOrderID != takerOrder.ID {
			return nil, &spotPlanFailure{}
		}
		makerOrder := book.FindOrder(exec.MakerOrderID)
		if makerOrder == nil || makerOrder.ClientID != exec.MakerClientID || makerOrder.Side != exec.MakerSide || makerOrder.PositionSide != exec.MakerPosSide || exec.MakerTotalQty != makerOrder.Qty {
			return nil, &spotPlanFailure{}
		}
		nextTakerFilled, ok := etypes.TryAdd(finalFilled[takerOrder.ID], exec.Qty)
		if !ok || nextTakerFilled != exec.TakerFilledQty || nextTakerFilled > takerOrder.Qty {
			return nil, &spotPlanFailure{}
		}
		currentMakerFilled, seen := makerFilled[makerOrder.ID]
		if !seen {
			currentMakerFilled = makerOrder.FilledQty
		}
		nextMakerFilled, ok := etypes.TryAdd(currentMakerFilled, exec.Qty)
		if !ok || nextMakerFilled != exec.MakerFilledQty || nextMakerFilled > makerOrder.Qty {
			return nil, &spotPlanFailure{}
		}
		orders[makerOrder.ID] = makerOrder
		finalFilled[takerOrder.ID] = nextTakerFilled
		finalFilled[makerOrder.ID] = nextMakerFilled
		makerFilled[makerOrder.ID] = nextMakerFilled
		if !plan.fills[i].fingerprint.matches(exec) {
			// The first detached match supplied the fee plan. A second detached
			// match while the exchange lock is held must be identical; accepting a
			// changed sequence would make the price-dependent fee quote stale.
			return nil, &spotPlanFailure{}
		}
	}

	state := newSpotPlanState(e.Clients)
	initialized := make(map[uint64]*spotPlanOrder, len(orders))
	initialize := func(order *Order, maker bool) (*spotPlanOrder, *spotPlanFailure) {
		if adjustment := initialized[order.ID]; adjustment != nil {
			return adjustment, nil
		}
		adjustment, ok, err := state.beginOrder(book, order, finalFilled[order.ID])
		if err != nil {
			return nil, &spotPlanFailure{err: err}
		}
		if !ok {
			if maker {
				return nil, &spotPlanFailure{makerOrderID: order.ID}
			}
			return nil, &spotPlanFailure{}
		}
		initialized[order.ID] = adjustment
		return adjustment, nil
	}

	for i, fill := range plan.fills {
		exec := result.Executions[i]
		makerOrder := orders[exec.MakerOrderID]
		takerAdjustment, failed := initialize(takerOrder, false)
		if failed != nil {
			return nil, failed
		}
		makerAdjustment, failed := initialize(makerOrder, true)
		if failed != nil {
			return nil, failed
		}
		if !state.applyExecution(book.Instrument, exec.TakerClientID, takerOrder.Side, exec.Qty, exec.Price, fill.takerFee) {
			return nil, &spotPlanFailure{}
		}
		if !state.applyExecution(book.Instrument, exec.MakerClientID, exec.MakerSide, exec.Qty, exec.Price, fill.makerFee) {
			return nil, &spotPlanFailure{makerOrderID: makerOrder.ID}
		}
		if !state.finishOrder(takerAdjustment) {
			return nil, &spotPlanFailure{}
		}
		if !state.finishOrder(makerAdjustment) {
			return nil, &spotPlanFailure{makerOrderID: makerOrder.ID}
		}
	}
	// Market fee escrow deliberately stays locked during the full live match:
	// restoreForeignFeeReservation releases it only in restOrReleaseOrder.
	// That can make available balance transiently negative even though the
	// atomic batch is solvent, so mirror the terminal release here rather than
	// falsely releasing it on the first execution.
	for _, adjustment := range initialized {
		if adjustment.order.Type != Market {
			continue
		}
		if !state.finishTerminalOrder(adjustment) {
			if adjustment.order.ID == takerOrder.ID {
				return nil, &spotPlanFailure{}
			}
			return nil, &spotPlanFailure{makerOrderID: adjustment.order.ID}
		}
		if !state.validClient(adjustment.order.ClientID) {
			if adjustment.order.ID == takerOrder.ID {
				return nil, &spotPlanFailure{}
			}
			return nil, &spotPlanFailure{makerOrderID: adjustment.order.ID}
		}
	}
	return plan, nil
}

type spotPlanState struct {
	balances map[uint64]map[string]int64
	reserved map[uint64]map[string]int64
	clients  map[uint64]*Client
}

func newSpotPlanState(clients map[uint64]*Client) *spotPlanState {
	return &spotPlanState{
		balances: make(map[uint64]map[string]int64),
		reserved: make(map[uint64]map[string]int64),
		clients:  clients,
	}
}

func (s *spotPlanState) initializeClient(clientID uint64) bool {
	if s.balances[clientID] != nil {
		return true
	}
	client := s.clients[clientID]
	if client == nil {
		return false
	}
	balances := make(map[string]int64, len(client.Balances)+len(client.Reserved))
	reserved := make(map[string]int64, len(client.Balances)+len(client.Reserved))
	for asset, balance := range client.Balances {
		balances[asset] = balance
		reserved[asset] = client.Reserved[asset]
	}
	for asset, amount := range client.Reserved {
		if _, exists := balances[asset]; !exists {
			balances[asset] = 0
		}
		reserved[asset] = amount
	}
	s.balances[clientID] = balances
	s.reserved[clientID] = reserved
	return s.validClient(clientID)
}

func (s *spotPlanState) validClient(clientID uint64) bool {
	balances, reserved := s.balances[clientID], s.reserved[clientID]
	for asset, balance := range balances {
		if balance < 0 || reserved[asset] < 0 || balance < reserved[asset] {
			return false
		}
	}
	for asset, amount := range reserved {
		if amount < 0 || balances[asset] < amount {
			return false
		}
	}
	return true
}

func (s *spotPlanState) release(clientID uint64, asset string, amount int64) bool {
	if amount < 0 || !s.initializeClient(clientID) || s.reserved[clientID][asset] < amount {
		return false
	}
	s.reserved[clientID][asset] -= amount
	return true
}

func (s *spotPlanState) reserve(clientID uint64, asset string, amount int64) bool {
	if amount < 0 || !s.initializeClient(clientID) {
		return false
	}
	next, ok := etypes.TryAdd(s.reserved[clientID][asset], amount)
	if !ok || s.balances[clientID][asset] < next {
		return false
	}
	s.reserved[clientID][asset] = next
	return true
}

func (s *spotPlanState) addBalance(clientID uint64, asset string, delta int64) bool {
	if !s.initializeClient(clientID) {
		return false
	}
	next, ok := etypes.TryAdd(s.balances[clientID][asset], delta)
	if !ok || next < 0 {
		return false
	}
	s.balances[clientID][asset] = next
	return true
}

type spotPlanOrder struct {
	order          *Order
	foreignReserve map[string]int64
	finished       bool
}

func (s *spotPlanState) beginOrder(book *OrderBook, order *Order, finalFilled int64) (*spotPlanOrder, bool, error) {
	if order == nil || order.ClientID == 0 || finalFilled < order.FilledQty || finalFilled > order.Qty {
		return nil, false, nil
	}
	remaining, ok := etypes.TrySub(order.Qty, finalFilled)
	if !ok || remaining < 0 {
		return nil, false, nil
	}

	tradeReserve := int64(0)
	foreignReserve := map[string]int64(nil)
	var err error
	if order.Type == LimitOrder && order.TimeInForce == GTC && remaining > 0 {
		client := s.clients[order.ClientID]
		if client == nil {
			return nil, false, nil
		}
		future := *order
		future.Qty = remaining
		future.FilledQty = 0
		tradeReserve, ok, err = spotOrderReservation(client.FeePlan, book.Instrument, order.Side, remaining, order.Price, book.Instrument.BasePrecision())
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		foreignReserve, ok, err = eForeignFeeReservations(client.FeePlan, book, &future, book.Instrument.BasePrecision())
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
	}

	if order.Reserved < 0 || tradeReserve > order.Reserved {
		return nil, false, nil
	}
	tradeRelease, ok := etypes.TrySub(order.Reserved, tradeReserve)
	if !ok || !s.release(order.ClientID, reserveAsset(book.Instrument, order.Side), tradeRelease) {
		return nil, false, nil
	}
	return &spotPlanOrder{order: order, foreignReserve: foreignReserve}, true, nil
}

func (s *spotPlanState) finishOrder(adjustment *spotPlanOrder) bool {
	if adjustment == nil || adjustment.order == nil {
		return false
	}
	order := adjustment.order
	if adjustment.finished {
		return order.Type == Market || s.validClient(order.ClientID)
	}
	if order.Type == Market {
		adjustment.finished = true
		return true
	}
	for _, asset := range sortedAssetNames(order.FeeReserved) {
		if !s.release(order.ClientID, asset, order.FeeReserved[asset]) {
			return false
		}
	}
	for _, asset := range sortedAssetNames(adjustment.foreignReserve) {
		if !s.reserve(order.ClientID, asset, adjustment.foreignReserve[asset]) {
			return false
		}
	}
	if !s.validClient(order.ClientID) {
		return false
	}
	adjustment.finished = true
	return true
}

func (s *spotPlanState) finishTerminalOrder(adjustment *spotPlanOrder) bool {
	if adjustment == nil || adjustment.order == nil || adjustment.order.Type != Market {
		return false
	}
	for _, asset := range sortedAssetNames(adjustment.order.FeeReserved) {
		if !s.release(adjustment.order.ClientID, asset, adjustment.order.FeeReserved[asset]) {
			return false
		}
	}
	return true
}

func (s *spotPlanState) applyExecution(instrument Instrument, clientID uint64, side Side, qty, price int64, fee Fee) bool {
	if qty <= 0 || price <= 0 {
		return false
	}
	notional, ok := etypes.TryMulDiv(qty, price, instrument.BasePrecision())
	if !ok || notional < 0 {
		return false
	}
	deltas := make(map[string]int64, 3)
	addDelta := func(asset string, delta int64) bool {
		current := deltas[asset]
		next, ok := etypes.TryAdd(current, delta)
		if !ok {
			return false
		}
		deltas[asset] = next
		return true
	}
	if side == Buy {
		if !addDelta(instrument.QuoteAsset(), -notional) || !addDelta(instrument.BaseAsset(), qty) {
			return false
		}
	} else if side == Sell {
		if !addDelta(instrument.BaseAsset(), -qty) || !addDelta(instrument.QuoteAsset(), notional) {
			return false
		}
	} else {
		return false
	}
	if fee.Amount != 0 {
		feeDelta, ok := etypes.TrySub(0, fee.Amount)
		if !ok || !addDelta(fee.Asset, feeDelta) {
			return false
		}
	}
	for _, asset := range sortedAssetNames(deltas) {
		if !s.addBalance(clientID, asset, deltas[asset]) {
			return false
		}
	}
	return true
}

// eForeignFeeReservations is the non-method form used by the dry-run state.
// It keeps the future-resting-order reservation formula identical to the live
// path without giving the state helper access to the exchange.
func eForeignFeeReservations(feePlan FeeModel, book *OrderBook, order *Order, precision int64) (map[string]int64, bool, error) {
	if feePlan == nil || order.Qty <= 0 {
		return nil, true, nil
	}
	base, quote := book.Instrument.BaseAsset(), book.Instrument.QuoteAsset()
	reserved := make(map[string]int64)
	for _, isMaker := range []bool{false, true} {
		probe := Execution{Price: order.Price, Qty: order.Qty}
		fee, err := feePlan.CalculateFee(FillContext{Exec: &probe, IsMaker: isMaker, BaseAsset: base, QuoteAsset: quote, Precision: precision})
		if err != nil {
			return nil, false, err
		}
		if fee.Amount <= 0 || fee.Asset == "" || fee.Asset == quote || fee.Asset == base {
			continue
		}
		if fee.Amount > reserved[fee.Asset] {
			reserved[fee.Asset] = fee.Amount
		}
	}
	return reserved, true, nil
}
