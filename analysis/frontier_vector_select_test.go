package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
)

func TestSelectVerifiedDecisionFrontierVectorsKeepsTypedIdentity(t *testing.T) {
	dir := writeFrontierVectorOracleFixture(t)
	rows, err := SelectVerifiedDecisionFrontierVectors(dir, DecisionFrontierVectorSelection{
		Symbol: "ABC/USD", ClientByLink: map[uint32]uint64{1: 11},
	})
	if err != nil || len(rows) != 1 || rows[0].DecisionID != 1 || rows[0].ActorID != 99 || rows[0].ClientID != 11 ||
		rows[0].RequestID != 31 || rows[0].TradingLinkID != 1 || rows[0].Side != exchange.Buy ||
		rows[0].OrderType != exchange.LimitOrder || rows[0].TimeInForce != exchange.GTC ||
		rows[0].Price != 99 || rows[0].Qty != 3 || rows[0].DecisionAt != 120 || rows[0].ComponentCount != 1 {
		t.Fatalf("selected verified decision = %#v, %v", rows, err)
	}
	if len(rows[0].Components) != 1 || rows[0].Components[0].ClientID != 11 || rows[0].Components[0].LinkID != 1 ||
		rows[0].Components[0].Ordinal != 1 || rows[0].Components[0].DeliveredAt != 110 || rows[0].Components[0].Digest == ([16]byte{}) {
		t.Fatalf("verified decision component = %#v", rows[0].Components)
	}
	if _, err := SelectVerifiedDecisionFrontierVectors(dir, DecisionFrontierVectorSelection{
		Symbol: "XYZ/USD", ClientByLink: map[uint32]uint64{1: 11},
	}); err == nil {
		t.Fatal("selected actor's other-symbol decision omitted from complete audit")
	}
}

func TestSelectVerifiedDecisionFrontierVectorsRejectsUnverifiedOrWrongLink(t *testing.T) {
	dir := writeFrontierVectorOracleFixture(t)
	if _, err := SelectVerifiedDecisionFrontierVectors(dir, DecisionFrontierVectorSelection{
		Symbol: "ABC/USD", ClientByLink: map[uint32]uint64{1: 12},
	}); err == nil {
		t.Fatal("same link attributed to wrong client")
	}
	if _, err := SelectVerifiedDecisionFrontierVectors(dir, DecisionFrontierVectorSelection{
		Symbol: "ABC/USD", ClientByLink: map[uint32]uint64{2: 11},
	}); err == nil {
		t.Fatal("unregistered link misread as no actor decisions")
	}
	path := filepath.Join(dir, "market-data-decision-vectors-v1.bin")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[24] ^= 1
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectVerifiedDecisionFrontierVectors(dir, DecisionFrontierVectorSelection{
		Symbol: "ABC/USD", ClientByLink: map[uint32]uint64{1: 11},
	}); err == nil {
		t.Fatal("mutated decision vector selected as verified")
	}
}
