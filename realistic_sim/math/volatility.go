package math

type RollingVolatility struct {
	returns   *CircularBuffer
	lastPrice int64
	scale     int64
}

func NewRollingVolatility(windowSize int, scale int64) *RollingVolatility {
	return &RollingVolatility{
		returns: NewCircularBuffer(windowSize),
		scale:   scale,
	}
}

func (rv *RollingVolatility) AddPrice(price int64) {
	if rv.lastPrice != 0 {
		ret := LogReturn(price, rv.lastPrice, rv.scale)
		rv.returns.Add(ret)
	}
	rv.lastPrice = price
}

func (rv *RollingVolatility) Volatility() int64 {
	mean := rv.returns.SMA()
	return rv.returns.StdDev(mean)
}

func (rv *RollingVolatility) Size() int {
	return rv.returns.Count()
}
