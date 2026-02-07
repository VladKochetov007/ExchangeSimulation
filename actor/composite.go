package actor

import (
	"context"
	"sync/atomic"
	"time"

	"exchange_sim/exchange"
)

// symbolLPState tracks per-symbol state for MultiSymbolLP
type symbolLPState struct {
	Symbol        string
	Instrument    exchange.Instrument
	BaseAsset     string
	QuoteAsset    string
	ActiveBidID   uint64
	ActiveAskID   uint64
	lastBidReqID  uint64
	lastAskReqID  uint64
	BidSize       int64
	AskSize       int64
	LastMidPrice  int64
	BestBid       int64
	BestAsk       int64
	BidLiquidity  int64
	AskLiquidity  int64
	NetPosition   int64
	AvgEntryPrice int64
	BootstrapPrice int64
}

// MultiSymbolLPConfig configures a multi-symbol liquidity provider
type MultiSymbolLPConfig struct {
	Symbols         []string                       // All symbols to trade
	Instruments     map[string]exchange.Instrument // Symbol -> Instrument
	SpreadBps       int64                          // Spread for all symbols
	BootstrapPrices map[string]int64               // Symbol -> bootstrap price
	MonitorInterval time.Duration                  // How often to check exit conditions
	LiquidityMultiple int64                        // Exit when market has this multiple
	MinExitSize     int64                          // Minimum position to exit
	SkewFactor      float64                        // Price skew per unit of inventory (bps or ratio)
}


// MultiSymbolLP manages FirstLP logic for multiple symbols with shared balance
type MultiSymbolLP struct {
	*BaseActor
	config       MultiSymbolLPConfig
	symbolStates map[string]*symbolLPState

	// Shared balances across all symbols
	baseBalances map[string]int64 // Asset -> balance
	quoteBalance int64            // Shared quote balance (USD)

	monitorTicker *time.Ticker
	stopCh        chan struct{}
}

// NewMultiSymbolLP creates a new multi-symbol liquidity provider
func NewMultiSymbolLP(id uint64, gateway *exchange.ClientGateway, config MultiSymbolLPConfig) *MultiSymbolLP {
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 100 * time.Millisecond
	}
	if config.LiquidityMultiple == 0 {
		config.LiquidityMultiple = 10
	}

	lp := &MultiSymbolLP{
		BaseActor:    NewBaseActor(id, gateway),
		config:       config,
		symbolStates: make(map[string]*symbolLPState),
		baseBalances: make(map[string]int64),
		stopCh:       make(chan struct{}),
	}

	// Initialize per-symbol state
	for _, symbol := range config.Symbols {
		inst := config.Instruments[symbol]
		if inst == nil {
			continue
		}
		lp.symbolStates[symbol] = &symbolLPState{
			Symbol:         symbol,
			Instrument:     inst,
			BaseAsset:      inst.BaseAsset(),
			QuoteAsset:     inst.QuoteAsset(),
			BootstrapPrice: config.BootstrapPrices[symbol],
		}
	}

	return lp
}

// SetBalances sets the shared balances
func (m *MultiSymbolLP) SetBalances(baseBalances map[string]int64, quoteBalance int64) {
	m.baseBalances = baseBalances
	m.quoteBalance = quoteBalance
}

// Start starts the actor
func (m *MultiSymbolLP) Start(ctx context.Context) error {
	m.monitorTicker = time.NewTicker(m.config.MonitorInterval)

	go m.eventLoop(ctx)
	go m.monitorLoop(ctx)

	if err := m.BaseActor.Start(ctx); err != nil {
		return err
	}

	// Subscribe to all symbols
	for _, symbol := range m.config.Symbols {
		m.Subscribe(symbol)
	}
	m.QueryBalance()

	return nil
}

// Stop stops the actor
func (m *MultiSymbolLP) Stop() error {
	if m.monitorTicker != nil {
		m.monitorTicker.Stop()
	}
	close(m.stopCh)
	return m.BaseActor.Stop()
}

func (m *MultiSymbolLP) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case event := <-m.EventChannel():
			m.OnEvent(event)
		}
	}
}

func (m *MultiSymbolLP) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-m.monitorTicker.C:
			m.checkAllExitConditions()
		}
	}
}

// OnEvent handles incoming events
func (m *MultiSymbolLP) OnEvent(event *Event) {
	switch event.Type {
	case EventBookSnapshot:
		m.onBookSnapshot(event.Data.(BookSnapshotEvent))
	case EventBookDelta:
		m.onBookDelta(event.Data.(BookDeltaEvent))
	case EventOrderAccepted:
		m.onOrderAccepted(event.Data.(OrderAcceptedEvent))
	case EventOrderFilled, EventOrderPartialFill:
		m.onOrderFilled(event.Data.(OrderFillEvent))
	case EventOrderCancelled:
		m.onOrderCancelled(event.Data.(OrderCancelledEvent))
	case EventOrderRejected:
		m.onOrderRejected(event.Data.(OrderRejectedEvent))
	}
}

func (m *MultiSymbolLP) onBookSnapshot(snap BookSnapshotEvent) {
	state := m.symbolStates[snap.Symbol]
	if state == nil {
		return
	}

	// Update market state
	if len(snap.Snapshot.Bids) > 0 {
		state.BestBid = snap.Snapshot.Bids[0].Price
		state.BidLiquidity = snap.Snapshot.Bids[0].VisibleQty
	}
	if len(snap.Snapshot.Asks) > 0 {
		state.BestAsk = snap.Snapshot.Asks[0].Price
		state.AskLiquidity = snap.Snapshot.Asks[0].VisibleQty
	}

	if state.BestBid > 0 && state.BestAsk > 0 {
		state.LastMidPrice = (state.BestBid + state.BestAsk) / 2
	} else if state.LastMidPrice == 0 && state.BootstrapPrice > 0 {
		state.LastMidPrice = state.BootstrapPrice
	}

	// Place quotes if we have balances and no active orders
	canPlace := state.LastMidPrice > 0 && state.ActiveBidID == 0 && state.ActiveAskID == 0
	if canPlace {
		m.placeQuotesForSymbol(state)
	}
}

func (m *MultiSymbolLP) onBookDelta(delta BookDeltaEvent) {
	state := m.symbolStates[delta.Symbol]
	if state == nil {
		return
	}

	if delta.Delta.Side == exchange.Buy {
		if delta.Delta.Price >= state.BestBid {
			state.BestBid = delta.Delta.Price
			state.BidLiquidity = max(delta.Delta.VisibleQty, 0)
		}
	} else {
		if state.BestAsk == 0 || delta.Delta.Price <= state.BestAsk {
			state.BestAsk = delta.Delta.Price
			state.AskLiquidity = max(delta.Delta.VisibleQty, 0)
		}
	}
}

func (m *MultiSymbolLP) onOrderAccepted(accepted OrderAcceptedEvent) {
	for _, state := range m.symbolStates {
		if accepted.RequestID == state.lastBidReqID {
			state.ActiveBidID = accepted.OrderID
			state.lastBidReqID = 0
			return
		} else if accepted.RequestID == state.lastAskReqID {
			state.ActiveAskID = accepted.OrderID
			state.lastAskReqID = 0
			return
		}
	}
}

func (m *MultiSymbolLP) onOrderFilled(fill OrderFillEvent) {
	// Find which symbol this order belongs to
	var state *symbolLPState
	for _, s := range m.symbolStates {
		if fill.OrderID == s.ActiveBidID || fill.OrderID == s.ActiveAskID {
			state = s
			break
		}
	}
	if state == nil || state.Instrument == nil {
		return
	}

	basePrecision := state.Instrument.BasePrecision()

	// Update shared balances
	if fill.Side == exchange.Buy {
		m.updatePosition(state, fill.Qty, fill.Price)
		m.baseBalances[state.BaseAsset] += fill.Qty
		notional := (fill.Qty * fill.Price) / basePrecision
		m.quoteBalance -= (notional + fill.FeeAmount)
	} else {
		m.updatePosition(state, -fill.Qty, fill.Price)
		m.baseBalances[state.BaseAsset] -= fill.Qty
		notional := (fill.Qty * fill.Price) / basePrecision
		m.quoteBalance += (notional - fill.FeeAmount)
	}

	// Clear active order IDs on full fill
	if fill.IsFull {
		if fill.OrderID == state.ActiveBidID {
			state.ActiveBidID = 0
		} else if fill.OrderID == state.ActiveAskID {
			state.ActiveAskID = 0
		}
	}

	// Cancel and requote
	m.cancelOrdersForSymbol(state)
	state.ActiveBidID = 0
	state.ActiveAskID = 0
	m.placeQuotesForSymbol(state)
}

func (m *MultiSymbolLP) onOrderCancelled(cancelled OrderCancelledEvent) {
	for _, state := range m.symbolStates {
		if cancelled.OrderID == state.ActiveBidID {
			state.ActiveBidID = 0
			return
		}
		if cancelled.OrderID == state.ActiveAskID {
			state.ActiveAskID = 0
			return
		}
	}
}

func (m *MultiSymbolLP) onOrderRejected(rejected OrderRejectedEvent) {
	for _, state := range m.symbolStates {
		if rejected.RequestID == state.lastBidReqID {
			state.lastBidReqID = 0
			return
		} else if rejected.RequestID == state.lastAskReqID {
			state.lastAskReqID = 0
			return
		}
	}
}

func (m *MultiSymbolLP) nextRequestID() uint64 {
	return atomic.LoadUint64(&m.BaseActor.requestSeq) + 1
}

func (m *MultiSymbolLP) placeQuotesForSymbol(state *symbolLPState) {
	if state.LastMidPrice == 0 || state.Instrument == nil {
		return
	}

	basePrecision := state.Instrument.BasePrecision()
	tickSize := state.Instrument.TickSize()
	if tickSize == 0 {
		return
	}

	halfSpread := (state.LastMidPrice * m.config.SpreadBps) / (2 * 10000)
	midPrice := state.LastMidPrice

	// Apply inventory skew if configured
	if m.config.SkewFactor > 0 && state.NetPosition != 0 {
		// Example: NetPosition = 100, SkewFactor = 0.0001 (1 bps per unit)
		// Skew = 100 * 0.0001 = 0.01 (1%)
		// If Long (pos > 0), we want to lower price to sell. Skew should be negative.
		// If Short (pos < 0), we want to raise price to buy. Skew should be positive.
		skew := -float64(state.NetPosition) * m.config.SkewFactor
		midPrice = int64(float64(midPrice) * (1.0 + skew))
	}

	bidPrice := midPrice - halfSpread
	askPrice := midPrice + halfSpread


	// Align prices to tick size
	bidPrice = (bidPrice / tickSize) * tickSize
	askPrice = (askPrice / tickSize) * tickSize

	// Safety check - prices must be positive
	if bidPrice <= 0 || askPrice <= 0 {
		return
	}

	// Get available balance for this symbol
	baseBalance := m.baseBalances[state.BaseAsset]

	// Place sell order if we have base asset
	if baseBalance > 0 && askPrice > 0 {
		state.lastAskReqID = m.nextRequestID()
		m.SubmitOrder(state.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, baseBalance)
		state.AskSize = baseBalance
	}

	// Place buy order if we have quote (allocate portion of quote balance)
	numSymbols := int64(len(m.config.Symbols))
	if numSymbols == 0 {
		return
	}
	quotePerSymbol := m.quoteBalance / numSymbols
	if quotePerSymbol > 0 && bidPrice > 0 {
		bidQty := (quotePerSymbol * basePrecision) / bidPrice
		if bidQty > 0 {
			state.lastBidReqID = m.nextRequestID()
			m.SubmitOrder(state.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, bidQty)
			state.BidSize = bidQty
		}
	}
}

func (m *MultiSymbolLP) cancelOrdersForSymbol(state *symbolLPState) {
	if state.ActiveBidID != 0 {
		m.CancelOrder(state.ActiveBidID)
	}
	if state.ActiveAskID != 0 {
		m.CancelOrder(state.ActiveAskID)
	}
}

func (m *MultiSymbolLP) updatePosition(state *symbolLPState, deltaQty int64, price int64) {
	if state.NetPosition == 0 {
		state.NetPosition = deltaQty
		state.AvgEntryPrice = price
		return
	}

	if (state.NetPosition > 0 && deltaQty > 0) || (state.NetPosition < 0 && deltaQty < 0) {
		totalNotional := (state.NetPosition * state.AvgEntryPrice) + (deltaQty * price)
		state.NetPosition += deltaQty
		if state.NetPosition != 0 {
			state.AvgEntryPrice = totalNotional / state.NetPosition
		}
	} else {
		state.NetPosition += deltaQty
		if state.NetPosition == 0 {
			state.AvgEntryPrice = 0
		} else if (state.NetPosition > 0 && deltaQty < 0) || (state.NetPosition < 0 && deltaQty > 0) {
			state.AvgEntryPrice = price
		}
	}
}

func (m *MultiSymbolLP) checkAllExitConditions() {
	for _, state := range m.symbolStates {
		m.checkExitConditionForSymbol(state)
	}
}

func (m *MultiSymbolLP) checkExitConditionForSymbol(state *symbolLPState) {
	absExposure := state.NetPosition
	if absExposure < 0 {
		absExposure = -absExposure
	}

	if absExposure < m.config.MinExitSize {
		return
	}

	shouldExit := DefaultExitStrategy(
		state.NetPosition,
		state.BestBid,
		state.BestAsk,
		state.BidLiquidity,
		state.AskLiquidity,
		m.config.LiquidityMultiple,
	)

	if shouldExit {
		m.executeExitForSymbol(state)
	}
}

func (m *MultiSymbolLP) executeExitForSymbol(state *symbolLPState) {
	if state.NetPosition == 0 {
		return
	}

	m.cancelOrdersForSymbol(state)

	exitSize := state.NetPosition
	if exitSize < 0 {
		exitSize = -exitSize
	}

	if state.NetPosition > 0 {
		m.SubmitOrder(state.Symbol, exchange.Sell, exchange.Market, 0, exitSize)
	} else {
		m.SubmitOrder(state.Symbol, exchange.Buy, exchange.Market, 0, exitSize)
	}
}

// GetSymbolState returns the state for a symbol (for testing)
func (m *MultiSymbolLP) GetSymbolState(symbol string) *symbolLPState {
	return m.symbolStates[symbol]
}

// GetBalances returns current balances (for testing)
func (m *MultiSymbolLP) GetBalances() (map[string]int64, int64) {
	return m.baseBalances, m.quoteBalance
}

// symbolMMState tracks per-symbol state for MultiSymbolMM
type symbolMMState struct {
	Symbol        string
	Instrument    exchange.Instrument
	ActiveBidID   uint64
	ActiveAskID   uint64
	lastBidReqID  uint64
	lastAskReqID  uint64
	CurrentBid    int64
	CurrentAsk    int64
	LastMidPrice  int64
	Inventory     int64
}

// MultiSymbolMMConfig configures a multi-symbol market maker
type MultiSymbolMMConfig struct {
	Symbols          []string                       // All symbols to trade
	Instruments      map[string]exchange.Instrument // Symbol -> Instrument
	SpreadBps        int64                          // Fixed spread in basis points
	QuoteSize        int64                          // Order size per side per symbol
	MaxInventory     int64                          // Maximum absolute position per symbol
	RequoteThreshold int64                          // Minimum bps change to requote
	MonitorInterval  time.Duration                  // How often to check conditions
}

// MultiSymbolMM manages market making for multiple symbols
type MultiSymbolMM struct {
	*BaseActor
	config       MultiSymbolMMConfig
	symbolStates map[string]*symbolMMState

	monitorTicker *time.Ticker
	stopCh        chan struct{}
}

// NewMultiSymbolMM creates a new multi-symbol market maker
func NewMultiSymbolMM(id uint64, gateway *exchange.ClientGateway, config MultiSymbolMMConfig) *MultiSymbolMM {
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 50 * time.Millisecond
	}
	if config.RequoteThreshold == 0 {
		config.RequoteThreshold = 5
	}

	mm := &MultiSymbolMM{
		BaseActor:    NewBaseActor(id, gateway),
		config:       config,
		symbolStates: make(map[string]*symbolMMState),
		stopCh:       make(chan struct{}),
	}

	for _, symbol := range config.Symbols {
		inst := config.Instruments[symbol]
		if inst == nil {
			continue
		}
		mm.symbolStates[symbol] = &symbolMMState{
			Symbol:     symbol,
			Instrument: inst,
		}
	}

	return mm
}

// Start starts the actor
func (mm *MultiSymbolMM) Start(ctx context.Context) error {
	mm.monitorTicker = time.NewTicker(mm.config.MonitorInterval)

	go mm.eventLoop(ctx)
	go mm.monitorLoop(ctx)

	if err := mm.BaseActor.Start(ctx); err != nil {
		return err
	}

	for _, symbol := range mm.config.Symbols {
		mm.Subscribe(symbol)
	}
	mm.QueryBalance()

	return nil
}

// Stop stops the actor
func (mm *MultiSymbolMM) Stop() error {
	if mm.monitorTicker != nil {
		mm.monitorTicker.Stop()
	}
	close(mm.stopCh)
	return mm.BaseActor.Stop()
}

func (mm *MultiSymbolMM) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-mm.stopCh:
			return
		case event := <-mm.EventChannel():
			mm.OnEvent(event)
		}
	}
}

func (mm *MultiSymbolMM) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-mm.stopCh:
			return
		case <-mm.monitorTicker.C:
			// Periodic health check
		}
	}
}

// OnEvent handles incoming events
func (mm *MultiSymbolMM) OnEvent(event *Event) {
	switch event.Type {
	case EventBookSnapshot:
		mm.onBookSnapshot(event.Data.(BookSnapshotEvent))
	case EventOrderAccepted:
		mm.onOrderAccepted(event.Data.(OrderAcceptedEvent))
	case EventOrderFilled, EventOrderPartialFill:
		mm.onOrderFilled(event.Data.(OrderFillEvent))
	case EventOrderCancelled:
		mm.onOrderCancelled(event.Data.(OrderCancelledEvent))
	}
}

func (mm *MultiSymbolMM) onBookSnapshot(snap BookSnapshotEvent) {
	state := mm.symbolStates[snap.Symbol]
	if state == nil || state.Instrument == nil {
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	bestBid := snap.Snapshot.Bids[0].Price
	bestAsk := snap.Snapshot.Asks[0].Price
	midPrice := (bestBid + bestAsk) / 2

	if state.LastMidPrice == 0 {
		state.LastMidPrice = midPrice
		mm.requoteSymbol(state, midPrice)
		return
	}

	bpsChange := BPSChange(state.LastMidPrice, midPrice)
	if bpsChange < 0 {
		bpsChange = -bpsChange
	}

	if bpsChange >= mm.config.RequoteThreshold {
		state.LastMidPrice = midPrice
		mm.requoteSymbol(state, midPrice)
	}
}

func (mm *MultiSymbolMM) onOrderAccepted(accepted OrderAcceptedEvent) {
	for _, state := range mm.symbolStates {
		if accepted.RequestID == state.lastBidReqID {
			state.ActiveBidID = accepted.OrderID
			return
		} else if accepted.RequestID == state.lastAskReqID {
			state.ActiveAskID = accepted.OrderID
			return
		}
	}
}

func (mm *MultiSymbolMM) onOrderFilled(fill OrderFillEvent) {
	var state *symbolMMState
	for _, s := range mm.symbolStates {
		if fill.OrderID == s.ActiveBidID || fill.OrderID == s.ActiveAskID {
			state = s
			break
		}
	}
	if state == nil {
		return
	}

	if fill.Side == exchange.Buy {
		state.Inventory += int64(fill.Qty)
	} else {
		state.Inventory -= int64(fill.Qty)
	}

	if fill.IsFull {
		if fill.OrderID == state.ActiveBidID {
			state.ActiveBidID = 0
		} else if fill.OrderID == state.ActiveAskID {
			state.ActiveAskID = 0
		}
	}
}

func (mm *MultiSymbolMM) onOrderCancelled(cancelled OrderCancelledEvent) {
	for _, state := range mm.symbolStates {
		if cancelled.OrderID == state.ActiveBidID {
			state.ActiveBidID = 0
			return
		}
		if cancelled.OrderID == state.ActiveAskID {
			state.ActiveAskID = 0
			return
		}
	}
}

func (mm *MultiSymbolMM) requoteSymbol(state *symbolMMState, midPrice int64) {
	if state.Instrument == nil {
		return
	}

	absInventory := state.Inventory
	if absInventory < 0 {
		absInventory = -absInventory
	}
	if absInventory >= mm.config.MaxInventory {
		mm.cancelOrdersForSymbol(state)
		return
	}

	tickSize := state.Instrument.TickSize()

	spreadHalf := (midPrice * mm.config.SpreadBps) / (2 * 10000)
	bidPrice := midPrice - spreadHalf
	askPrice := midPrice + spreadHalf

	bidPrice = (bidPrice / tickSize) * tickSize
	askPrice = (askPrice / tickSize) * tickSize

	if bidPrice == state.CurrentBid && askPrice == state.CurrentAsk {
		return
	}

	mm.cancelOrdersForSymbol(state)

	state.CurrentBid = bidPrice
	state.CurrentAsk = askPrice

	state.lastBidReqID = atomic.AddUint64(&mm.BaseActor.requestSeq, 1)
	mm.SubmitOrder(state.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, mm.config.QuoteSize)

	state.lastAskReqID = atomic.AddUint64(&mm.BaseActor.requestSeq, 1)
	mm.SubmitOrder(state.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, mm.config.QuoteSize)
}

func (mm *MultiSymbolMM) cancelOrdersForSymbol(state *symbolMMState) {
	if state.ActiveBidID != 0 {
		mm.CancelOrder(state.ActiveBidID)
		state.ActiveBidID = 0
	}
	if state.ActiveAskID != 0 {
		mm.CancelOrder(state.ActiveAskID)
		state.ActiveAskID = 0
	}
}

// GetSymbolState returns the state for a symbol (for testing)
func (mm *MultiSymbolMM) GetSymbolState(symbol string) *symbolMMState {
	return mm.symbolStates[symbol]
}

// BPSChange calculates basis points change between two prices
func BPSChange(oldPrice, newPrice int64) int64 {
	if oldPrice == 0 {
		return 0
	}
	return ((newPrice - oldPrice) * 10000) / oldPrice
}
