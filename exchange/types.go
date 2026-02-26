package exchange

import (
	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	efee "exchange_sim/fee"
	einstrument "exchange_sim/instrument"
	emarketdata "exchange_sim/marketdata"
	ematching "exchange_sim/matching"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

type Side = etypes.Side
type OrderType = etypes.OrderType
type TimeInForce = etypes.TimeInForce
type Visibility = etypes.Visibility
type OrderStatus = etypes.OrderStatus
type RejectReason = etypes.RejectReason
type Order = etypes.Order
type Limit = etypes.Limit
type RequestType = etypes.RequestType
type QueryType = etypes.QueryType
type Request = etypes.Request
type Response = etypes.Response
type FillNotification = etypes.FillNotification
type OrderRequest = etypes.OrderRequest
type CancelRequest = etypes.CancelRequest
type QueryRequest = etypes.QueryRequest
type BalanceSnapshot = etypes.BalanceSnapshot
type AssetBalance = etypes.AssetBalance
type MDType = etypes.MDType
type MarketDataMsg = etypes.MarketDataMsg
type BookSnapshot = etypes.BookSnapshot
type BookDelta = etypes.BookDelta
type Trade = etypes.Trade
type PriceLevel = etypes.PriceLevel
type Subscription = etypes.Subscription
type Execution = etypes.Execution
type Fee = etypes.Fee
type FundingRate = etypes.FundingRate
type OpenInterest = etypes.OpenInterest
type Position = etypes.Position
type MarginCallEvent = etypes.MarginCallEvent
type LiquidationEvent = etypes.LiquidationEvent
type InsuranceFundEvent = etypes.InsuranceFundEvent
type MarginInterestEvent = etypes.MarginInterestEvent
type TransferEvent = etypes.TransferEvent
type BalanceChangeEvent = etypes.BalanceChangeEvent
type BalanceDelta = etypes.BalanceDelta
type BorrowEvent = etypes.BorrowEvent
type RepayEvent = etypes.RepayEvent
type PositionUpdateEvent = etypes.PositionUpdateEvent
type RealizedPnLEvent = etypes.RealizedPnLEvent
type MarkPriceUpdateEvent = etypes.MarkPriceUpdateEvent
type FundingRateUpdateEvent = etypes.FundingRateUpdateEvent
type OpenInterestEvent = etypes.OpenInterestEvent
type FeeRevenueEvent = etypes.FeeRevenueEvent
type MarginMode = etypes.MarginMode
type BorrowingConfig = etypes.BorrowingConfig
type IsolatedPosition = etypes.IsolatedPosition
type PositionDelta = etypes.PositionDelta

const (
	Buy  = etypes.Buy
	Sell = etypes.Sell
)

const (
	Market     = etypes.Market
	LimitOrder = etypes.LimitOrder
)

const (
	GTC = etypes.GTC
	IOC = etypes.IOC
	FOK = etypes.FOK
)

const (
	Normal  = etypes.Normal
	Iceberg = etypes.Iceberg
	Hidden  = etypes.Hidden
)

const (
	Open        = etypes.Open
	PartialFill = etypes.PartialFill
	Filled      = etypes.Filled
	Cancelled   = etypes.Cancelled
	Rejected    = etypes.Rejected
)

const (
	RejectInsufficientBalance = etypes.RejectInsufficientBalance
	RejectInvalidPrice        = etypes.RejectInvalidPrice
	RejectInvalidQty          = etypes.RejectInvalidQty
	RejectUnknownClient       = etypes.RejectUnknownClient
	RejectUnknownInstrument   = etypes.RejectUnknownInstrument
	RejectSelfTrade           = etypes.RejectSelfTrade
	RejectDuplicateOrderID    = etypes.RejectDuplicateOrderID
	RejectOrderNotFound       = etypes.RejectOrderNotFound
	RejectOrderNotOwned       = etypes.RejectOrderNotOwned
	RejectOrderAlreadyFilled  = etypes.RejectOrderAlreadyFilled
	RejectFOKNotFilled        = etypes.RejectFOKNotFilled
)

const (
	ReqPlaceOrder   = etypes.ReqPlaceOrder
	ReqCancelOrder  = etypes.ReqCancelOrder
	ReqQueryBalance = etypes.ReqQueryBalance
	ReqQueryOrders  = etypes.ReqQueryOrders
	ReqSubscribe    = etypes.ReqSubscribe
	ReqUnsubscribe  = etypes.ReqUnsubscribe
)

const (
	QueryBalance = etypes.QueryBalance
	QueryOrders  = etypes.QueryOrders
	QueryOrder   = etypes.QueryOrder
)

const (
	MDSnapshot     = etypes.MDSnapshot
	MDDelta        = etypes.MDDelta
	MDTrade        = etypes.MDTrade
	MDFunding      = etypes.MDFunding
	MDOpenInterest = etypes.MDOpenInterest
)

const (
	CrossMargin    = etypes.CrossMargin
	IsolatedMargin = etypes.IsolatedMargin
)

type Logger = etypes.Logger
type FeeModel = etypes.FeeModel
type Instrument = etypes.Instrument
type PriceOracle = etypes.PriceOracle
type Clock = etypes.Clock
type Ticker = etypes.Ticker
type TickerFactory = etypes.TickerFactory
type MatchingEngine = ematching.MatchingEngine
type MatchResult = ematching.MatchResult
type MarkPriceCalculator = eprice.MarkPriceCalculator
type FundingCalculator = einstrument.FundingCalculator
type Book = ebook.Book
type OrderBook = ebook.OrderBook
type RealClock = eclock.RealClock
type RealTickerFactory = eclock.RealTickerFactory
type SpotInstrument = einstrument.SpotInstrument
type PerpFutures = einstrument.PerpFutures
type SimpleFundingCalc = einstrument.SimpleFundingCalc
type PercentageFee = efee.PercentageFee
type FixedFee = efee.FixedFee
type DefaultMatcher = ematching.DefaultMatcher
type ProRataMatcher = ematching.ProRataMatcher
type MDPublisher = emarketdata.MDPublisher
type LastPriceCalculator = eprice.LastPriceCalculator
type MidPriceCalculator = eprice.MidPriceCalculator
type WeightedMidPriceCalculator = eprice.WeightedMidPriceCalculator

const BPS = efee.BPS

var NewSpotInstrument = einstrument.NewSpotInstrument
var NewPerpFutures = einstrument.NewPerpFutures
var NewMidPriceCalculator = eprice.NewMidPriceCalculator
var NewStaticPriceOracle = eprice.NewStaticPriceOracle
var NewDynamicPriceOracle = eprice.NewDynamicPriceOracle
var NewMidPriceOracle = eprice.NewMidPriceOracle
var NewLastPriceCalculator = eprice.NewLastPriceCalculator
var NewWeightedMidPriceCalculator = eprice.NewWeightedMidPriceCalculator

var NewBook = ebook.NewBook
var GetLimit = ebook.GetLimit
var LinkOrder = ebook.LinkOrder
var UnlinkOrder = ebook.UnlinkOrder
var VisibleQty = ebook.VisibleQty

var GetExecution = ematching.GetExecution
var PutExecution = ematching.PutExecution

var GetMDMsg = emarketdata.GetMDMsg
var PutMDMsg = emarketdata.PutMDMsg

func NewMDPublisher() *MDPublisher { return emarketdata.NewMDPublisher() }

// NewDefaultMatcher injects a real-time clock; callers using simulation time
// should call matching.NewDefaultMatcher(clock) directly.
func NewDefaultMatcher() *DefaultMatcher { return ematching.NewDefaultMatcher(&eclock.RealClock{}) }

// NewProRataMatcher injects a real-time clock; callers using simulation time
// should call matching.NewProRataMatcher(clock) directly.
func NewProRataMatcher() *ProRataMatcher { return ematching.NewProRataMatcher(&eclock.RealClock{}) }
