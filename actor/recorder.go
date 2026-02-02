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
	SnapshotsPath string
	FlushInterval time.Duration
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
}

type RecorderActor struct {
	*BaseActor
	config     RecorderConfig
	writeCh    chan interface{}
	tradesFile *os.File
	snapsFile  *os.File
	tradesBuf  *bufio.Writer
	snapsBuf   *bufio.Writer
}

func NewRecorder(id uint64, gateway *exchange.ClientGateway, config RecorderConfig) (*RecorderActor, error) {
	if config.FlushInterval == 0 {
		config.FlushInterval = time.Second
	}

	tradesFile, err := os.Create(config.TradesPath)
	if err != nil {
		return nil, err
	}

	snapsFile, err := os.Create(config.SnapshotsPath)
	if err != nil {
		tradesFile.Close()
		return nil, err
	}

	r := &RecorderActor{
		BaseActor:  NewBaseActor(id, gateway),
		config:     config,
		writeCh:    make(chan interface{}, 10000),
		tradesFile: tradesFile,
		snapsFile:  snapsFile,
		tradesBuf:  bufio.NewWriter(tradesFile),
		snapsBuf:   bufio.NewWriter(snapsFile),
	}

	r.tradesBuf.WriteString("timestamp,symbol,trade_id,side,price,qty\n")
	r.snapsBuf.WriteString("timestamp,symbol,side,level,price,qty\n")

	return r, nil
}

func (r *RecorderActor) Start(ctx context.Context) error {
	for _, symbol := range r.config.Symbols {
		r.Subscribe(symbol)
	}
	go r.eventLoop(ctx)
	go r.writeLoop(ctx)
	return r.BaseActor.Start(ctx)
}

func (r *RecorderActor) Stop() error {
	close(r.writeCh)
	r.tradesBuf.Flush()
	r.snapsBuf.Flush()
	r.tradesFile.Close()
	r.snapsFile.Close()
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
	select {
	case r.writeCh <- rec:
	default:
	}
}

func (r *RecorderActor) onSnapshot(snapshot BookSnapshotEvent) {
	rec := snapshotRecord{
		timestamp: snapshot.Timestamp,
		symbol:    snapshot.Symbol,
		snapshot:  snapshot.Snapshot,
	}
	select {
	case r.writeCh <- rec:
	default:
	}
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
			r.snapsBuf.Flush()
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
			r.snapsBuf.Flush()
			return
		}
	}
}

func (r *RecorderActor) writeRecord(rec interface{}) {
	switch v := rec.(type) {
	case tradeRecord:
		side := "buy"
		if v.side == exchange.Sell {
			side = "sell"
		}
		fmt.Fprintf(r.tradesBuf, "%d,%s,%d,%s,%d,%d\n",
			v.timestamp, v.symbol, v.tradeID, side, v.price, v.qty)
	case snapshotRecord:
		for i, level := range v.snapshot.Bids {
			fmt.Fprintf(r.snapsBuf, "%d,%s,bid,%d,%d,%d\n",
				v.timestamp, v.symbol, i, level.Price, level.Qty)
		}
		for i, level := range v.snapshot.Asks {
			fmt.Fprintf(r.snapsBuf, "%d,%s,ask,%d,%d,%d\n",
				v.timestamp, v.symbol, i, level.Price, level.Qty)
		}
	}
}
