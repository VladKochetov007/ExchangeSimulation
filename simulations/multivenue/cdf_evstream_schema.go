package multivenue

import (
	"encoding/json"
	"fmt"

	"exchange_sim/evstream"
	etypes "exchange_sim/types"
)

// CDF successor schemas live in the application package that owns the
// participant contract. evstream deliberately has no central registry: these
// permanent IDs are private to this event family and are dispatched by the
// multivenue renderer alongside the exchange-owned schemas.
const (
	SchemaElasticLiquiditySupplierDecision uint16 = etypes.SchemaFirstExchange + 16
	SchemaElasticLiquiditySupplierFill     uint16 = etypes.SchemaFirstExchange + 17
)

const (
	cdfDecisionLegacyOptionalFields      = 12
	cdfDecisionOptionalFields            = 17
	cdfDecisionSideBit                   = 0
	cdfDecisionQuotePriceBit             = 1
	cdfDecisionQuoteQtyBit               = 2
	cdfDecisionMinimumQtyBit             = 3
	cdfDecisionRegisteredQtyBit          = 4
	cdfDecisionQuoteOrderBit             = 5
	cdfDecisionQuoteRequestBit           = 6
	cdfDecisionCancelRequestBit          = 7
	cdfDecisionSubmittedAtBit            = 8
	cdfDecisionCashAvailableBit          = 9
	cdfDecisionCashRequiredBit           = 10
	cdfDecisionReplacesOrderBit          = 11
	cdfDecisionObservationFingerprintBit = 12
	cdfDecisionObservationDigestBit      = 13
	cdfDecisionLocalBookModeBit          = 14
	cdfDecisionQuotePriceSourceBit       = 15
	cdfDecisionRiskMarkSourceBit         = 16
)

// The CDF decision wire layout is intentionally explicit. Strings are
// dictionary references, fixed-point values are signed integers, and the
// presence bitmap preserves the distinction between an omitted optional JSON
// field and a present nonzero field.
func (d ElasticLiquiditySupplierDecision) SchemaID() uint16 {
	return SchemaElasticLiquiditySupplierDecision
}

func (d ElasticLiquiditySupplierDecision) SchemaVersion() uint16 { return 4 }

func (d ElasticLiquiditySupplierDecision) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(cdfDecisionOptionalFields))...)
	if d.Side != "" {
		evstream.SetPresence(dst[start:], cdfDecisionSideBit)
	}
	if d.QuotePrice != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionQuotePriceBit)
	}
	if d.QuoteQty != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionQuoteQtyBit)
	}
	if d.MinimumQualifyingQty != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionMinimumQtyBit)
	}
	if d.RegisteredMinimumExecutableQty != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionRegisteredQtyBit)
	}
	if d.QuoteOrderID != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionQuoteOrderBit)
	}
	if d.QuoteRequestID != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionQuoteRequestBit)
	}
	if d.CancelRequestID != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionCancelRequestBit)
	}
	if d.QuoteSubmittedAt != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionSubmittedAtBit)
	}
	if d.QuoteCashAvailable != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionCashAvailableBit)
	}
	if d.QuoteCashRequired != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionCashRequiredBit)
	}
	if d.ReplacesOrderID != 0 {
		evstream.SetPresence(dst[start:], cdfDecisionReplacesOrderBit)
	}
	if d.ObservationFingerprint != "" {
		evstream.SetPresence(dst[start:], cdfDecisionObservationFingerprintBit)
	}
	if d.ObservationDigest != "" {
		evstream.SetPresence(dst[start:], cdfDecisionObservationDigestBit)
	}
	if d.LocalBookMode != "" {
		evstream.SetPresence(dst[start:], cdfDecisionLocalBookModeBit)
	}
	if d.QuotePriceSource != "" {
		evstream.SetPresence(dst[start:], cdfDecisionQuotePriceSourceBit)
	}
	if d.RiskMarkSource != "" {
		evstream.SetPresence(dst[start:], cdfDecisionRiskMarkSourceBit)
	}

	stringValues := [...]string{
		d.Role, d.Symbol, d.ObservationFingerprint, d.ObservationDigest,
		d.LocalBookMode, d.QuotePriceSource, d.RiskMarkSource, d.Action, d.Reason, d.Side,
	}
	optionalStringBits := [...]int{
		-1, -1, cdfDecisionObservationFingerprintBit, cdfDecisionObservationDigestBit,
		cdfDecisionLocalBookModeBit, cdfDecisionQuotePriceSourceBit, cdfDecisionRiskMarkSourceBit,
		-1, -1, cdfDecisionSideBit,
	}
	for index, value := range stringValues {
		if optionalStringBits[index] >= 0 && value == "" {
			dst = evstream.AppendUint32(dst, 0)
			continue
		}
		ref, err := in.Intern(value)
		if err != nil {
			return nil, err
		}
		dst = evstream.AppendUint32(dst, ref)
	}

	dst = evstream.AppendUint64(dst, d.ClientID)
	dst = evstream.AppendInt64(dst, d.DecisionTime)
	dst = evstream.AppendInt64(dst, d.DecisionPhaseOffset)
	dst = evstream.AppendInt64(dst, d.ObservationTime)
	dst = evstream.AppendInt64(dst, d.ObservationAge)
	dst = evstream.AppendUint64(dst, d.ObservationSequence)
	dst = evstream.AppendUint32(dst, d.ObservationLinkID)
	dst = evstream.AppendUint64(dst, d.ObservationOrdinal)
	dst = evstream.AppendInt64(dst, d.ObservationDeliveredAt)
	dst = evstream.AppendInt64(dst, d.BestBid)
	dst = evstream.AppendInt64(dst, d.BestBidQty)
	dst = evstream.AppendInt64(dst, d.BestAsk)
	dst = evstream.AppendInt64(dst, d.BestAskQty)
	dst = evstream.AppendInt64(dst, d.MarkPrice)
	dst = evstream.AppendInt64(dst, d.RiskMarkPrice)
	dst = evstream.AppendInt64(dst, d.ReferencePrice)
	dst = evstream.AppendInt64(dst, d.Position)
	dst = evstream.AppendInt64(dst, d.TargetPosition)
	dst = evstream.AppendInt64(dst, d.InventoryLimit)
	dst = evstream.AppendInt64(dst, d.InitialBaseBalance)
	dst = evstream.AppendInt64(dst, d.GrossInventory)
	dst = evstream.AppendInt64(dst, d.GrossInventoryLimit)
	dst = evstream.AppendInt64(dst, d.QuotePrice)
	dst = evstream.AppendInt64(dst, d.QuoteQty)
	dst = evstream.AppendInt64(dst, d.MinimumQualifyingQty)
	dst = evstream.AppendInt64(dst, d.RegisteredMinimumExecutableQty)
	dst = evstream.AppendUint64(dst, d.QuoteOrderID)
	dst = evstream.AppendUint64(dst, d.QuoteRequestID)
	dst = evstream.AppendUint64(dst, d.CancelRequestID)
	dst = evstream.AppendInt64(dst, d.QuoteSubmittedAt)
	dst = evstream.AppendInt64(dst, d.QuoteCashAvailable)
	dst = evstream.AppendInt64(dst, d.QuoteCashReserved)
	dst = evstream.AppendInt64(dst, d.QuoteCashRequired)
	dst = evstream.AppendInt64(dst, d.InitialEquityQuote)
	dst = evstream.AppendInt64(dst, d.EquityQuote)
	dst = evstream.AppendInt64(dst, d.PeakEquityQuote)
	dst = evstream.AppendInt64(dst, d.LossFromInitialQuote)
	dst = evstream.AppendInt64(dst, d.DrawdownQuote)
	dst = evstream.AppendInt64(dst, d.MaxLossQuote)
	dst = evstream.AppendBool(dst, d.EquityAvailable)
	dst = evstream.AppendBool(dst, d.RiskLimitTriggered)
	dst = evstream.AppendBool(dst, d.RiskMarkCurrent)
	return evstream.AppendUint64(dst, d.ReplacesOrderID), nil
}

func decodeElasticLiquiditySupplierDecision(payload []byte, resolve evstream.Resolver, into *ElasticLiquiditySupplierDecision) error {
	return decodeElasticLiquiditySupplierDecisionVersioned(payload, resolve, into, 1)
}

func decodeElasticLiquiditySupplierDecisionVersioned(payload []byte, resolve evstream.Resolver, into *ElasticLiquiditySupplierDecision, schemaVersion uint16) error {
	if schemaVersion < 1 || schemaVersion > 4 {
		return fmt.Errorf("%w: unsupported CDF decision schema version %d", evstream.ErrCorrupt, schemaVersion)
	}
	cursor := evstream.NewCursor(payload)
	optionalFields := cdfDecisionLegacyOptionalFields
	if schemaVersion >= 4 {
		optionalFields = cdfDecisionOptionalFields
	}
	presence := cursor.Presence(optionalFields)
	var stringRefs [10]uint32
	for index := range stringRefs {
		stringRefs[index] = cursor.Uint32()
	}
	into.ClientID = cursor.Uint64()
	into.DecisionTime = cursor.Int64()
	into.DecisionPhaseOffset = cursor.Int64()
	into.ObservationTime = cursor.Int64()
	into.ObservationAge = cursor.Int64()
	into.ObservationSequence = cursor.Uint64()
	into.ObservationLinkID = cursor.Uint32()
	into.ObservationOrdinal = cursor.Uint64()
	into.ObservationDeliveredAt = cursor.Int64()
	into.BestBid = cursor.Int64()
	into.BestBidQty = cursor.Int64()
	into.BestAsk = cursor.Int64()
	into.BestAskQty = cursor.Int64()
	into.MarkPrice = cursor.Int64()
	into.RiskMarkPrice = cursor.Int64()
	into.ReferencePrice = cursor.Int64()
	into.Position = cursor.Int64()
	into.TargetPosition = cursor.Int64()
	into.InventoryLimit = cursor.Int64()
	into.InitialBaseBalance = cursor.Int64()
	into.GrossInventory = cursor.Int64()
	into.GrossInventoryLimit = cursor.Int64()
	into.QuotePrice = cursor.Int64()
	into.QuoteQty = cursor.Int64()
	into.MinimumQualifyingQty = cursor.Int64()
	into.RegisteredMinimumExecutableQty = cursor.Int64()
	into.QuoteOrderID = cursor.Uint64()
	into.QuoteRequestID = cursor.Uint64()
	into.CancelRequestID = cursor.Uint64()
	into.QuoteSubmittedAt = cursor.Int64()
	into.QuoteCashAvailable = cursor.Int64()
	into.QuoteCashReserved = cursor.Int64()
	into.QuoteCashRequired = cursor.Int64()
	into.InitialEquityQuote = cursor.Int64()
	into.EquityQuote = cursor.Int64()
	into.PeakEquityQuote = cursor.Int64()
	into.LossFromInitialQuote = cursor.Int64()
	into.DrawdownQuote = cursor.Int64()
	into.MaxLossQuote = cursor.Int64()
	into.EquityAvailable = cursor.Bool()
	into.RiskLimitTriggered = cursor.Bool()
	if schemaVersion >= 2 {
		into.RiskMarkCurrent = cursor.Bool()
	}
	if schemaVersion >= 3 {
		into.ReplacesOrderID = cursor.Uint64()
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	if cdfPresenceHasUnknownBits(presence, optionalFields) {
		return fmt.Errorf("%w: unknown CDF decision presence bit", evstream.ErrCorrupt)
	}
	if schemaVersion >= 4 {
		if err := validateCDFDecisionOptionalNumbers(presence, *into); err != nil {
			return err
		}
	}
	if schemaVersion < 4 {
		legacyValues := [...]*string{
			&into.Role, &into.Symbol, &into.ObservationFingerprint, &into.ObservationDigest,
			&into.LocalBookMode, &into.QuotePriceSource, &into.RiskMarkSource, &into.Action, &into.Reason,
		}
		for index, target := range legacyValues {
			value, err := evstream.ResolveRequired(resolve, stringRefs[index])
			if err != nil {
				return err
			}
			*target = value
		}
		if presence.Has(cdfDecisionSideBit) {
			value, err := evstream.ResolveRequired(resolve, stringRefs[9])
			if err != nil {
				return err
			}
			into.Side = value
		} else {
			if stringRefs[9] != 0 {
				return fmt.Errorf("%w: absent CDF decision side has a nonzero reference", evstream.ErrCorrupt)
			}
			into.Side = ""
		}
	} else {
		var err error
		if into.Role, err = evstream.ResolveRequired(resolve, stringRefs[0]); err != nil {
			return err
		}
		if into.Symbol, err = evstream.ResolveRequired(resolve, stringRefs[1]); err != nil {
			return err
		}
		if into.ObservationFingerprint, err = resolveOptionalCDFString(resolve, presence, cdfDecisionObservationFingerprintBit, stringRefs[2]); err != nil {
			return err
		}
		if into.ObservationDigest, err = resolveOptionalCDFString(resolve, presence, cdfDecisionObservationDigestBit, stringRefs[3]); err != nil {
			return err
		}
		if into.LocalBookMode, err = resolveOptionalCDFString(resolve, presence, cdfDecisionLocalBookModeBit, stringRefs[4]); err != nil {
			return err
		}
		if into.QuotePriceSource, err = resolveOptionalCDFString(resolve, presence, cdfDecisionQuotePriceSourceBit, stringRefs[5]); err != nil {
			return err
		}
		if into.RiskMarkSource, err = resolveOptionalCDFString(resolve, presence, cdfDecisionRiskMarkSourceBit, stringRefs[6]); err != nil {
			return err
		}
		if into.Action, err = evstream.ResolveRequired(resolve, stringRefs[7]); err != nil {
			return err
		}
		if into.Reason, err = evstream.ResolveRequired(resolve, stringRefs[8]); err != nil {
			return err
		}
		if into.Side, err = resolveOptionalCDFString(resolve, presence, cdfDecisionSideBit, stringRefs[9]); err != nil {
			return err
		}
	}
	if !presence.Has(cdfDecisionQuotePriceBit) {
		into.QuotePrice = 0
	}
	if !presence.Has(cdfDecisionQuoteQtyBit) {
		into.QuoteQty = 0
	}
	if !presence.Has(cdfDecisionMinimumQtyBit) {
		into.MinimumQualifyingQty = 0
	}
	if !presence.Has(cdfDecisionRegisteredQtyBit) {
		into.RegisteredMinimumExecutableQty = 0
	}
	if !presence.Has(cdfDecisionQuoteOrderBit) {
		into.QuoteOrderID = 0
	}
	if !presence.Has(cdfDecisionQuoteRequestBit) {
		into.QuoteRequestID = 0
	}
	if !presence.Has(cdfDecisionCancelRequestBit) {
		into.CancelRequestID = 0
	}
	if !presence.Has(cdfDecisionSubmittedAtBit) {
		into.QuoteSubmittedAt = 0
	}
	if !presence.Has(cdfDecisionCashAvailableBit) {
		into.QuoteCashAvailable = 0
	}
	if !presence.Has(cdfDecisionCashRequiredBit) {
		into.QuoteCashRequired = 0
	}
	if !presence.Has(cdfDecisionReplacesOrderBit) {
		into.ReplacesOrderID = 0
	}
	return finishCDFSchemaCursor(cursor)
}

func validateCDFDecisionOptionalNumbers(presence evstream.PresenceSet, decision ElasticLiquiditySupplierDecision) error {
	int64Fields := [...]struct {
		name    string
		present bool
		value   int64
	}{
		{"QuotePrice", presence.Has(cdfDecisionQuotePriceBit), decision.QuotePrice},
		{"QuoteQty", presence.Has(cdfDecisionQuoteQtyBit), decision.QuoteQty},
		{"MinimumQualifyingQty", presence.Has(cdfDecisionMinimumQtyBit), decision.MinimumQualifyingQty},
		{"RegisteredMinimumExecutableQty", presence.Has(cdfDecisionRegisteredQtyBit), decision.RegisteredMinimumExecutableQty},
		{"QuoteSubmittedAt", presence.Has(cdfDecisionSubmittedAtBit), decision.QuoteSubmittedAt},
		{"QuoteCashAvailable", presence.Has(cdfDecisionCashAvailableBit), decision.QuoteCashAvailable},
		{"QuoteCashRequired", presence.Has(cdfDecisionCashRequiredBit), decision.QuoteCashRequired},
	}
	for _, field := range int64Fields {
		if field.present == (field.value == 0) {
			return fmt.Errorf("%w: CDF decision optional numeric field %s has contradictory presence and value", evstream.ErrCorrupt, field.name)
		}
	}
	uint64Fields := [...]struct {
		name    string
		present bool
		value   uint64
	}{
		{"QuoteOrderID", presence.Has(cdfDecisionQuoteOrderBit), decision.QuoteOrderID},
		{"QuoteRequestID", presence.Has(cdfDecisionQuoteRequestBit), decision.QuoteRequestID},
		{"CancelRequestID", presence.Has(cdfDecisionCancelRequestBit), decision.CancelRequestID},
		{"ReplacesOrderID", presence.Has(cdfDecisionReplacesOrderBit), decision.ReplacesOrderID},
	}
	for _, field := range uint64Fields {
		if field.present == (field.value == 0) {
			return fmt.Errorf("%w: CDF decision optional numeric field %s has contradictory presence and value", evstream.ErrCorrupt, field.name)
		}
	}
	return nil
}

func resolveOptionalCDFString(resolve evstream.Resolver, presence evstream.PresenceSet, bit int, ref uint32) (string, error) {
	if !presence.Has(bit) {
		if ref != 0 {
			return "", fmt.Errorf("%w: absent CDF string field has a nonzero reference", evstream.ErrCorrupt)
		}
		return "", nil
	}
	return evstream.ResolveRequired(resolve, ref)
}

func (f ElasticLiquiditySupplierFill) SchemaID() uint16 { return SchemaElasticLiquiditySupplierFill }

func (f ElasticLiquiditySupplierFill) SchemaVersion() uint16 { return 1 }

func (f ElasticLiquiditySupplierFill) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	for _, value := range [...]string{f.Role, f.Symbol, f.Side, f.FeeAsset} {
		ref, err := in.Intern(value)
		if err != nil {
			return nil, err
		}
		dst = evstream.AppendUint32(dst, ref)
	}
	dst = evstream.AppendUint64(dst, f.ClientID)
	dst = evstream.AppendUint64(dst, f.OrderID)
	dst = evstream.AppendUint64(dst, f.TradeID)
	dst = evstream.AppendInt64(dst, f.Timestamp)
	dst = evstream.AppendInt64(dst, f.Price)
	dst = evstream.AppendInt64(dst, f.Qty)
	dst = evstream.AppendInt64(dst, f.FeeAmount)
	dst = evstream.AppendBool(dst, f.IsFull)
	dst = evstream.AppendInt64(dst, f.PositionBefore)
	return evstream.AppendInt64(dst, f.PositionAfter), nil
}

func decodeElasticLiquiditySupplierFill(payload []byte, resolve evstream.Resolver, into *ElasticLiquiditySupplierFill) error {
	cursor := evstream.NewCursor(payload)
	var stringRefs [4]uint32
	for index := range stringRefs {
		stringRefs[index] = cursor.Uint32()
	}
	into.ClientID = cursor.Uint64()
	into.OrderID = cursor.Uint64()
	into.TradeID = cursor.Uint64()
	into.Timestamp = cursor.Int64()
	into.Price = cursor.Int64()
	into.Qty = cursor.Int64()
	into.FeeAmount = cursor.Int64()
	into.IsFull = cursor.Bool()
	into.PositionBefore = cursor.Int64()
	into.PositionAfter = cursor.Int64()
	if err := cursor.Err(); err != nil {
		return err
	}
	values := [...]*string{&into.Role, &into.Symbol, &into.Side, &into.FeeAsset}
	for index, target := range values {
		value, err := evstream.ResolveRequired(resolve, stringRefs[index])
		if err != nil {
			return err
		}
		*target = value
	}
	return finishCDFSchemaCursor(cursor)
}

func renderCDFPayloadJSONVersioned(schemaID, schemaVersion uint16, payload []byte, resolve evstream.Resolver) ([]byte, bool, error) {
	switch schemaID {
	case SchemaElasticLiquiditySupplierDecision:
		if schemaVersion != 1 && schemaVersion != 2 && schemaVersion != 3 && schemaVersion != 4 {
			return nil, true, fmt.Errorf("%w: unsupported CDF decision schema version %d", evstream.ErrCorrupt, schemaVersion)
		}
		var value ElasticLiquiditySupplierDecision
		if err := decodeElasticLiquiditySupplierDecisionVersioned(payload, resolve, &value, schemaVersion); err != nil {
			return nil, true, err
		}
		raw, err := json.Marshal(value)
		return raw, true, err
	case SchemaElasticLiquiditySupplierFill:
		if schemaVersion != 1 {
			return nil, true, fmt.Errorf("%w: unsupported CDF fill schema version %d", evstream.ErrCorrupt, schemaVersion)
		}
		var value ElasticLiquiditySupplierFill
		if err := decodeElasticLiquiditySupplierFill(payload, resolve, &value); err != nil {
			return nil, true, err
		}
		raw, err := json.Marshal(value)
		return raw, true, err
	default:
		return nil, false, nil
	}
}

func cdfPresenceHasUnknownBits(presence evstream.PresenceSet, fieldCount int) bool {
	if fieldCount == 0 || len(presence) == 0 || fieldCount%8 == 0 {
		return false
	}
	knownBits := byte((1 << uint(fieldCount%8)) - 1)
	return presence[len(presence)-1]&^knownBits != 0
}

func finishCDFSchemaCursor(cursor *evstream.Cursor) error {
	if err := cursor.Err(); err != nil {
		return err
	}
	if cursor.Remaining() != 0 {
		return evstream.ErrCorrupt
	}
	return nil
}

var (
	_ evstream.InterningAppender = ElasticLiquiditySupplierDecision{}
	_ evstream.InterningAppender = ElasticLiquiditySupplierFill{}
)
