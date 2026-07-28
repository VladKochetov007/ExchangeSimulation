package exchange

// Client represents an exchange client's account state.
// All balance and margin accounting is managed internally by the exchange.
// Users cannot cause negative reserved balances through legitimate trading;
// any such occurrence indicates an exchange-side accounting bug.
type Client struct {
	ID           uint64
	Balances     map[string]int64
	Reserved     map[string]int64
	PerpBalances map[string]int64
	PerpReserved map[string]int64
	Borrowed     map[string]int64
	// BorrowedSpot is the portion of Borrowed whose cash was credited to the
	// spot wallet (auto-borrow for a spot order). The split decides which wallet
	// a liability is netted against: perp equity, liquidation estimates, and
	// snapshots must not charge a spot-credited loan to the perp wallet.
	BorrowedSpot      map[string]int64
	OrderIDs          []uint64
	FeePlan           FeeModel
	MarginMode        MarginMode
	IsolatedPositions map[string]*IsolatedPosition
}

func NewClient(id uint64, feePlan FeeModel) *Client {
	return &Client{
		ID:                id,
		Balances:          make(map[string]int64, 8),
		Reserved:          make(map[string]int64, 8),
		PerpBalances:      make(map[string]int64, 4),
		PerpReserved:      make(map[string]int64, 4),
		Borrowed:          make(map[string]int64, 4),
		BorrowedSpot:      make(map[string]int64, 4),
		OrderIDs:          make([]uint64, 0, 16),
		FeePlan:           feePlan,
		MarginMode:        CrossMargin,
		IsolatedPositions: make(map[string]*IsolatedPosition),
	}
}

func (c *Client) GetBalance(asset string) int64 {
	return c.Balances[asset]
}

func (c *Client) GetAvailable(asset string) int64 {
	return c.Balances[asset] - c.Reserved[asset]
}

func (c *Client) GetReserved(asset string) int64 {
	return c.Reserved[asset]
}

func (c *Client) AddBalance(asset string, amount int64) {
	c.Balances[asset] += amount
}

func (c *Client) SubBalance(asset string, amount int64) bool {
	if c.GetAvailable(asset) < amount {
		return false
	}
	c.Balances[asset] -= amount
	return true
}

func (c *Client) Reserve(asset string, amount int64) bool {
	if c.GetAvailable(asset) < amount {
		return false
	}
	c.Reserved[asset] += amount
	return true
}

func (c *Client) Release(asset string, amount int64) {
	c.Reserved[asset] = max(0, c.Reserved[asset]-amount)
}

func (c *Client) PerpAvailable(asset string) int64 {
	return c.PerpBalances[asset] - c.PerpReserved[asset]
}

// BorrowedPerpPortion returns the share of the asset's debt whose cash was
// credited to the perp wallet; BorrowedSpotPortion the share credited to spot.
// Clamped so the two never sum past the outstanding total even after partial
// repayments shrank Borrowed below the recorded spot attribution.
func (c *Client) BorrowedPerpPortion(asset string) int64 {
	if d := c.Borrowed[asset] - c.BorrowedSpot[asset]; d > 0 {
		return d
	}
	return 0
}

func (c *Client) BorrowedSpotPortion(asset string) int64 {
	return min(c.BorrowedSpot[asset], c.Borrowed[asset])
}

func (c *Client) PerpBalance(asset string) int64 {
	return c.PerpBalances[asset]
}

func (c *Client) MutatePerpBalance(asset string, delta int64) {
	c.PerpBalances[asset] += delta
}

func (c *Client) ReservePerp(asset string, amount int64) bool {
	if c.PerpAvailable(asset) < amount {
		return false
	}
	c.PerpReserved[asset] += amount
	return true
}

// ForceReservePerp earmarks margin unconditionally. Used post-trade: the fill
// already happened, so margin is owed even when it pushes available negative;
// the liquidation sweep resolves the shortfall.
func (c *Client) ForceReservePerp(asset string, amount int64) {
	c.PerpReserved[asset] += amount
}

func (c *Client) ReleasePerp(asset string, amount int64) {
	c.PerpReserved[asset] = max(0, c.PerpReserved[asset]-amount)
}

func (c *Client) AddOrder(orderID uint64) {
	c.OrderIDs = append(c.OrderIDs, orderID)
}

func (c *Client) RemoveOrder(orderID uint64) {
	for i, id := range c.OrderIDs {
		if id == orderID {
			c.OrderIDs[i] = c.OrderIDs[len(c.OrderIDs)-1]
			c.OrderIDs = c.OrderIDs[:len(c.OrderIDs)-1]
			return
		}
	}
}

func (c *Client) GetBalanceSnapshot(timestamp int64) *BalanceSnapshot {
	// Each wallet nets only the debt attributed to it: the account-level
	// liability must reduce net worth exactly once, not once per wallet row.
	spotBalances := make([]AssetBalance, 0, len(c.Balances))
	for asset, total := range c.Balances {
		locked := c.Reserved[asset]
		borrowed := c.BorrowedSpotPortion(asset)
		spotBalances = append(spotBalances, AssetBalance{
			Asset:    asset,
			Free:     total - locked,
			Locked:   locked,
			Borrowed: borrowed,
			NetAsset: total - borrowed,
		})
	}

	perpBalances := make([]AssetBalance, 0, len(c.PerpBalances))
	for asset, total := range c.PerpBalances {
		locked := c.PerpReserved[asset]
		borrowed := c.BorrowedPerpPortion(asset)
		perpBalances = append(perpBalances, AssetBalance{
			Asset:    asset,
			Free:     total - locked,
			Locked:   locked,
			Borrowed: borrowed,
			NetAsset: total - borrowed,
		})
	}

	borrowed := make(map[string]int64, len(c.Borrowed))
	for asset, amount := range c.Borrowed {
		if amount > 0 {
			borrowed[asset] = amount
		}
	}

	return &BalanceSnapshot{
		Timestamp:    timestamp,
		ClientID:     c.ID,
		SpotBalances: spotBalances,
		PerpBalances: perpBalances,
		Borrowed:     borrowed,
	}
}
