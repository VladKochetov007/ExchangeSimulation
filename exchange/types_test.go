package exchange

import "testing"

func TestTimeInForce_String(t *testing.T) {
	cases := []struct {
		v    TimeInForce
		want string
	}{
		{GTC, "GTC"},
		{IOC, "IOC"},
		{FOK, "FOK"},
		{TimeInForce(99), "UNKNOWN"},
	}
	for _, c := range cases {
		if got := c.v.String(); got != c.want {
			t.Errorf("TimeInForce(%d).String() = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestVisibility_String(t *testing.T) {
	cases := []struct {
		v    Visibility
		want string
	}{
		{Normal, "NORMAL"},
		{Iceberg, "ICEBERG"},
		{Hidden, "HIDDEN"},
		{Visibility(99), "UNKNOWN"},
	}
	for _, c := range cases {
		if got := c.v.String(); got != c.want {
			t.Errorf("Visibility(%d).String() = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestRejectReason_String(t *testing.T) {
	cases := []struct {
		v    RejectReason
		want string
	}{
		{RejectInsufficientBalance, "INSUFFICIENT_BALANCE"},
		{RejectInvalidPrice, "INVALID_PRICE"},
		{RejectInvalidQty, "INVALID_QTY"},
		{RejectUnknownClient, "UNKNOWN_CLIENT"},
		{RejectUnknownInstrument, "UNKNOWN_INSTRUMENT"},
		{RejectSelfTrade, "SELF_TRADE"},
		{RejectDuplicateOrderID, "DUPLICATE_ORDER_ID"},
		{RejectOrderNotFound, "ORDER_NOT_FOUND"},
		{RejectOrderNotOwned, "ORDER_NOT_OWNED"},
		{RejectOrderAlreadyFilled, "ORDER_ALREADY_FILLED"},
		{RejectFOKNotFilled, "FOK_NOT_FILLED"},
		{RejectReason(99), "UNKNOWN"},
	}
	for _, c := range cases {
		if got := c.v.String(); got != c.want {
			t.Errorf("RejectReason(%d).String() = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestMarginMode_String(t *testing.T) {
	cases := []struct {
		v    MarginMode
		want string
	}{
		{CrossMargin, "cross"},
		{IsolatedMargin, "isolated"},
		{MarginMode(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.v.String(); got != c.want {
			t.Errorf("MarginMode(%d).String() = %q, want %q", c.v, got, c.want)
		}
	}
}
