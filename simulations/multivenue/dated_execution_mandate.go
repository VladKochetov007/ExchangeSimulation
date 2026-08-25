package multivenue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

const datedExecutionMandatePolicyVersion = "v2_5_p5_dated_execution_mandate_v1"

// DatedExecutionMandateConfig declares a finite end-user execution objective.
// It contains no valuation or basis target: an ordinary IOC child is attempted
// against the participant's delivered book until the parent horizon ends.
type DatedExecutionMandateConfig struct {
	Enabled           bool                                `json:"enabled"`
	Underlying        string                              `json:"underlying"`
	TargetTenor       time.Duration                       `json:"target_tenor_nanos"`
	Side              string                              `json:"side"`
	ParentQty         int64                               `json:"parent_qty"`
	ChildQty          int64                               `json:"child_qty"`
	StartDelay        time.Duration                       `json:"start_delay_nanos"`
	ExecutionDuration time.Duration                       `json:"execution_duration_nanos"`
	DecisionPeriod    time.Duration                       `json:"decision_period_nanos"`
	DecisionPhase     time.Duration                       `json:"decision_phase_offset_nanos"`
	MaxMarketAge      time.Duration                       `json:"max_market_age_nanos"`
	SlippageBps       int64                               `json:"slippage_bps"`
	TickSize          int64                               `json:"tick_size"`
	VenueID           string                              `json:"-"`
	Desk              string                              `json:"-"`
	ClientID          uint64                              `json:"-"`
	DecisionObserver  func(DatedExecutionMandateDecision) `json:"-"`
	OutcomeObserver   func(DatedExecutionMandateOutcome)  `json:"-"`
}

func (c DatedExecutionMandateConfig) validate() error {
	if c.Underlying == "" {
		return errors.New("underlying is required")
	}
	if c.Side != exchange.Buy.String() && c.Side != exchange.Sell.String() {
		return fmt.Errorf("side must be BUY or SELL, got %q", c.Side)
	}
	if c.TargetTenor <= 0 || c.ParentQty <= 0 || c.ChildQty <= 0 || c.ExecutionDuration <= 0 || c.DecisionPeriod <= 0 || c.MaxMarketAge <= 0 || c.TickSize <= 0 {
		return errors.New("tenor, quantities, duration, period, market age, and tick must be positive")
	}
	if c.StartDelay < 0 || c.DecisionPhase < 0 || c.DecisionPhase >= c.DecisionPeriod {
		return errors.New("start delay and decision phase are invalid")
	}
	if c.ChildQty > c.ParentQty {
		return errors.New("child quantity exceeds parent quantity")
	}
	if c.SlippageBps < 0 || c.SlippageBps > 10_000 {
		return errors.New("slippage bps must be in [0,10000]")
	}
	return nil
}

type DatedExecutionMandateDecision struct {
	VenueID       string `json:"venue_id"`
	Desk          string `json:"desk"`
	ClientID      uint64 `json:"client_id"`
	PolicyVersion string `json:"policy_version"`
	DecisionTime  int64  `json:"decision_time"`
	Action        string `json:"action_or_defer_reason"`
	Enabled       bool   `json:"enabled"`
	Subscribed    bool   `json:"subscribed"`

	Symbol                      string `json:"symbol"`
	Underlying                  string `json:"underlying"`
	Side                        string `json:"side"`
	ListedNano                  int64  `json:"listed_nano"`
	ExpiryNano                  int64  `json:"expiry_nano"`
	OriginalTenorNanos          int64  `json:"original_tenor_nanos"`
	ListingPublishedAt          int64  `json:"listing_published_at"`
	ListingSequence             uint64 `json:"listing_sequence"`
	ListingFingerprint          string `json:"listing_fingerprint"`
	ParentQty                   int64  `json:"parent_qty"`
	FilledQty                   int64  `json:"filled_qty"`
	RemainingQty                int64  `json:"remaining_qty"`
	StartAt                     int64  `json:"start_at"`
	EndAt                       int64  `json:"end_at"`
	HasBook                     bool   `json:"has_book"`
	BookPublishedAt             int64  `json:"book_published_at"`
	BookSequence                uint64 `json:"book_sequence"`
	HasBid                      bool   `json:"has_bid"`
	Bid                         int64  `json:"bid"`
	BidQty                      int64  `json:"bid_qty"`
	HasAsk                      bool   `json:"has_ask"`
	Ask                         int64  `json:"ask"`
	AskQty                      int64  `json:"ask_qty"`
	MarketAgeNanos              int64  `json:"market_age_nanos"`
	LimitPrice                  int64  `json:"limit_price"`
	RequestedQty                int64  `json:"requested_qty"`
	RequestID                   uint64 `json:"request_id"`
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`
}

type DatedExecutionMandateOutcome struct {
	VenueID         string `json:"venue_id"`
	Desk            string `json:"desk"`
	ClientID        uint64 `json:"client_id"`
	DecisionTime    int64  `json:"decision_time"`
	ExecutionTime   int64  `json:"execution_time"`
	Event           string `json:"event"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	RequestID       uint64 `json:"request_id"`
	OrderID         uint64 `json:"order_id"`
	TradeID         uint64 `json:"trade_id"`
	Qty             int64  `json:"qty"`
	Price           int64  `json:"price"`
	FeeAmount       int64  `json:"fee_amount"`
	FeeAsset        string `json:"fee_asset"`
	HasSettlement   bool   `json:"has_settlement"`
	SettlementPrice int64  `json:"settlement_price"`
	RemainingQty    int64  `json:"remaining_qty"`
	RejectReason    string `json:"reject_reason"`
}

type datedMandateContract struct {
	symbol, underlying           string
	listedAt, expiryAt           int64
	listingPublishedAt           int64
	listingSequence              uint64
	listingFingerprint           string
	startAt, endAt               int64
	filled                       int64
	book                         fundingCarryBook
	pendingRequest, pendingOrder uint64
	pendingDecision              int64
}

type DatedExecutionMandate struct {
	*actor.BaseActor
	cfg        DatedExecutionMandateConfig
	contracts  map[string]*datedMandateContract
	requestSym map[uint64]string
	orderSym   map[uint64]string
	subscribed bool
}

func NewDatedExecutionMandate(id uint64, gateway actor.Gateway, cfg DatedExecutionMandateConfig) *DatedExecutionMandate {
	m := &DatedExecutionMandate{
		BaseActor: actor.NewBaseActor(id, gateway), cfg: cfg,
		contracts: make(map[string]*datedMandateContract), requestSym: make(map[uint64]string), orderSym: make(map[uint64]string),
	}
	m.SetHandler(m)
	m.AddTickerWithOffset(cfg.DecisionPeriod, cfg.DecisionPhase, m.onTick)
	return m
}

func (m *DatedExecutionMandate) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventInstrument:
		m.observeInstrument(event.Data.(actor.InstrumentEvent))
	case actor.EventBookSnapshot:
		m.observeBook(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		m.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		m.onRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		m.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		m.onCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (m *DatedExecutionMandate) observeInstrument(event actor.InstrumentEvent) {
	announcement := event.Announcement
	if announcement == nil || announcement.Underlying != m.cfg.Underlying || announcement.InstrumentType != "FUTURE" {
		return
	}
	if announcement.Action == "settled" {
		if contract := m.contracts[announcement.Symbol]; contract != nil {
			hasSettlement := announcement.SettlementPrice != nil
			settlementPrice := int64(0)
			if hasSettlement {
				settlementPrice = *announcement.SettlementPrice
			}
			m.emitOutcome(contract, "CONTRACT_SETTLED", event.Timestamp, contract.pendingRequest, contract.pendingOrder, 0, 0, 0, 0, "", "", hasSettlement, settlementPrice)
			m.clearPending(contract)
			delete(m.contracts, announcement.Symbol)
		}
		return
	}
	if announcement.Action != "listed" || announcement.ListedNano == nil {
		return
	}
	tenor, ok := etypes.TrySub(announcement.ExpiryNano, *announcement.ListedNano)
	if !ok || tenor != int64(m.cfg.TargetTenor) {
		return
	}
	start, ok := etypes.TryAdd(*announcement.ListedNano, int64(m.cfg.StartDelay))
	if !ok {
		return
	}
	end, ok := etypes.TryAdd(start, int64(m.cfg.ExecutionDuration))
	if !ok || end >= announcement.ExpiryNano {
		return
	}
	if _, exists := m.contracts[announcement.Symbol]; exists {
		return
	}
	fingerprint, ok := datedInstrumentFingerprint(event)
	if !ok {
		return
	}
	m.contracts[announcement.Symbol] = &datedMandateContract{
		symbol: announcement.Symbol, underlying: announcement.Underlying,
		listedAt: *announcement.ListedNano, expiryAt: announcement.ExpiryNano,
		listingPublishedAt: event.Timestamp, listingSequence: event.SeqNum, listingFingerprint: fingerprint,
		startAt: start, endAt: end,
	}
	m.Subscribe(announcement.Symbol, exchange.MDSnapshot)
}

func (m *DatedExecutionMandate) observeBook(event actor.BookSnapshotEvent) {
	contract := m.contracts[event.Symbol]
	if contract == nil {
		return
	}
	book := fundingCarryBook{hasSnapshot: true, publishedAt: event.Timestamp, sequence: event.SeqNum}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.hasBid, book.bid, book.bidQty = true, event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.hasAsk, book.ask, book.askQty = true, event.Snapshot.Asks[0].Price, event.Snapshot.Asks[0].VisibleQty
		}
	}
	contract.book = book
}

func (m *DatedExecutionMandate) onTick(now time.Time) {
	if !m.subscribed {
		decision := m.baseDecision(now.UnixNano(), nil, "NOT_SUBSCRIBED")
		m.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		m.subscribed = true
		m.emitDecision(decision)
		return
	}
	contract := m.nextContract()
	if contract == nil {
		m.emitDecision(m.baseDecision(now.UnixNano(), nil, "NO_ELIGIBLE_CONTRACT"))
		return
	}
	decision := m.decide(now.UnixNano(), contract)
	m.emitDecision(decision)
	if decision.RequestID == 0 {
		return
	}
	requestID := m.SubmitOrderWithTimeInForce(contract.symbol, mandateSide(m.cfg.Side), exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
	if requestID != decision.RequestID {
		panic(fmt.Sprintf("dated execution mandate request ID changed from %d to %d", decision.RequestID, requestID))
	}
	contract.pendingRequest, contract.pendingDecision = requestID, decision.DecisionTime
	m.requestSym[requestID] = contract.symbol
}

func (m *DatedExecutionMandate) nextContract() *datedMandateContract {
	contracts := make([]*datedMandateContract, 0, len(m.contracts))
	for _, contract := range m.contracts {
		contracts = append(contracts, contract)
	}
	slices.SortFunc(contracts, func(a, b *datedMandateContract) int {
		if byExpiry := cmp.Compare(a.expiryAt, b.expiryAt); byExpiry != 0 {
			return byExpiry
		}
		return cmp.Compare(a.symbol, b.symbol)
	})
	for _, contract := range contracts {
		if contract.filled < m.cfg.ParentQty {
			return contract
		}
	}
	return nil
}

func (m *DatedExecutionMandate) decide(now int64, contract *datedMandateContract) DatedExecutionMandateDecision {
	decision := m.baseDecision(now, contract, "")
	if !m.cfg.Enabled {
		decision.Action = "POLICY_DISABLED"
		return decision
	}
	if contract.pendingRequest != 0 {
		decision.Action = "CHILD_PENDING"
		return decision
	}
	if now < contract.startAt {
		decision.Action = "BEFORE_START"
		return decision
	}
	if now >= contract.endAt {
		decision.Action = "PARENT_HORIZON_EXPIRED"
		return decision
	}
	if !contract.book.hasSnapshot {
		decision.Action = "BOOK_UNAVAILABLE"
		return decision
	}
	age, ok := etypes.TrySub(now, contract.book.publishedAt)
	if !ok || age < 0 {
		decision.Action = "BOOK_PUBLICATION_FUTURE"
		return decision
	}
	decision.MarketAgeNanos = age
	if age > int64(m.cfg.MaxMarketAge) {
		decision.Action = "BOOK_STALE"
		return decision
	}
	touch := contract.book.ask
	if m.cfg.Side == exchange.Sell.String() {
		touch = contract.book.bid
	}
	if (m.cfg.Side == exchange.Buy.String() && !contract.book.hasAsk) || (m.cfg.Side == exchange.Sell.String() && !contract.book.hasBid) {
		decision.Action = "EXECUTABLE_TOUCH_UNAVAILABLE"
		return decision
	}
	limit := rebalanceOutwardLimit(touch, m.cfg.SlippageBps, m.cfg.TickSize, mandateSide(m.cfg.Side))
	if limit <= 0 {
		decision.Action = "LIMIT_PRICE_OUTSIDE_DOMAIN"
		return decision
	}
	remaining, ok := etypes.TrySub(m.cfg.ParentQty, contract.filled)
	if !ok || remaining <= 0 {
		decision.Action = "PARENT_COMPLETE"
		return decision
	}
	quantity := minInt64(remaining, m.cfg.ChildQty)
	decision.Action, decision.LimitPrice, decision.RequestedQty, decision.RequestID = "SUBMIT_CHILD_IOC", limit, quantity, m.PeekNextRequestID()
	return decision
}

func (m *DatedExecutionMandate) baseDecision(now int64, contract *datedMandateContract, action string) DatedExecutionMandateDecision {
	frontier := fundingCarryFrontier(m.Gateway())
	decision := DatedExecutionMandateDecision{
		VenueID: m.cfg.VenueID, Desk: m.cfg.Desk, ClientID: m.cfg.ClientID, PolicyVersion: datedExecutionMandatePolicyVersion,
		DecisionTime: now, Action: action, Enabled: m.cfg.Enabled, Subscribed: m.subscribed,
		Underlying: m.cfg.Underlying, Side: m.cfg.Side, ParentQty: m.cfg.ParentQty,
		DecisionFrontierLinkID: frontier.LinkID, DecisionFrontierOrdinal: frontier.Ordinal,
		DecisionFrontierDeliveredAt: frontier.DeliveredAt, DecisionFrontierDigest: fmt.Sprintf("%x", frontier.Digest),
	}
	if contract == nil {
		return decision
	}
	remaining, _ := etypes.TrySub(m.cfg.ParentQty, contract.filled)
	decision.Symbol, decision.ListedNano, decision.ExpiryNano = contract.symbol, contract.listedAt, contract.expiryAt
	decision.OriginalTenorNanos = contract.expiryAt - contract.listedAt
	decision.ListingPublishedAt, decision.ListingSequence = contract.listingPublishedAt, contract.listingSequence
	decision.ListingFingerprint = contract.listingFingerprint
	decision.FilledQty, decision.RemainingQty, decision.StartAt, decision.EndAt = contract.filled, remaining, contract.startAt, contract.endAt
	book := contract.book
	decision.HasBook, decision.BookPublishedAt, decision.BookSequence = book.hasSnapshot, book.publishedAt, book.sequence
	decision.HasBid, decision.Bid, decision.BidQty = book.hasBid, book.bid, book.bidQty
	decision.HasAsk, decision.Ask, decision.AskQty = book.hasAsk, book.ask, book.askQty
	return decision
}

func (m *DatedExecutionMandate) onAccepted(event actor.OrderAcceptedEvent) {
	symbol := m.requestSym[event.RequestID]
	contract := m.contracts[symbol]
	if contract == nil || contract.pendingRequest != event.RequestID {
		return
	}
	contract.pendingOrder = event.OrderID
	m.orderSym[event.OrderID] = symbol
	m.emitOutcome(contract, "ORDER_ACCEPTED", 0, event.RequestID, event.OrderID, 0, 0, 0, 0, "", "", false, 0)
}

func (m *DatedExecutionMandate) onRejected(event actor.OrderRejectedEvent) {
	symbol := m.requestSym[event.RequestID]
	contract := m.contracts[symbol]
	if contract == nil || contract.pendingRequest != event.RequestID {
		return
	}
	m.emitOutcome(contract, "ORDER_REJECTED", 0, event.RequestID, 0, 0, 0, 0, 0, "", string(event.Reason), false, 0)
	m.clearPending(contract)
}

func (m *DatedExecutionMandate) onFill(event actor.OrderFillEvent) {
	symbol := m.orderSym[event.OrderID]
	contract := m.contracts[symbol]
	if contract == nil || event.Symbol != symbol || event.Side != mandateSide(m.cfg.Side) {
		return
	}
	filled, ok := etypes.TryAdd(contract.filled, event.Qty)
	if !ok || filled > m.cfg.ParentQty {
		panic("dated execution mandate fill exceeds parent")
	}
	contract.filled = filled
	m.emitOutcome(contract, "ORDER_FILL", event.Timestamp, contract.pendingRequest, event.OrderID, event.TradeID, event.Qty, event.Price, event.FeeAmount, event.FeeAsset, "", false, 0)
	if event.IsFull {
		m.clearPending(contract)
	}
}

func (m *DatedExecutionMandate) onCancelled(event actor.OrderCancelledEvent) {
	symbol := m.orderSym[event.OrderID]
	contract := m.contracts[symbol]
	if contract == nil {
		return
	}
	m.emitOutcome(contract, "ORDER_CANCELLED", 0, contract.pendingRequest, event.OrderID, 0, 0, 0, 0, "", "", false, 0)
	m.clearPending(contract)
}

func (m *DatedExecutionMandate) clearPending(contract *datedMandateContract) {
	delete(m.requestSym, contract.pendingRequest)
	delete(m.orderSym, contract.pendingOrder)
	contract.pendingRequest, contract.pendingOrder, contract.pendingDecision = 0, 0, 0
}

func (m *DatedExecutionMandate) emitDecision(decision DatedExecutionMandateDecision) {
	if m.cfg.DecisionObserver != nil {
		m.cfg.DecisionObserver(decision)
	}
}

func (m *DatedExecutionMandate) emitOutcome(contract *datedMandateContract, event string, executionTime int64, requestID, orderID, tradeID uint64, quantity, price, feeAmount int64, feeAsset, reject string, hasSettlement bool, settlementPrice int64) {
	if m.cfg.OutcomeObserver == nil {
		return
	}
	remaining, _ := etypes.TrySub(m.cfg.ParentQty, contract.filled)
	m.cfg.OutcomeObserver(DatedExecutionMandateOutcome{
		VenueID: m.cfg.VenueID, Desk: m.cfg.Desk, ClientID: m.cfg.ClientID,
		DecisionTime: contract.pendingDecision, ExecutionTime: executionTime, Event: event, Symbol: contract.symbol, Side: m.cfg.Side,
		RequestID: requestID, OrderID: orderID, TradeID: tradeID, Qty: quantity, Price: price, FeeAmount: feeAmount, FeeAsset: feeAsset,
		HasSettlement: hasSettlement, SettlementPrice: settlementPrice, RemainingQty: remaining, RejectReason: reject,
	})
}

func mandateSide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}

func datedInstrumentFingerprint(event actor.InstrumentEvent) (string, bool) {
	if event.Announcement == nil {
		return "", false
	}
	digest, err := etypes.MarketDataFingerprint(&etypes.MarketDataMsg{
		Type: exchange.MDInstrument, Symbol: exchange.InstrumentFeedSymbol, SeqNum: event.SeqNum, Timestamp: event.Timestamp, Data: event.Announcement,
	})
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%x", digest), true
}
