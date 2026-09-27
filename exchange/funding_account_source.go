package exchange

import (
	"fmt"
	"math/big"
	"slices"
)

// FundingPositionSnapshotter supplies a complete, copied position inventory
// for one symbol, including retained zero-size entries. An E2 source cannot
// infer completeness from a callback that visits only active known sides.
type FundingPositionSnapshotter interface {
	SnapshotFundingPositions(symbol string) ([]Position, error)
}

type FundingAccountSnapshotRequest struct {
	VenueID           string
	PerpSymbol        string
	TimestampNano     int64
	ExpectedClientIDs []uint64
}

type FundingPositionLeg struct {
	Side PositionSide
	Size int64
}

type FundingAccountState struct {
	ClientID    uint64
	PerpCash    int64
	NetPosition int64
	Legs        []FundingPositionLeg
}

// FundingAccountSnapshot is a venue-local read, not a committable funding
// batch. Remainder ownership, the finite reserve, canonical evidence and the
// runner's pre-instant phase must be attested separately.
type FundingAccountSnapshot struct {
	VenueID       string
	PerpSymbol    string
	QuoteAsset    string
	BasePrecision int64
	TimestampNano int64
	Accounts      []FundingAccountState
}

// SnapshotFundingPositions scans every stored key, including malformed and
// zero-size keys, while holding the position store's read lock. It returns
// every valid stored entry, sorted by client and side.
func (pm *PositionManager) SnapshotFundingPositions(symbol string) ([]Position, error) {
	if pm == nil || symbol == "" {
		return nil, fmt.Errorf("funding positions: missing store or symbol")
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	positions := make([]Position, 0)
	for clientID, clientPositions := range pm.positions {
		for key, position := range clientPositions {
			if key.Symbol != symbol {
				if position != nil && position.Symbol == symbol {
					return nil, fmt.Errorf("funding positions: position symbol disagrees with stored key")
				}
				continue
			}
			if position == nil || position.ClientID != clientID || position.Symbol != symbol ||
				position.PositionSide != key.Side || !fundingPositionSideValid(key.Side) ||
				key.Side == PositionLong && position.Size < 0 ||
				key.Side == PositionShort && position.Size > 0 {
				return nil, fmt.Errorf("funding positions: malformed stored position for client %d", clientID)
			}
			positions = append(positions, *position)
		}
	}
	slices.SortFunc(positions, func(left, right Position) int {
		if left.ClientID != right.ClientID {
			if left.ClientID < right.ClientID {
				return -1
			}
			return 1
		}
		return int(left.PositionSide) - int(right.PositionSide)
	})
	return positions, nil
}

func fundingPositionSideValid(side PositionSide) bool {
	switch side {
	case PositionBoth, PositionLong, PositionShort:
		return true
	default:
		return false
	}
}

// CaptureFundingAccountSnapshot reads venue wallets and complete positions
// under one exchange read lock. The caller must own the runner clock and invoke
// this at the verified t-minus phase; the lock cannot prove that phase alone.
func (e *DefaultExchange) CaptureFundingAccountSnapshot(request FundingAccountSnapshotRequest) (FundingAccountSnapshot, error) {
	if e == nil || e.Clock == nil || request.VenueID == "" || request.PerpSymbol == "" || request.TimestampNano < 0 {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: invalid venue, symbol or timestamp")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.ID != request.VenueID || e.Clock.NowUnixNano() != request.TimestampNano {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: venue or live clock does not match requested source")
	}
	perp := e.Instruments[request.PerpSymbol]
	book := e.Books[request.PerpSymbol]
	if !fundingInstrumentBindingMatches(perp, book) || book.Symbol != request.PerpSymbol ||
		perp.InstrumentType() != "PERP" || !perp.IsPerp() || perp.Symbol() != request.PerpSymbol ||
		perp.QuoteAsset() == "" || perp.BasePrecision() <= 0 {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: incompatible perpetual instrument or book binding")
	}
	positionSource, ok := e.Positions.(FundingPositionSnapshotter)
	if !ok {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: position store cannot attest a complete symbol inventory")
	}
	registered := slices.Clone(request.ExpectedClientIDs)
	slices.Sort(registered)
	if len(registered) != len(e.Clients) {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: registered client roster changed")
	}
	for index, clientID := range registered {
		if index > 0 && registered[index-1] == clientID || e.Clients[clientID] == nil || e.Clients[clientID].ID != clientID {
			return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: registered client roster or account identity changed")
		}
	}
	positions, err := positionSource.SnapshotFundingPositions(request.PerpSymbol)
	if err != nil {
		return FundingAccountSnapshot{}, err
	}
	legsByClient := make(map[uint64][]FundingPositionLeg, len(registered))
	seenSide := make(map[uint64]map[PositionSide]bool, len(registered))
	for _, position := range positions {
		if e.Clients[position.ClientID] == nil || position.Symbol != request.PerpSymbol ||
			!fundingPositionSideValid(position.PositionSide) ||
			position.PositionSide == PositionLong && position.Size < 0 ||
			position.PositionSide == PositionShort && position.Size > 0 {
			return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: orphaned or malformed position for client %d", position.ClientID)
		}
		if seenSide[position.ClientID] == nil {
			seenSide[position.ClientID] = make(map[PositionSide]bool)
		}
		if seenSide[position.ClientID][position.PositionSide] {
			return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: duplicate position side for client %d", position.ClientID)
		}
		seenSide[position.ClientID][position.PositionSide] = true
		if position.Size == 0 {
			continue
		}
		legsByClient[position.ClientID] = append(legsByClient[position.ClientID], FundingPositionLeg{
			Side: position.PositionSide, Size: position.Size,
		})
	}
	result := FundingAccountSnapshot{
		VenueID: request.VenueID, PerpSymbol: request.PerpSymbol,
		QuoteAsset: perp.QuoteAsset(), BasePrecision: perp.BasePrecision(),
		TimestampNano: request.TimestampNano, Accounts: make([]FundingAccountState, 0, len(registered)),
	}
	venueNet := new(big.Int)
	for _, clientID := range registered {
		client := e.Clients[clientID]
		cash := client.PerpBalances[result.QuoteAsset]
		if cash < 0 {
			return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: client %d begins with negative perp cash", clientID)
		}
		legs := legsByClient[clientID]
		slices.SortFunc(legs, func(left, right FundingPositionLeg) int {
			return int(left.Side) - int(right.Side)
		})
		clientNet := new(big.Int)
		for _, leg := range legs {
			clientNet.Add(clientNet, big.NewInt(leg.Size))
		}
		if !clientNet.IsInt64() {
			return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: client %d net position overflows int64", clientID)
		}
		venueNet.Add(venueNet, clientNet)
		result.Accounts = append(result.Accounts, FundingAccountState{
			ClientID: clientID, PerpCash: cash, NetPosition: clientNet.Int64(), Legs: legs,
		})
	}
	if venueNet.Sign() != 0 {
		return FundingAccountSnapshot{}, fmt.Errorf("funding accounts: unmatched venue-local positions %s", venueNet)
	}
	return result, nil
}

var _ FundingPositionSnapshotter = (*PositionManager)(nil)
