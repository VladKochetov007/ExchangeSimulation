package exchange

// shims.go bridges unexported names used by package-level tests to their
// new homes in sub-packages after the package split.

import (
	ebook "exchange_sim/exchange/book"
	emarketdata "exchange_sim/exchange/marketdata"
	ematching "exchange_sim/exchange/matching"
)

func newBook(side Side) *Book          { return ebook.NewBook(side) }
func linkOrder(limit *Limit, o *Order) { ebook.LinkOrder(limit, o) }
func unlinkOrder(o *Order)             { ebook.UnlinkOrder(o) }
func visibleQty(limit *Limit) int64    { return ebook.VisibleQty(limit) }
func putExecution(e *Execution)        { ematching.PutExecution(e) }
func putMDMsg(m *MarketDataMsg)        { emarketdata.PutMDMsg(m) }

func getExecution() *Execution    { return ematching.GetExecution() }
func getMDMsg() *MarketDataMsg    { return emarketdata.GetMDMsg() }
func getLimit(price int64) *Limit { return ebook.GetLimit(price) }
