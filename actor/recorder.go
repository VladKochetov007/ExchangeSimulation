package actor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"exchange_sim/exchange"
)

type RotationStrategy uint8

const (
	RotationNone RotationStrategy = iota
	RotationDaily
	RotationHourly
)

type RecorderConfig struct {
	OutputDir            string
	Symbols              []string
	FlushInterval        time.Duration
	SnapshotInterval     time.Duration
	SnapshotDeltaCount   uint64
	RotationStrategy     RotationStrategy
	RecordTrades         bool
	RecordOrderbook      bool
	RecordOpenInterest   bool
	RecordFunding        bool
	SeparateHiddenFiles  bool
}

type instrumentRecorder struct {
	symbol              string
	instrumentType      string
	orderbookFile       *os.File
	orderbookBuf        *bufio.Writer
	orderbookHiddenFile *os.File
	orderbookHiddenBuf  *bufio.Writer
	tradesFile          *os.File
	tradesBuf           *bufio.Writer
	oiFile              *os.File
	oiBuf               *bufio.Writer
	fundingFile         *os.File
	fundingBuf          *bufio.Writer
	deltasSinceSnapshot uint64
	lastSnapshotTime    int64
}

type recordEvent struct {
	symbol    string
	eventType EventType
	data      any
}

type RecorderActor struct {
	*BaseActor
	config      RecorderConfig
	recorders   map[string]*instrumentRecorder
	writeCh     chan recordEvent
	instruments map[string]exchange.Instrument
	writeWg     sync.WaitGroup
}

func NewRecorder(id uint64, gateway *exchange.ClientGateway, config RecorderConfig, instruments map[string]exchange.Instrument) (*RecorderActor, error) {
	if config.FlushInterval == 0 {
		config.FlushInterval = time.Second
	}
	if config.SnapshotInterval == 0 {
		config.SnapshotInterval = 30 * time.Second
	}
	if config.SnapshotDeltaCount == 0 {
		config.SnapshotDeltaCount = 100
	}
	if config.OutputDir == "" {
		config.OutputDir = "output"
	}

	os.MkdirAll(config.OutputDir, 0755)

	r := &RecorderActor{
		BaseActor:   NewBaseActor(id, gateway),
		config:      config,
		recorders:   make(map[string]*instrumentRecorder),
		writeCh:     make(chan recordEvent, 10000),
		instruments: instruments,
	}

	for _, symbol := range config.Symbols {
		instrument := instruments[symbol]
		if instrument == nil {
			r.cleanup()
			return nil, fmt.Errorf("unknown instrument: %s", symbol)
		}

		rec, err := r.createInstrumentRecorder(symbol, instrument)
		if err != nil {
			r.cleanup()
			return nil, err
		}
		r.recorders[symbol] = rec
	}

	return r, nil
}

func (r *RecorderActor) createInstrumentRecorder(symbol string, instrument exchange.Instrument) (*instrumentRecorder, error) {
	instType := instrument.InstrumentType()

	rec := &instrumentRecorder{
		symbol:         symbol,
		instrumentType: instType,
	}

	if r.config.RecordOrderbook {
		obPath := r.getFilePath(symbol, instType, "orderbook")
		obFile, err := os.Create(obPath)
		if err != nil {
			return nil, err
		}
		rec.orderbookFile = obFile
		rec.orderbookBuf = bufio.NewWriter(obFile)

		if r.config.SeparateHiddenFiles {
			rec.orderbookBuf.WriteString("timestamp,seq,type,side,price,qty\n")

			hiddenPath := r.getFilePath(symbol, instType, "orderbook_hidden")
			hiddenFile, err := os.Create(hiddenPath)
			if err != nil {
				return nil, err
			}
			rec.orderbookHiddenFile = hiddenFile
			rec.orderbookHiddenBuf = bufio.NewWriter(hiddenFile)
			rec.orderbookHiddenBuf.WriteString("timestamp,seq,type,side,price,qty\n")
		} else {
			rec.orderbookBuf.WriteString("timestamp,seq,type,side,price,visible_qty,hidden_qty\n")
		}
	}

	if r.config.RecordTrades {
		tradesPath := r.getFilePath(symbol, instType, "trades")
		tradesFile, err := os.Create(tradesPath)
		if err != nil {
			return nil, err
		}
		rec.tradesFile = tradesFile
		rec.tradesBuf = bufio.NewWriter(tradesFile)
		rec.tradesBuf.WriteString("timestamp,seq,trade_id,side,price,qty\n")
	}

	if r.config.RecordOpenInterest && instrument.IsPerp() {
		oiPath := r.getFilePath(symbol, instType, "openinterest")
		oiFile, err := os.Create(oiPath)
		if err != nil {
			return nil, err
		}
		rec.oiFile = oiFile
		rec.oiBuf = bufio.NewWriter(oiFile)
		rec.oiBuf.WriteString("timestamp,seq,total_contracts\n")
	}

	if r.config.RecordFunding && instrument.IsPerp() {
		fundingPath := r.getFilePath(symbol, instType, "funding")
		fundingFile, err := os.Create(fundingPath)
		if err != nil {
			return nil, err
		}
		rec.fundingFile = fundingFile
		rec.fundingBuf = bufio.NewWriter(fundingFile)
		rec.fundingBuf.WriteString("timestamp,seq,rate,mark_price,index_price,next_funding_time,interval_seconds\n")
	}

	return rec, nil
}

func (r *RecorderActor) getFilePath(symbol, instType, dataType string) string {
	filename := fmt.Sprintf("%s_%s_%s.csv", symbol, instType, dataType)

	if r.config.RotationStrategy != RotationNone {
		now := time.Now()
		var timeSuffix string
		switch r.config.RotationStrategy {
		case RotationDaily:
			timeSuffix = now.Format("20060102")
		case RotationHourly:
			timeSuffix = now.Format("2006010215")
		}
		filename = fmt.Sprintf("%s_%s_%s_%s.csv", symbol, instType, dataType, timeSuffix)
	}

	return filepath.Join(r.config.OutputDir, filename)
}

func (r *RecorderActor) Start(ctx context.Context) error {
	for _, symbol := range r.config.Symbols {
		r.Subscribe(symbol)
	}
	go r.eventLoop(ctx)
	r.writeWg.Add(1)
	go r.writeLoop(ctx)
	return r.BaseActor.Start(ctx)
}

func (r *RecorderActor) Stop() error {
	close(r.writeCh)
	r.writeWg.Wait()
	r.cleanup()
	return r.BaseActor.Stop()
}

func (r *RecorderActor) cleanup() {
	for _, rec := range r.recorders {
		if rec.orderbookFile != nil {
			rec.orderbookBuf.Flush()
			rec.orderbookFile.Close()
		}
		if rec.orderbookHiddenFile != nil {
			rec.orderbookHiddenBuf.Flush()
			rec.orderbookHiddenFile.Close()
		}
		if rec.tradesFile != nil {
			rec.tradesBuf.Flush()
			rec.tradesFile.Close()
		}
		if rec.oiFile != nil {
			rec.oiBuf.Flush()
			rec.oiFile.Close()
		}
		if rec.fundingFile != nil {
			rec.fundingBuf.Flush()
			rec.fundingFile.Close()
		}
	}
}

func (r *RecorderActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-r.EventChannel():
			r.OnEvent(event)
		}
	}
}

func (r *RecorderActor) OnEvent(event *Event) {
	switch event.Type {
	case EventTrade:
		r.onTrade(event.Data.(TradeEvent))
	case EventBookSnapshot:
		r.onSnapshot(event.Data.(BookSnapshotEvent))
	case EventBookDelta:
		r.onDelta(event.Data.(BookDeltaEvent))
	case EventOpenInterest:
		r.onOpenInterest(event.Data.(OpenInterestEvent))
	case EventFundingUpdate:
		r.onFunding(event.Data.(FundingUpdateEvent))
	}
}

func (r *RecorderActor) onSnapshot(snap BookSnapshotEvent) {
	rec := r.recorders[snap.Symbol]
	if rec == nil || !r.config.RecordOrderbook {
		return
	}

	rec.deltasSinceSnapshot = 0
	rec.lastSnapshotTime = snap.Timestamp

	r.writeCh <- recordEvent{
		symbol:    snap.Symbol,
		eventType: EventBookSnapshot,
		data:      snap,
	}
}

func (r *RecorderActor) onDelta(delta BookDeltaEvent) {
	rec := r.recorders[delta.Symbol]
	if rec == nil || !r.config.RecordOrderbook {
		return
	}

	rec.deltasSinceSnapshot++

	r.writeCh <- recordEvent{
		symbol:    delta.Symbol,
		eventType: EventBookDelta,
		data:      delta,
	}

	needSnapshot := false
	if r.config.SnapshotDeltaCount > 0 && rec.deltasSinceSnapshot >= r.config.SnapshotDeltaCount {
		needSnapshot = true
	}
	if r.config.SnapshotInterval > 0 {
		elapsed := delta.Timestamp - rec.lastSnapshotTime
		if elapsed >= r.config.SnapshotInterval.Nanoseconds() {
			needSnapshot = true
		}
	}

	if needSnapshot {
		r.Subscribe(delta.Symbol)
	}
}

func (r *RecorderActor) onTrade(trade TradeEvent) {
	if !r.config.RecordTrades {
		return
	}

	r.writeCh <- recordEvent{
		symbol:    trade.Symbol,
		eventType: EventTrade,
		data:      trade,
	}
}

func (r *RecorderActor) onOpenInterest(oi OpenInterestEvent) {
	if !r.config.RecordOpenInterest {
		return
	}

	r.writeCh <- recordEvent{
		symbol:    oi.Symbol,
		eventType: EventOpenInterest,
		data:      oi,
	}
}

func (r *RecorderActor) onFunding(funding FundingUpdateEvent) {
	if !r.config.RecordFunding {
		return
	}

	r.writeCh <- recordEvent{
		symbol:    funding.Symbol,
		eventType: EventFundingUpdate,
		data:      funding,
	}
}

func (r *RecorderActor) writeLoop(ctx context.Context) {
	defer r.writeWg.Done()
	ticker := time.NewTicker(r.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.drainWriteBuffer()
			return
		case event, ok := <-r.writeCh:
			if !ok {
				r.drainWriteBuffer()
				return
			}
			r.writeRecordEvent(event)
		case <-ticker.C:
			r.flushAll()
		}
	}
}

func (r *RecorderActor) drainWriteBuffer() {
	for {
		select {
		case event, ok := <-r.writeCh:
			if !ok {
				r.flushAll()
				return
			}
			r.writeRecordEvent(event)
		default:
			r.flushAll()
			return
		}
	}
}

func (r *RecorderActor) writeRecordEvent(event recordEvent) {
	rec := r.recorders[event.symbol]
	if rec == nil {
		return
	}

	switch event.eventType {
	case EventBookSnapshot:
		r.writeSnapshot(rec, event.data.(BookSnapshotEvent))
	case EventBookDelta:
		r.writeDelta(rec, event.data.(BookDeltaEvent))
	case EventTrade:
		r.writeTrade(rec, event.data.(TradeEvent))
	case EventOpenInterest:
		r.writeOpenInterest(rec, event.data.(OpenInterestEvent))
	case EventFundingUpdate:
		r.writeFunding(rec, event.data.(FundingUpdateEvent))
	}
}

func (r *RecorderActor) writeSnapshot(rec *instrumentRecorder, snap BookSnapshotEvent) {
	if r.config.SeparateHiddenFiles {
		for _, level := range snap.Snapshot.Bids {
			if level.VisibleQty > 0 {
				fmt.Fprintf(rec.orderbookBuf, "%d,%d,snapshot,bid,%d,%d\n",
					snap.Timestamp, snap.SeqNum, level.Price, level.VisibleQty)
			}
		}
		for _, level := range snap.Snapshot.Asks {
			if level.VisibleQty > 0 {
				fmt.Fprintf(rec.orderbookBuf, "%d,%d,snapshot,ask,%d,%d\n",
					snap.Timestamp, snap.SeqNum, level.Price, level.VisibleQty)
			}
		}

		for _, level := range snap.Snapshot.Bids {
			if level.HiddenQty > 0 {
				fmt.Fprintf(rec.orderbookHiddenBuf, "%d,%d,snapshot,bid,%d,%d\n",
					snap.Timestamp, snap.SeqNum, level.Price, level.HiddenQty)
			}
		}
		for _, level := range snap.Snapshot.Asks {
			if level.HiddenQty > 0 {
				fmt.Fprintf(rec.orderbookHiddenBuf, "%d,%d,snapshot,ask,%d,%d\n",
					snap.Timestamp, snap.SeqNum, level.Price, level.HiddenQty)
			}
		}
	} else {
		for _, level := range snap.Snapshot.Bids {
			fmt.Fprintf(rec.orderbookBuf, "%d,%d,snapshot,bid,%d,%d,%d\n",
				snap.Timestamp, snap.SeqNum, level.Price, level.VisibleQty, level.HiddenQty)
		}
		for _, level := range snap.Snapshot.Asks {
			fmt.Fprintf(rec.orderbookBuf, "%d,%d,snapshot,ask,%d,%d,%d\n",
				snap.Timestamp, snap.SeqNum, level.Price, level.VisibleQty, level.HiddenQty)
		}
	}
}

func (r *RecorderActor) writeDelta(rec *instrumentRecorder, delta BookDeltaEvent) {
	side := "bid"
	if delta.Delta.Side == exchange.Sell {
		side = "ask"
	}

	if r.config.SeparateHiddenFiles {
		fmt.Fprintf(rec.orderbookBuf, "%d,%d,delta,%s,%d,%d\n",
			delta.Timestamp, delta.SeqNum, side, delta.Delta.Price, delta.Delta.VisibleQty)

		fmt.Fprintf(rec.orderbookHiddenBuf, "%d,%d,delta,%s,%d,%d\n",
			delta.Timestamp, delta.SeqNum, side, delta.Delta.Price, delta.Delta.HiddenQty)
	} else {
		fmt.Fprintf(rec.orderbookBuf, "%d,%d,delta,%s,%d,%d,%d\n",
			delta.Timestamp, delta.SeqNum, side, delta.Delta.Price,
			delta.Delta.VisibleQty, delta.Delta.HiddenQty)
	}
}

func (r *RecorderActor) writeTrade(rec *instrumentRecorder, trade TradeEvent) {
	side := "Buy"
	if trade.Trade.Side == exchange.Sell {
		side = "Sell"
	}

	fmt.Fprintf(rec.tradesBuf, "%d,%d,%d,%s,%d,%d\n",
		trade.Timestamp, trade.Trade.TradeID, trade.Trade.TradeID,
		side, trade.Trade.Price, trade.Trade.Qty)
}

func (r *RecorderActor) writeOpenInterest(rec *instrumentRecorder, oi OpenInterestEvent) {
	fmt.Fprintf(rec.oiBuf, "%d,0,%d\n",
		oi.Timestamp, oi.OpenInterest.TotalContracts)
}

func (r *RecorderActor) writeFunding(rec *instrumentRecorder, funding FundingUpdateEvent) {
	fmt.Fprintf(rec.fundingBuf, "%d,0,%d,%d,%d,%d,%d\n",
		funding.Timestamp,
		funding.FundingRate.Rate,
		funding.FundingRate.MarkPrice,
		funding.FundingRate.IndexPrice,
		funding.FundingRate.NextFunding,
		funding.FundingRate.Interval)
}

func (r *RecorderActor) flushAll() {
	for _, rec := range r.recorders {
		if rec.orderbookBuf != nil {
			rec.orderbookBuf.Flush()
		}
		if rec.orderbookHiddenBuf != nil {
			rec.orderbookHiddenBuf.Flush()
		}
		if rec.tradesBuf != nil {
			rec.tradesBuf.Flush()
		}
		if rec.oiBuf != nil {
			rec.oiBuf.Flush()
		}
		if rec.fundingBuf != nil {
			rec.fundingBuf.Flush()
		}
	}
}
