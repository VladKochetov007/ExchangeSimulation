package actor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"exchange_sim/exchange"
)

type RecorderConfig struct {
	Symbols       []string
	TradesPath    string
	ObservedPath   string
	HiddenPath     string
	FlushInterval  time.Duration
	SnapshotInterval time.Duration
}

type tradeRecord struct {
	timestamp int64
	symbol    string
	tradeID   uint64
	side      exchange.Side
	price     int64
	qty       int64
}

type snapshotRecord struct {
	timestamp int64
	symbol    string
	snapshot  *exchange.BookSnapshot
	seqNum    uint64
}

type deltaRecord struct {
	timestamp int64
	symbol    string
	delta     *exchange.BookDelta
	seqNum    uint64
}

type RecorderActor struct {
	*BaseActor
	config      RecorderConfig
	writeCh     chan any
	tradesFile  *os.File
	obsFile     *os.File
	hiddenFile  *os.File
	tradesBuf   *bufio.Writer
	obsBuf      *bufio.Writer
	hiddenBuf   *bufio.Writer
}

func NewRecorder(id uint64, gateway *exchange.ClientGateway, config RecorderConfig) (*RecorderActor, error) {
	if config.FlushInterval == 0 {
		config.FlushInterval = time.Second
	}

	tradesFile, err := os.Create(config.TradesPath)
	if err != nil {
		return nil, err
	}

	obsFile, err := os.Create(config.ObservedPath)
	if err != nil {
		tradesFile.Close()
		return nil, err
	}

	hiddenFile, err := os.Create(config.HiddenPath)
	if err != nil {
		tradesFile.Close()
		obsFile.Close()
		return nil, err
	}

	r := &RecorderActor{
		BaseActor:   NewBaseActor(id, gateway),
		config:      config,
		writeCh:     make(chan any, 10000),
		tradesFile:  tradesFile,
		obsFile:     obsFile,
		hiddenFile:  hiddenFile,
		tradesBuf:   bufio.NewWriter(tradesFile),
		obsBuf:      bufio.NewWriter(obsFile),
		hiddenBuf:   bufio.NewWriter(hiddenFile),
	}

	r.tradesBuf.WriteString("timestamp,symbol,trade_id,side,price,qty\n")
	r.obsBuf.WriteString("timestamp,seq,symbol,type,side,price,qty\n")
	r.hiddenBuf.WriteString("timestamp,seq,symbol,type,side,price,qty\n")

	return r, nil
}

func (r *RecorderActor) Start(ctx context.Context) error {
	for _, symbol := range r.config.Symbols {
		r.Subscribe(symbol)
	}
	go r.eventLoop(ctx)
	go r.writeLoop(ctx)
	go r.snapshotLoop(ctx)
	return r.BaseActor.Start(ctx)
}

func (r *RecorderActor) Stop() error {
	close(r.writeCh)
	r.tradesBuf.Flush()
	r.obsBuf.Flush()
	r.hiddenBuf.Flush()
	r.tradesFile.Close()
	r.obsFile.Close()
	r.hiddenFile.Close()
	return r.BaseActor.Stop()
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
	}
}

func (r *RecorderActor) onTrade(trade TradeEvent) {
	rec := tradeRecord{
		timestamp: trade.Timestamp,
		symbol:    trade.Symbol,
		tradeID:   trade.Trade.TradeID,
		side:      trade.Trade.Side,
		price:     trade.Trade.Price,
		qty:       trade.Trade.Qty,
	}
	r.writeCh <- rec
}

func (r *RecorderActor) onSnapshot(snapshot BookSnapshotEvent) {
	rec := snapshotRecord{
		timestamp: snapshot.Timestamp,
		symbol:    snapshot.Symbol,
		snapshot:  snapshot.Snapshot,
		seqNum:    snapshot.SeqNum,
	}
	r.writeCh <- rec
}

func (r *RecorderActor) onDelta(delta BookDeltaEvent) {
	rec := deltaRecord{
		timestamp: delta.Timestamp,
		symbol:    delta.Symbol,
		delta:     delta.Delta,
		seqNum:    delta.SeqNum,
	}
	r.writeCh <- rec
}

func (r *RecorderActor) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.drainWriteBuffer()
			return
		case rec, ok := <-r.writeCh:
			if !ok {
				return
			}
			r.writeRecord(rec)
		case <-ticker.C:
			r.tradesBuf.Flush()
			r.obsBuf.Flush()
			r.hiddenBuf.Flush()
		}
	}
}

func (r *RecorderActor) drainWriteBuffer() {
	for {
		select {
		case rec := <-r.writeCh:
			r.writeRecord(rec)
		default:
			r.tradesBuf.Flush()
			r.obsBuf.Flush()
			r.hiddenBuf.Flush()
			return
		}
	}
}

func (r *RecorderActor) writeRecord(rec any) {
	switch v := rec.(type) {
	case tradeRecord:
		side := "buy"
		if v.side == exchange.Sell {
			side = "sell"
		}
		fmt.Fprintf(r.tradesBuf, "%d,%s,%d,%s,%d,%d\n",
			v.timestamp, v.symbol, v.tradeID, side, v.price, v.qty)
	case snapshotRecord:
		// Write initial snapshot as SNAP
		// Observed
		for _, level := range v.snapshot.Bids {
			if level.VisibleQty > 0 {
				fmt.Fprintf(r.obsBuf, "%d,%d,%s,SNAP,bid,%d,%d\n",
					v.timestamp, v.seqNum, v.symbol, level.Price, level.VisibleQty)
			}
			if level.HiddenQty > 0 {
				fmt.Fprintf(r.hiddenBuf, "%d,%d,%s,SNAP,bid,%d,%d\n",
					v.timestamp, v.seqNum, v.symbol, level.Price, level.HiddenQty)
			}
		}
		for _, level := range v.snapshot.Asks {
			if level.VisibleQty > 0 {
				fmt.Fprintf(r.obsBuf, "%d,%d,%s,SNAP,ask,%d,%d\n",
					v.timestamp, v.seqNum, v.symbol, level.Price, level.VisibleQty)
			}
			if level.HiddenQty > 0 {
				fmt.Fprintf(r.hiddenBuf, "%d,%d,%s,SNAP,ask,%d,%d\n",
					v.timestamp, v.seqNum, v.symbol, level.Price, level.HiddenQty)
			}
		}
	case deltaRecord:
		side := "bid"
		if v.delta.Side == exchange.Sell {
			side = "ask"
		}
		// Write Delta
		// Observed
		fmt.Fprintf(r.obsBuf, "%d,%d,%s,DELTA,%s,%d,%d\n",
			v.timestamp, v.seqNum, v.symbol, side, v.delta.Price, v.delta.VisibleQty)

		// Hidden
		fmt.Fprintf(r.hiddenBuf, "%d,%d,%s,DELTA,%s,%d,%d\n",
			v.timestamp, v.seqNum, v.symbol, side, v.delta.Price, v.delta.HiddenQty)
	}
}

func (r *RecorderActor) snapshotLoop(ctx context.Context) {
	if r.config.SnapshotInterval <= 0 {
		return
	}
	ticker := time.NewTicker(r.config.SnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, symbol := range r.config.Symbols {
				// Re-subscribing triggers a snapshot
				r.Subscribe(symbol)
			}
		}
	}
}
