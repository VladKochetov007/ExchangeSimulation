package exchange

import (
	ebook "exchange_sim/exchange/book"
	ecircuit "exchange_sim/exchange/circuit_breaker"
	eclock "exchange_sim/exchange/clock"
	efee "exchange_sim/exchange/fee"
	einstrument "exchange_sim/exchange/instrument"
	emarketdata "exchange_sim/exchange/marketdata"
	ematching "exchange_sim/exchange/matching"
	eprice "exchange_sim/exchange/price"
	etypes "exchange_sim/exchange/types"
)

// Value types
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

// Enumerations — const blocks must be redeclared; type alias makes them compatible.
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

// Interfaces
type Logger = etypes.Logger
type FeeModel = etypes.FeeModel
type Instrument = etypes.Instrument
type PriceOracle = etypes.PriceOracle
type Clock = etypes.Clock
type Ticker = etypes.Ticker
type TickerFactory = etypes.TickerFactory
type MatchingEngine = ematching.MatchingEngine
type MarkPriceCalculator = eprice.MarkPriceCalculator
type FundingCalculator = einstrument.FundingCalculator
type CircuitBreaker = ecircuit.CircuitBreaker
type HaltEvaluator = ecircuit.HaltEvaluator

// Book types
type Book = ebook.Book
type OrderBook = ebook.OrderBook

// Concrete types
type RealClock = eclock.RealClock
type RealTickerFactory = eclock.RealTickerFactory
type SpotInstrument = einstrument.SpotInstrument
type PerpFutures = einstrument.PerpFutures
type SimpleFundingCalc = einstrument.SimpleFundingCalc
type PercentageFee = efee.PercentageFee
type FixedFee = efee.FixedFee
type MatchResult = ematching.MatchResult
type DefaultMatcher = ematching.DefaultMatcher
type ProRataMatcher = ematching.ProRataMatcher
type MDPublisher = emarketdata.MDPublisher
type LastPriceCalculator = eprice.LastPriceCalculator
type MidPriceCalculator = eprice.MidPriceCalculator
type WeightedMidPriceCalculator = eprice.WeightedMidPriceCalculator
type MedianMarkPrice = eprice.MedianMarkPrice
type EMAMarkPrice = eprice.EMAMarkPrice
type ClampedEMAMarkPrice = eprice.ClampedEMAMarkPrice
type TWAPMarkPrice = eprice.TWAPMarkPrice
type StaticPriceOracle = eprice.StaticPriceOracle
type DynamicPriceOracle = eprice.DynamicPriceOracle
type MidPriceOracle = eprice.MidPriceOracle
type CBAction = ecircuit.CBAction
type CBResult = ecircuit.CBResult
type BreakerTier = ecircuit.BreakerTier
type PercentBandCircuitBreaker = ecircuit.PercentBandCircuitBreaker
type AsymmetricBandCircuitBreaker = ecircuit.AsymmetricBandCircuitBreaker
type TieredCircuitBreaker = ecircuit.TieredCircuitBreaker
type TieredHaltEvaluator = ecircuit.TieredHaltEvaluator
type CompositeCircuitBreaker = ecircuit.CompositeCircuitBreaker
type CircuitBreakerMatcher = ecircuit.CircuitBreakerMatcher

const BPS = efee.BPS

const (
	CBAllow  = ecircuit.CBAllow
	CBReject = ecircuit.CBReject
	CBHalt   = ecircuit.CBHalt
)

// Constructor functions
var NewSpotInstrument = einstrument.NewSpotInstrument
var NewPerpFutures = einstrument.NewPerpFutures
var NewStaticPriceOracle = eprice.NewStaticPriceOracle
var NewDynamicPriceOracle = eprice.NewDynamicPriceOracle
var NewLastPriceCalculator = eprice.NewLastPriceCalculator
var NewMidPriceCalculator = eprice.NewMidPriceCalculator
var NewWeightedMidPriceCalculator = eprice.NewWeightedMidPriceCalculator
var NewMedianMarkPrice = eprice.NewMedianMarkPrice
var NewEMAMarkPrice = eprice.NewEMAMarkPrice
var NewClampedEMAMarkPrice = eprice.NewClampedEMAMarkPrice
var NewTWAPMarkPrice = eprice.NewTWAPMarkPrice
var NewTieredCircuitBreaker = ecircuit.NewTieredCircuitBreaker
var NewTieredHaltEvaluator = ecircuit.NewTieredHaltEvaluator

func NewMDPublisher() *MDPublisher { return emarketdata.NewMDPublisher() }

// NewDefaultMatcher injects a real-time clock; callers using simulation time
// should call matching.NewDefaultMatcher(clock) directly.
func NewDefaultMatcher() *DefaultMatcher {
	return ematching.NewDefaultMatcher(&eclock.RealClock{})
}

func NewProRataMatcher() *ProRataMatcher {
	return ematching.NewProRataMatcher(&eclock.RealClock{})
}

func NewMidPriceOracle(provider eprice.BookProvider) *MidPriceOracle {
	return eprice.NewMidPriceOracle(provider)
}

func NewCircuitBreakerMatcher(
	inner ematching.MatchingEngine,
	breaker ecircuit.CircuitBreaker,
	books map[string]*ebook.OrderBook,
	clock etypes.Clock,
) *CircuitBreakerMatcher {
	return ecircuit.NewCircuitBreakerMatcher(inner, breaker, books, clock)
}
