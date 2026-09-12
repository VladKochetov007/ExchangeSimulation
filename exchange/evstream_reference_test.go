package exchange

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"exchange_sim/evstream"
	etypes "exchange_sim/types"
)

type permissiveReferenceResolver struct {
	delegate evstream.Resolver
}

func (r permissiveReferenceResolver) Lookup(ref uint32) (string, bool) {
	if ref == 0 {
		return "", true
	}
	return r.delegate.Lookup(ref)
}

func TestExchangeRequiredEvidenceRejectsEmptyDictionaryValues(t *testing.T) {
	cases := []struct {
		name    string
		payload evstream.InterningAppender
	}{
		{name: "fill", payload: fillEvidence{}},
		{name: "book delta", payload: bookDeltaEvidence{}},
		{name: "venue balance", payload: VenueBalanceEvent{}},
		{name: "balance change", payload: etypes.BalanceChangeEvent{}},
		{name: "fee revenue", payload: etypes.FeeRevenueEvent{}},
		{name: "instrument wrapper", payload: instrumentLogEvent{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			writer := evstream.NewWriter(&output, evstream.WriterOptions{})
			err := writer.AppendInterning(1, 1, 0, tc.payload)
			if !errors.Is(err, evstream.ErrEmptyDictionaryValue) {
				t.Fatalf("empty required field error = %v, want ErrEmptyDictionaryValue", err)
			}
		})
	}
}

func TestExchangeRequiredEvidenceDecodersRejectZeroReference(t *testing.T) {
	t.Run("fill", func(t *testing.T) {
		frame, reader := roundTripFrame(t, fillEvidence{
			FeeAsset: "USD", PositionSide: "BOTH", Role: "maker", Side: "BUY", Symbol: "ABC/USD",
		})
		zeroReferenceAfter(t, frame.Payload, func(cursor *evstream.Cursor) {
			cursor.Int64()
			cursor.Int64()
			cursor.Bool()
			cursor.Int64()
			cursor.Int64()
			cursor.Uint64()
			cursor.Int64()
			cursor.Int64()
			cursor.Int64()
			cursor.Int64()
			cursor.Uint64()
		})
		var decoded fillEvidence
		if err := DecodeFillEvidence(frame.Payload, permissiveReferenceResolver{delegate: reader}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero fill reference error = %v, want ErrCorrupt", err)
		}
	})

	t.Run("book delta", func(t *testing.T) {
		frame, reader := roundTripFrame(t, bookDeltaEvidence{Side: "BUY"})
		zeroReferenceAfter(t, frame.Payload, func(cursor *evstream.Cursor) {
			for range 4 {
				cursor.Int64()
			}
		})
		var decoded bookDeltaEvidence
		if err := DecodeBookDelta(frame.Payload, permissiveReferenceResolver{delegate: reader}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero book-delta reference error = %v, want ErrCorrupt", err)
		}
	})

	t.Run("venue balance", func(t *testing.T) {
		frame, reader := roundTripFrame(t, VenueBalanceEvent{Bucket: VenueFeeRevenue, Asset: "USD", Reason: "fee"})
		zeroReferenceAfter(t, frame.Payload, func(cursor *evstream.Cursor) {
			cursor.Presence(venueBalanceOptionalFields)
			for range 6 {
				cursor.Int64()
			}
		})
		var decoded VenueBalanceEvent
		if err := DecodeVenueBalance(frame.Payload, permissiveReferenceResolver{delegate: reader}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero venue-balance reference error = %v, want ErrCorrupt", err)
		}
	})

	t.Run("instrument wrapper", func(t *testing.T) {
		frame, reader := roundTripFrame(t, instrumentLogEvent{Symbol: "ABC/USD", Payload: bookDeltaEvidence{Side: "BUY"}})
		binary.LittleEndian.PutUint32(frame.Payload, 0)
		if _, err := RenderPayloadJSONVersioned(SchemaInstrumentLog, 1, frame.Payload, permissiveReferenceResolver{delegate: reader}); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero instrument-wrapper reference error = %v, want ErrCorrupt", err)
		}
	})
}

func zeroReferenceAfter(t *testing.T, payload []byte, readFields func(*evstream.Cursor)) {
	t.Helper()
	cursor := evstream.NewCursor(payload)
	readFields(cursor)
	if err := cursor.Err(); err != nil {
		t.Fatalf("locate dictionary reference: %v", err)
	}
	offset := cursor.Offset()
	if len(payload)-offset < 4 {
		t.Fatalf("dictionary reference offset %d exceeds payload length %d", offset, len(payload))
	}
	binary.LittleEndian.PutUint32(payload[offset:offset+4], 0)
}
