package stocks

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	"github.com/yourusername/astra-backend/internal/domain/stocks"
)

// instrument is a static reference row for the handful of NSE instruments
// this mock exchange knows how to quote and trade. A real provider would
// resolve this from the exchange/broker instrument master instead.
type instrument struct {
	token     string
	isin      string
	exchange  string
	basePrice float64
	lotSize   int
	tickSize  float64
}

var instruments = map[string]instrument{
	"RELIANCE":   {token: "738561", isin: "INE002A01018", exchange: "NSE", basePrice: 2921.40, lotSize: 1, tickSize: 0.05},
	"TCS":        {token: "2953217", isin: "INE467B01029", exchange: "NSE", basePrice: 4152.30, lotSize: 1, tickSize: 0.05},
	"INFY":       {token: "408065", isin: "INE009A01021", exchange: "NSE", basePrice: 1876.90, lotSize: 1, tickSize: 0.05},
	"HDFCBANK":   {token: "341249", isin: "INE040A01034", exchange: "NSE", basePrice: 1689.55, lotSize: 1, tickSize: 0.05},
	"ICICIBANK":  {token: "1270529", isin: "INE090A01021", exchange: "NSE", basePrice: 1234.75, lotSize: 1, tickSize: 0.05},
	"TATAMOTORS": {token: "884737", isin: "INE155A01022", exchange: "NSE", basePrice: 967.20, lotSize: 1, tickSize: 0.05},
}

func lookupInstrument(symbol string) (instrument, bool) {
	inst, ok := instruments[strings.ToUpper(symbol)]
	return inst, ok
}

// livePrice derives a small, deterministic-within-a-30s-window jitter around
// an instrument's base price, so repeated quote calls look "live" without
// needing a background price-feed goroutine.
func livePrice(base float64, symbol string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(symbol))
	bucket := time.Now().Unix() / 30
	r := rand.New(rand.NewSource(int64(h.Sum64()) + bucket)) //nolint:gosec // mock market data, not security-sensitive
	pctMove := (r.Float64() - 0.5) * 0.01
	return round2(base * (1 + pctMove))
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func newOrderID() string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, so read helpers can
// run either inside a transaction (for a locked read) or standalone.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// MockProvider is the stateful mock implementation of Provider: it persists
// realistic per-user holdings and orders to Postgres and simulates order
// fills against the static instrument table above.
type MockProvider struct {
	pool *pgxpool.Pool
}

func NewMockProvider(pool *pgxpool.Pool) *MockProvider {
	return &MockProvider{pool: pool}
}

func (p *MockProvider) GetHoldings(ctx context.Context, userID uuid.UUID) ([]stocks.Holding, error) {
	if err := p.seedHoldings(ctx, userID); err != nil {
		return nil, err
	}

	rows, err := p.pool.Query(ctx, `
		SELECT isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date
		FROM demat_holdings
		WHERE user_id = $1 AND quantity > 0
		ORDER BY trading_symbol
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query holdings: %w", err)
	}
	defer rows.Close()

	holdings := make([]stocks.Holding, 0)
	for rows.Next() {
		var h stocks.Holding
		var authorizedDate *time.Time
		if err := rows.Scan(&h.ISIN, &h.TradingSymbol, &h.Exchange, &h.Product, &h.Quantity, &h.AveragePrice, &h.LastPrice, &h.ClosePrice, &authorizedDate); err != nil {
			return nil, fmt.Errorf("scan holding: %w", err)
		}
		if authorizedDate != nil {
			at := apitime.New(*authorizedDate)
			h.AuthorizedDate = &at
		}
		holdings = append(holdings, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate holdings: %w", err)
	}
	return holdings, nil
}

func (p *MockProvider) seedHoldings(ctx context.Context, userID uuid.UUID) error {
	seed := []struct {
		symbol   string
		qty      int
		avgPrice float64
	}{
		{"RELIANCE", 42, 2380.55},
		{"TCS", 10, 3890.00},
		{"HDFCBANK", 60, 1550.20},
	}
	for _, s := range seed {
		inst, ok := lookupInstrument(s.symbol)
		if !ok {
			continue
		}
		if _, err := p.pool.Exec(ctx, `
			INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date)
			VALUES ($1, $2, $3, $4, 'CNC', $5, $6, $7, $7, CURRENT_DATE - INTERVAL '400 days')
			ON CONFLICT (user_id, isin, product) DO NOTHING
		`, userID, inst.isin, s.symbol, inst.exchange, s.qty, s.avgPrice, inst.basePrice); err != nil {
			return fmt.Errorf("seed holding %s: %w", s.symbol, err)
		}
	}
	return nil
}

func (p *MockProvider) GetQuote(ctx context.Context, exchange, tradingSymbol string) (*stocks.Quote, error) {
	_ = ctx
	inst, ok := lookupInstrument(tradingSymbol)
	if !ok {
		return nil, fmt.Errorf("instrument %s not found: %w", tradingSymbol, apiresponse.ErrNotFound)
	}
	if exchange != "" && !strings.EqualFold(exchange, inst.exchange) {
		return nil, fmt.Errorf("instrument %s is not listed on %s: %w", tradingSymbol, exchange, apiresponse.ErrNotFound)
	}

	last := livePrice(inst.basePrice, tradingSymbol)
	open := round2(inst.basePrice * 0.995)
	high := round2(math.Max(last, open) * 1.004)
	low := round2(math.Min(last, open) * 0.996)

	h := fnv.New64a()
	_, _ = h.Write([]byte(tradingSymbol))
	volume := int64(1_000_000 + h.Sum64()%3_000_000)

	return &stocks.Quote{
		InstrumentToken: inst.token,
		Exchange:        inst.exchange,
		TradingSymbol:   strings.ToUpper(tradingSymbol),
		ISIN:            inst.isin,
		LastPrice:       last,
		OHLC:            stocks.OHLC{Open: open, High: high, Low: low, Close: inst.basePrice},
		Volume:          volume,
		LotSize:         inst.lotSize,
		TickSize:        inst.tickSize,
		Timestamp:       apitime.New(time.Now().UTC()),
	}, nil
}

func validateOrderRequest(req stocks.OrderRequest) error {
	if req.Exchange == "" || req.TradingSymbol == "" {
		return fmt.Errorf("exchange and trading_symbol are required: %w", apiresponse.ErrValidation)
	}
	if req.TransactionType != stocks.TxnBuy && req.TransactionType != stocks.TxnSell {
		return fmt.Errorf("transaction_type must be BUY or SELL: %w", apiresponse.ErrValidation)
	}
	if req.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive: %w", apiresponse.ErrValidation)
	}
	if req.Product == "" {
		return fmt.Errorf("product is required: %w", apiresponse.ErrValidation)
	}
	switch req.OrderType {
	case stocks.OrderTypeMarket:
	case stocks.OrderTypeLimit:
		if req.Price == nil || *req.Price <= 0 {
			return fmt.Errorf("price is required for LIMIT orders: %w", apiresponse.ErrValidation)
		}
	case stocks.OrderTypeSL:
		if req.Price == nil || *req.Price <= 0 || req.TriggerPrice == nil || *req.TriggerPrice <= 0 {
			return fmt.Errorf("price and trigger_price are required for SL orders: %w", apiresponse.ErrValidation)
		}
	case stocks.OrderTypeSLM:
		if req.TriggerPrice == nil || *req.TriggerPrice <= 0 {
			return fmt.Errorf("trigger_price is required for SL-M orders: %w", apiresponse.ErrValidation)
		}
	default:
		return fmt.Errorf("order_type must be one of MARKET, LIMIT, SL, SL-M: %w", apiresponse.ErrValidation)
	}
	return nil
}

// decideFill reports whether the mock exchange fills this order right now,
// and at what price. MARKET orders always fill instantly; LIMIT orders fill
// only if the current quote already satisfies the limit. SL/SL-M orders have
// no live tick-triggered matching engine in this mock and stay OPEN until
// the user modifies or cancels them — a deliberate, documented limitation
// rather than a fake trigger simulation.
func decideFill(order *stocks.Order, quote float64) (float64, bool) {
	switch order.OrderType {
	case stocks.OrderTypeMarket:
		return quote, true
	case stocks.OrderTypeLimit:
		limit := *order.Price
		if order.TransactionType == stocks.TxnBuy && quote <= limit {
			return quote, true
		}
		if order.TransactionType == stocks.TxnSell && quote >= limit {
			return quote, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// attemptFill mutates order in place to reflect a fill (or leaves it OPEN),
// applying the corresponding holdings change inside the caller's transaction.
func attemptFill(ctx context.Context, tx pgx.Tx, userID uuid.UUID, inst instrument, quote float64, order *stocks.Order) error {
	fillPrice, shouldFill := decideFill(order, quote)
	if !shouldFill {
		return nil
	}

	if order.TransactionType == stocks.TxnSell {
		var heldQty int
		err := tx.QueryRow(ctx, `
			SELECT quantity FROM demat_holdings
			WHERE user_id = $1 AND isin = $2 AND product = $3
			FOR UPDATE
		`, userID, inst.isin, order.Product).Scan(&heldQty)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("no holding to sell for %s: %w", order.TradingSymbol, apiresponse.ErrValidation)
			}
			return fmt.Errorf("lock holding for sell: %w", err)
		}
		if heldQty < order.Quantity {
			return fmt.Errorf("insufficient holding quantity for %s: have %d, requested %d: %w", order.TradingSymbol, heldQty, order.Quantity, apiresponse.ErrValidation)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE demat_holdings SET quantity = quantity - $1, updated_at = now()
			WHERE user_id = $2 AND isin = $3 AND product = $4
		`, order.Quantity, userID, inst.isin, order.Product); err != nil {
			return fmt.Errorf("update holding on sell: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $7)
			ON CONFLICT (user_id, isin, product) DO UPDATE SET
				average_price = ((demat_holdings.quantity * demat_holdings.average_price) + ($6 * $7)) / NULLIF(demat_holdings.quantity + $6, 0),
				quantity = demat_holdings.quantity + $6,
				last_price = $7,
				updated_at = now()
		`, userID, inst.isin, order.TradingSymbol, inst.exchange, order.Product, order.Quantity, fillPrice); err != nil {
			return fmt.Errorf("upsert holding on buy: %w", err)
		}
	}

	order.Status = stocks.StatusComplete
	order.FilledQuantity = order.Quantity
	order.PendingQuantity = 0
	avg := fillPrice
	order.AveragePrice = &avg
	now := time.Now().UTC()
	et := apitime.New(now)
	order.ExchangeTimestamp = &et
	exchOrderID := "EXC-" + order.OrderID
	order.ExchangeOrderID = &exchOrderID
	return nil
}

func (p *MockProvider) PlaceOrder(ctx context.Context, userID uuid.UUID, req stocks.OrderRequest) (*stocks.Order, error) {
	if err := validateOrderRequest(req); err != nil {
		return nil, err
	}
	inst, ok := lookupInstrument(req.TradingSymbol)
	if !ok {
		return nil, fmt.Errorf("instrument %s not found: %w", req.TradingSymbol, apiresponse.ErrNotFound)
	}
	if req.Validity == "" {
		req.Validity = "DAY"
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin place order tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	order := &stocks.Order{
		OrderID:           newOrderID(),
		Exchange:          inst.exchange,
		TradingSymbol:     strings.ToUpper(req.TradingSymbol),
		ISIN:              inst.isin,
		TransactionType:   req.TransactionType,
		Quantity:          req.Quantity,
		Product:           req.Product,
		OrderType:         req.OrderType,
		Price:             req.Price,
		TriggerPrice:      req.TriggerPrice,
		DisclosedQuantity: req.DisclosedQuantity,
		Validity:          req.Validity,
		Status:            stocks.StatusOpen,
		PendingQuantity:   req.Quantity,
		OrderTimestamp:    apitime.New(time.Now().UTC()),
	}

	quote := livePrice(inst.basePrice, req.TradingSymbol)
	if err := attemptFill(ctx, tx, userID, inst, quote, order); err != nil {
		return nil, err
	}
	if err := insertOrder(ctx, tx, userID, order); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit place order: %w", err)
	}
	return order, nil
}

func (p *MockProvider) ModifyOrder(ctx context.Context, userID uuid.UUID, orderID string, req stocks.OrderRequest) (*stocks.Order, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin modify order tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	order, err := loadOrder(ctx, tx, userID, orderID, true)
	if err != nil {
		return nil, err
	}
	if order.Status != stocks.StatusOpen {
		return nil, fmt.Errorf("order %s is %s and cannot be modified: %w", orderID, order.Status, apiresponse.ErrConflict)
	}
	inst, ok := lookupInstrument(order.TradingSymbol)
	if !ok {
		return nil, fmt.Errorf("instrument %s no longer tradable: %w", order.TradingSymbol, apiresponse.ErrInternal)
	}

	if req.Quantity > 0 {
		order.Quantity = req.Quantity
		order.PendingQuantity = req.Quantity
	}
	if req.OrderType != "" {
		order.OrderType = req.OrderType
	}
	if req.Price != nil {
		order.Price = req.Price
	}
	if req.TriggerPrice != nil {
		order.TriggerPrice = req.TriggerPrice
	}
	if req.Validity != "" {
		order.Validity = req.Validity
	}

	merged := stocks.OrderRequest{
		Exchange: order.Exchange, TradingSymbol: order.TradingSymbol, TransactionType: order.TransactionType,
		Quantity: order.Quantity, Product: order.Product, OrderType: order.OrderType,
		Price: order.Price, TriggerPrice: order.TriggerPrice,
	}
	if err := validateOrderRequest(merged); err != nil {
		return nil, err
	}

	quote := livePrice(inst.basePrice, order.TradingSymbol)
	if err := attemptFill(ctx, tx, userID, inst, quote, order); err != nil {
		return nil, err
	}
	if err := updateOrder(ctx, tx, userID, order); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit modify order: %w", err)
	}
	return order, nil
}

func (p *MockProvider) CancelOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocks.Order, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin cancel order tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	order, err := loadOrder(ctx, tx, userID, orderID, true)
	if err != nil {
		return nil, err
	}
	if order.Status != stocks.StatusOpen {
		return nil, fmt.Errorf("order %s is %s and cannot be cancelled: %w", orderID, order.Status, apiresponse.ErrConflict)
	}
	order.Status = stocks.StatusCancelled
	order.CancelledQuantity = order.PendingQuantity
	order.PendingQuantity = 0

	if err := updateOrder(ctx, tx, userID, order); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit cancel order: %w", err)
	}
	return order, nil
}

func (p *MockProvider) GetOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocks.Order, error) {
	return loadOrder(ctx, p.pool, userID, orderID, false)
}

const orderColumns = `order_id, exchange_order_id, exchange, trading_symbol, isin, transaction_type, quantity,
	product, order_type, price, trigger_price, disclosed_quantity, validity, status, status_message,
	filled_quantity, pending_quantity, cancelled_quantity, average_price, order_timestamp, exchange_timestamp`

func scanOrder(row interface{ Scan(dest ...any) error }) (stocks.Order, error) {
	var o stocks.Order
	var isin *string
	var exchangeTimestamp *time.Time
	err := row.Scan(
		&o.OrderID, &o.ExchangeOrderID, &o.Exchange, &o.TradingSymbol, &isin, &o.TransactionType, &o.Quantity,
		&o.Product, &o.OrderType, &o.Price, &o.TriggerPrice, &o.DisclosedQuantity, &o.Validity, &o.Status, &o.StatusMessage,
		&o.FilledQuantity, &o.PendingQuantity, &o.CancelledQuantity, &o.AveragePrice, &o.OrderTimestamp, &exchangeTimestamp,
	)
	if err != nil {
		return stocks.Order{}, fmt.Errorf("scan order: %w", err)
	}
	if isin != nil {
		o.ISIN = *isin
	}
	o.ExchangeTimestamp = apitime.NewPtr(exchangeTimestamp)
	return o, nil
}

func (p *MockProvider) ListOrders(ctx context.Context, userID uuid.UUID, statusFilter string) ([]stocks.Order, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT `+orderColumns+`
		FROM stock_orders WHERE user_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY order_timestamp DESC
	`, userID, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	orders := make([]stocks.Order, 0)
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	return orders, nil
}

func loadOrder(ctx context.Context, q querier, userID uuid.UUID, orderID string, forUpdate bool) (*stocks.Order, error) {
	sql := `SELECT ` + orderColumns + ` FROM stock_orders WHERE order_id = $1 AND user_id = $2`
	if forUpdate {
		sql += " FOR UPDATE"
	}

	o, err := scanOrder(q.QueryRow(ctx, sql, orderID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("order %s not found: %w", orderID, apiresponse.ErrNotFound)
		}
		return nil, err
	}
	return &o, nil
}

func insertOrder(ctx context.Context, tx pgx.Tx, userID uuid.UUID, order *stocks.Order) error {
	var isin *string
	if order.ISIN != "" {
		isin = &order.ISIN
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO stock_orders (
			order_id, exchange_order_id, user_id, exchange, trading_symbol, isin,
			transaction_type, quantity, product, order_type, price, trigger_price,
			disclosed_quantity, validity, status, status_message,
			filled_quantity, pending_quantity, cancelled_quantity, average_price,
			order_timestamp, exchange_timestamp
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	`, order.OrderID, order.ExchangeOrderID, userID, order.Exchange, order.TradingSymbol, isin,
		order.TransactionType, order.Quantity, order.Product, order.OrderType, order.Price, order.TriggerPrice,
		order.DisclosedQuantity, order.Validity, order.Status, order.StatusMessage,
		order.FilledQuantity, order.PendingQuantity, order.CancelledQuantity, order.AveragePrice,
		order.OrderTimestamp, apitime.ToTimePtr(order.ExchangeTimestamp))
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	return nil
}

func updateOrder(ctx context.Context, tx pgx.Tx, userID uuid.UUID, order *stocks.Order) error {
	_, err := tx.Exec(ctx, `
		UPDATE stock_orders SET
			exchange_order_id = $1, quantity = $2, order_type = $3, price = $4, trigger_price = $5,
			validity = $6, status = $7, filled_quantity = $8, pending_quantity = $9, cancelled_quantity = $10,
			average_price = $11, exchange_timestamp = $12, updated_at = now()
		WHERE order_id = $13 AND user_id = $14
	`, order.ExchangeOrderID, order.Quantity, order.OrderType, order.Price, order.TriggerPrice,
		order.Validity, order.Status, order.FilledQuantity, order.PendingQuantity, order.CancelledQuantity,
		order.AveragePrice, apitime.ToTimePtr(order.ExchangeTimestamp), order.OrderID, userID)
	if err != nil {
		return fmt.Errorf("update order: %w", err)
	}
	return nil
}
