// Package stocks defines the wire types for the Demat & Exchange domain —
// holdings, live quotes, and the buy/sell order lifecycle. Field names match
// the IDBI sandbox spec doc verbatim (snake_case JSON keys) since the
// frontend has not yet defined a fromJson contract for this domain.
package stocks

import "github.com/yourusername/astra-backend/internal/apitime"

type Holding struct {
	ISIN           string        `json:"isin"`
	TradingSymbol  string        `json:"trading_symbol"`
	Exchange       string        `json:"exchange"`
	Product        string        `json:"product"`
	Quantity       int           `json:"quantity"`
	AveragePrice   float64       `json:"average_price"`
	LastPrice      float64       `json:"last_price"`
	ClosePrice     float64       `json:"close_price"`
	AuthorizedDate *apitime.Time `json:"authorized_date,omitempty"`
}

type OHLC struct {
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

type Quote struct {
	InstrumentToken string       `json:"instrument_token"`
	Exchange        string       `json:"exchange"`
	TradingSymbol   string       `json:"trading_symbol"`
	ISIN            string       `json:"isin"`
	LastPrice       float64      `json:"last_price"`
	OHLC            OHLC         `json:"ohlc"`
	Volume          int64        `json:"volume"`
	LotSize         int          `json:"lot_size"`
	TickSize        float64      `json:"tick_size"`
	Timestamp       apitime.Time `json:"timestamp"`
}

// OrderRequest is the payload for placing or modifying an order. Modify
// requests only need to set the fields being changed; PlaceOrder requires
// the mandatory fields documented per-field below.
type OrderRequest struct {
	Exchange          string   `json:"exchange"`                // mandatory: NSE / BSE
	TradingSymbol     string   `json:"trading_symbol"`          // mandatory
	TransactionType   string   `json:"transaction_type"`        // mandatory: BUY / SELL
	Quantity          int      `json:"quantity"`                // mandatory
	Product           string   `json:"product"`                 // mandatory: CNC / MIS / NRML
	OrderType         string   `json:"order_type"`              // mandatory: MARKET / LIMIT / SL / SL-M
	Price             *float64 `json:"price,omitempty"`         // required for LIMIT/SL
	TriggerPrice      *float64 `json:"trigger_price,omitempty"` // required for SL/SL-M
	DisclosedQuantity int      `json:"disclosed_quantity,omitempty"`
	Validity          string   `json:"validity,omitempty"` // DAY / IOC
}

type Order struct {
	OrderID           string        `json:"order_id"`
	ExchangeOrderID   *string       `json:"exchange_order_id,omitempty"`
	Exchange          string        `json:"exchange"`
	TradingSymbol     string        `json:"trading_symbol"`
	ISIN              string        `json:"isin,omitempty"`
	TransactionType   string        `json:"transaction_type"`
	Quantity          int           `json:"quantity"`
	Product           string        `json:"product"`
	OrderType         string        `json:"order_type"`
	Price             *float64      `json:"price,omitempty"`
	TriggerPrice      *float64      `json:"trigger_price,omitempty"`
	DisclosedQuantity int           `json:"disclosed_quantity"`
	Validity          string        `json:"validity"`
	Status            string        `json:"status"`
	StatusMessage     *string       `json:"status_message,omitempty"`
	FilledQuantity    int           `json:"filled_quantity"`
	PendingQuantity   int           `json:"pending_quantity"`
	CancelledQuantity int           `json:"cancelled_quantity"`
	AveragePrice      *float64      `json:"average_price,omitempty"`
	OrderTimestamp    apitime.Time  `json:"order_timestamp"`
	ExchangeTimestamp *apitime.Time `json:"exchange_timestamp,omitempty"`
}

const (
	StatusOpen      = "OPEN"
	StatusComplete  = "COMPLETE"
	StatusCancelled = "CANCELLED"
	StatusRejected  = "REJECTED"

	TxnBuy  = "BUY"
	TxnSell = "SELL"

	OrderTypeMarket = "MARKET"
	OrderTypeLimit  = "LIMIT"
	OrderTypeSL     = "SL"
	OrderTypeSLM    = "SL-M"
)
