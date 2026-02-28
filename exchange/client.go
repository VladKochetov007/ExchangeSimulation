package exchange

// Client represents an exchange client's account state.
// All balance and margin accounting is managed internally by the exchange.
// Users cannot cause negative reserved balances through legitimate trading;
// any such occurrence indicates an exchange-side accounting bug.
type Client struct {
	ID                uint64
	Balances          map[string]int64
	Reserved          map[string]int64
	PerpBalances      map[string]int64
	PerpReserved      map[string]int64
	Borrowed          map[string]int64
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
	spotBalances := make([]AssetBalance, 0, len(c.Balances))
	for asset, total := range c.Balances {
		locked := c.Reserved[asset]
		borrowed := c.Borrowed[asset]
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
		perpBalances = append(perpBalances, AssetBalance{
			Asset:    asset,
			Free:     total - locked,
			Locked:   locked,
			NetAsset: total,
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
