package exchange

import (
	"testing"
)

func TestRejectUnknownPositionSideBeforePerpMatch(t *testing.T) {
	ex, _ := fundingBookPairFixture(t, true)
	beforeOrderID := ex.NextOrderID
	beforeAsk := ex.Books["ABC-PERP"].Asks.Best.TotalQty
	unknownSide := PositionSide(255)

	response := ex.PlaceOrder(1, &OrderRequest{
		Symbol: "ABC-PERP", Side: Buy, PositionSide: unknownSide,
		Type: LimitOrder, Price: 105, Qty: 1, TimeInForce: GTC,
	})
	if response.Success || response.Error != RejectInvalidPositionSide {
		t.Fatalf("unknown position side was not rejected explicitly: %+v", response)
	}
	if ex.NextOrderID != beforeOrderID || ex.Books["ABC-PERP"].Asks.Best.TotalQty != beforeAsk ||
		ex.Positions.GetPositionBySide(1, "ABC-PERP", unknownSide) != nil {
		t.Fatalf("rejected position side changed order sequence, book or position")
	}
}
