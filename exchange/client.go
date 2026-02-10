package exchange

type Client struct {
	ID                uint64
	Balances          map[string]int64
	Reserved          map[string]int64
	PerpBalances      map[string]int64
	PerpReserved      map[string]int64
	Borrowed          map[string]int64
	OrderIDs          []uint64
	FeePlan           FeeModel
	VIPLevel          int
	MakerVolume       int64
	TakerVolume       int64
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
	c.Reserved[asset] -= amount
}

func (c *Client) PerpAvailable(asset string) int64 {
	return c.PerpBalances[asset] - c.PerpReserved[asset]
}

func (c *Client) ReservePerp(asset string, amount int64) bool {
	if c.PerpAvailable(asset) < amount {
		return false
	}
	c.PerpReserved[asset] += amount
	return true
}

func (c *Client) ReleasePerp(asset string, amount int64) {
	c.PerpReserved[asset] -= amount
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
	balances := make([]AssetBalance, 0, len(c.Balances)+len(c.PerpBalances))
	for asset, total := range c.Balances {
		reserved := c.Reserved[asset]
		balances = append(balances, AssetBalance{
			Asset:     asset,
			Total:     total,
			Available: total - reserved,
			Reserved:  reserved,
		})
	}
	return &BalanceSnapshot{
		Timestamp: timestamp,
		Balances:  balances,
	}
}
