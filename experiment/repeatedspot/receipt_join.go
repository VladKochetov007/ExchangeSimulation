package repeatedspot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"exchange_sim/experiment/executionpilot"
	"exchange_sim/simulation"
	"exchange_sim/types"
)

type receiptManifest struct {
	Receipts struct {
		File    string `json:"file"`
		Records int64  `json:"records"`
		Digest  string `json:"digest"`
	} `json:"receipts"`
	Symbols []struct {
		ID     uint32 `json:"id"`
		Symbol string `json:"symbol"`
	} `json:"symbols"`
}

type receiptIdentity struct {
	clientID    uint64
	messageType uint8
	sequence    uint64
	fingerprint [16]byte
}

type receiptTiming struct {
	publishedAt int64
	deliveredAt int64
}

func (state *replayState) verifyMakerReceipts(directory string) error {
	manifestRaw, err := os.ReadFile(filepath.Join(directory, "market-data-evidence-v2.json"))
	if err != nil {
		return err
	}
	if err := executionpilot.ValidateStrictJSON(manifestRaw); err != nil {
		return err
	}
	var manifest receiptManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return err
	}
	if manifest.Receipts.File != "market-data-receipts-v2.bin" || manifest.Receipts.Records < 0 {
		return errors.New("repeated spot: unsupported receipt sidecar identity")
	}
	var symbolID uint32
	for _, symbol := range manifest.Symbols {
		if symbol.Symbol == state.contract.Instrument.Symbol {
			symbolID = symbol.ID
		}
	}
	if symbolID == 0 {
		return errors.New("repeated spot: spot symbol absent from receipt catalog")
	}
	file, err := os.Open(filepath.Join(directory, manifest.Receipts.File))
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	receipts := make(map[receiptIdentity]receiptTiming)
	var raw [simulation.MarketDataReceiptRecordBytes]byte
	for index := int64(0); index < manifest.Receipts.Records; index++ {
		if _, err := io.ReadFull(file, raw[:]); err != nil {
			return fmt.Errorf("repeated spot: truncated receipt sidecar at %d: %w", index, err)
		}
		_, _ = hasher.Write(raw[:])
		if binary.BigEndian.Uint32(raw[12:16]) != symbolID {
			continue
		}
		identity := receiptIdentity{clientID: binary.BigEndian.Uint64(raw[0:8]), messageType: raw[16],
			sequence: binary.BigEndian.Uint64(raw[20:28])}
		copy(identity.fingerprint[:], raw[28:44])
		if _, duplicate := receipts[identity]; duplicate {
			return errors.New("repeated spot: duplicate maker receipt identity")
		}
		receipts[identity] = receiptTiming{publishedAt: int64(binary.BigEndian.Uint64(raw[44:52])),
			deliveredAt: int64(binary.BigEndian.Uint64(raw[60:68]))}
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); n != 0 || err != io.EOF {
		return errors.New("repeated spot: receipt sidecar has trailing bytes")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != manifest.Receipts.Digest {
		return errors.New("repeated spot: receipt sidecar digest changed during maker join")
	}
	for _, record := range state.makerObservations {
		observation := record.observation
		var message *types.MarketDataMsg
		if observation.Kind == "snapshot" {
			source := state.publicSnapshots[observation.SourceSequence]
			message = &types.MarketDataMsg{Type: types.MDSnapshot, Symbol: observation.Symbol,
				SeqNum: observation.SourceSequence, Timestamp: observation.SourceAt,
				Data: &types.BookSnapshot{Bids: source.bids, Asks: source.asks}}
		} else {
			source := state.trades.trades[observation.TradeID]
			side := types.Buy
			if source.trade.Side == "SELL" {
				side = types.Sell
			}
			message = &types.MarketDataMsg{Type: types.MDTrade, Symbol: observation.Symbol,
				SeqNum: observation.SourceSequence, Timestamp: observation.SourceAt,
				Data: &types.Trade{TradeID: source.trade.TradeID, Price: source.trade.Price,
					Qty: source.trade.Qty, Side: side, TakerOrderID: source.trade.TakerOrderID,
					MakerOrderID: source.trade.MakerOrderID}}
		}
		fingerprint, err := types.MarketDataFingerprint(message)
		if err != nil {
			return err
		}
		identity := receiptIdentity{clientID: record.clientID, messageType: uint8(message.Type),
			sequence: message.SeqNum, fingerprint: fingerprint}
		timing, found := receipts[identity]
		if !found || timing.publishedAt != observation.SourceAt || timing.deliveredAt > observation.ProcessedAt {
			return errors.New("repeated spot: maker processed information not bound to a prior delivered public message")
		}
	}
	return nil
}
