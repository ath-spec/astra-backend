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
	token       string
	isin        string
	exchange    string
	basePrice   float64
	lotSize     int
	tickSize    float64
	companyName string
	sector      string
}

var instruments = map[string]instrument{
	"RELIANCE":   {token: "738561", isin: "INE002A01018", exchange: "NSE", basePrice: 2921.40, lotSize: 1, tickSize: 0.05, companyName: "Reliance Industries", sector: "Energy & Conglomerate"},
	"TCS":        {token: "2953217", isin: "INE467B01029", exchange: "NSE", basePrice: 4152.30, lotSize: 1, tickSize: 0.05, companyName: "Tata Consultancy Services", sector: "Information Technology"},
	"INFY":       {token: "408065", isin: "INE009A01021", exchange: "NSE", basePrice: 1876.90, lotSize: 1, tickSize: 0.05, companyName: "Infosys", sector: "Information Technology"},
	"HDFCBANK":   {token: "341249", isin: "INE040A01034", exchange: "NSE", basePrice: 1689.55, lotSize: 1, tickSize: 0.05, companyName: "HDFC Bank", sector: "Financial Services"},
	"ICICIBANK":  {token: "1270529", isin: "INE090A01021", exchange: "NSE", basePrice: 1234.75, lotSize: 1, tickSize: 0.05, companyName: "ICICI Bank", sector: "Financial Services"},
	"TATAMOTORS": {token: "884737", isin: "INE155A01022", exchange: "NSE", basePrice: 967.20, lotSize: 1, tickSize: 0.05, companyName: "Tata Motors", sector: "Automobiles"},
	// These three are the only symbols the demo archetype seeding in
	// user_repo.go actually writes into demat_holdings — without entries
	// here, GetQuote/GetProfile 404'd ("instrument not found") for every
	// real seeded stock holding a user could ever have, which is the root
	// cause of the /stocks/profile 404s seen in the app.
	"MAZDOCK":    {token: "533280", isin: "INE249Z01012", exchange: "NSE", basePrice: 2305.00, lotSize: 1, tickSize: 0.05, companyName: "Mazagon Dock Shipbuilders", sector: "Defence & Shipbuilding"},
	"COCHINSHIP": {token: "540678", isin: "INE704P01017", exchange: "NSE", basePrice: 1480.00, lotSize: 1, tickSize: 0.05, companyName: "Cochin Shipyard", sector: "Defence & Shipbuilding"},
	"MSTCLTD":    {token: "542597", isin: "INE255X01014", exchange: "NSE", basePrice: 730.00, lotSize: 1, tickSize: 0.05, companyName: "MSTC Limited", sector: "Trading & Government Services"},
}

// sectorProfile holds the per-sector narrative building blocks GetProfile
// varies content by, so two stocks in different sectors never read like the
// same canned paragraph with the name swapped.
type sectorProfile struct {
	descriptionTemplate string // %s is filled with company name
	primaryRole         string
	secondaryRole       string
	strengths           []string
	tradeOffs           []string
	whyGetFund          []string
	suitableFor         []string
	avoidIf             []string
	impactText          string
}

var sectorProfiles = map[string]sectorProfile{
	"Financial Services": {
		descriptionTemplate: "%s is a leading financial institution offering retail banking, corporate banking, and wealth management services, with a large branch and digital footprint across India.",
		primaryRole:         "Core Portfolio Builder",
		secondaryRole:       "Dividend Yield",
		strengths:           []string{"Large, sticky deposit base", "Strong capital adequacy", "Diversified loan book"},
		tradeOffs:           []string{"Sensitive to interest rate cycles", "Regulatory and asset-quality risk"},
		whyGetFund:          []string{"Provides stability to your equity allocation", "Consistent compounder in the financial sector"},
		suitableFor:         []string{"Long-term wealth creation", "Core holding in large-cap financials"},
		avoidIf:             []string{"You already have heavy exposure to banking stocks"},
		impactText:          "Adding this stock increases exposure to India's financial sector, a core driver of the broader economy.",
	},
	"Information Technology": {
		descriptionTemplate: "%s is a major IT services company providing consulting, digital transformation, and outsourcing services to global enterprise clients.",
		primaryRole:         "Growth & Export Play",
		secondaryRole:       "Dollar Revenue Hedge",
		strengths:           []string{"High-margin, asset-light business", "Strong dollar-denominated revenue", "Large deal pipeline"},
		tradeOffs:           []string{"Sensitive to US/Europe IT spending cycles", "Currency and visa-policy risk"},
		whyGetFund:          []string{"Adds export-oriented, dollar-earning exposure to your portfolio", "Historically resilient margins across cycles"},
		suitableFor:         []string{"Diversifying away from domestic-only businesses", "Long-term growth allocation"},
		avoidIf:             []string{"You need short-term stability during a global tech slowdown"},
		impactText:          "Adding this stock increases your portfolio's exposure to global IT services demand.",
	},
	"Energy & Conglomerate": {
		descriptionTemplate: "%s is a diversified conglomerate with interests spanning energy, retail, and digital services, among India's largest companies by market capitalization.",
		primaryRole:         "Core Portfolio Builder",
		secondaryRole:       "Diversification Anchor",
		strengths:           []string{"Diversified revenue across sectors", "Scale advantages", "Strong balance sheet"},
		tradeOffs:           []string{"Complex conglomerate structure", "Capital-intensive expansion plans"},
		whyGetFund:          []string{"Gives broad exposure across energy, retail and digital in a single stock", "Large-cap stability with growth optionality"},
		suitableFor:         []string{"Core, long-term large-cap holding", "Investors wanting diversified sector exposure"},
		avoidIf:             []string{"You want a pure-play bet on a single sector"},
		impactText:          "Adding this stock increases diversification across energy, retail, and digital businesses.",
	},
	"Automobiles": {
		descriptionTemplate: "%s designs and manufactures commercial and passenger vehicles, with a growing footprint in electric mobility.",
		primaryRole:         "Cyclical Growth Play",
		secondaryRole:       "EV Transition Exposure",
		strengths:           []string{"Strong domestic market share", "Growing EV portfolio", "Improving margins"},
		tradeOffs:           []string{"Cyclical demand tied to the broader economy", "Input cost (commodity) sensitivity"},
		whyGetFund:          []string{"Adds exposure to India's auto and EV transition story", "Benefits from rising discretionary spending"},
		suitableFor:         []string{"Cyclical/growth allocation", "Investors betting on EV adoption"},
		avoidIf:             []string{"You are looking for a defensive, low-volatility holding"},
		impactText:          "Adding this stock increases your portfolio's exposure to the automobile and EV transition cycle.",
	},
	"Defence & Shipbuilding": {
		descriptionTemplate: "%s is a defence-sector shipbuilder engaged in the construction and repair of naval vessels and commercial ships, benefiting from India's defence indigenization push.",
		primaryRole:         "Thematic Growth Play",
		secondaryRole:       "Order-Book Visibility",
		strengths:           []string{"Strong order book from government contracts", "Beneficiary of defence indigenization policy", "High entry barriers"},
		tradeOffs:           []string{"Revenue concentrated in government contracts", "Execution and project-timeline risk"},
		whyGetFund:          []string{"Gives thematic exposure to India's defence and shipbuilding push", "Long revenue visibility from order backlogs"},
		suitableFor:         []string{"Thematic/satellite allocation", "Investors bullish on defence indigenization"},
		avoidIf:             []string{"You want low government-policy-dependency in your holdings"},
		impactText:          "Adding this stock increases your portfolio's exposure to the defence and shipbuilding theme.",
	},
	"Trading & Government Services": {
		descriptionTemplate: "%s operates as a government-linked trading and e-commerce services company, facilitating auctions and trade of metal scrap, e-waste, and other commodities.",
		primaryRole:         "Satellite / Tactical Holding",
		secondaryRole:       "Government-Policy Beneficiary",
		strengths:           []string{"Government-backed business model", "Low capital intensity", "Niche market position"},
		tradeOffs:           []string{"Revenue tied to commodity/scrap trading cycles", "Limited pricing power"},
		whyGetFund:          []string{"Adds a niche, government-linked trading business to your portfolio", "Low capital-intensity model"},
		suitableFor:         []string{"Small satellite allocation", "Investors seeking niche PSU exposure"},
		avoidIf:             []string{"You want a core, high-conviction long-term holding"},
		impactText:          "Adding this stock adds a niche, government-linked trading exposure to your portfolio.",
	},
}

// defaultSectorProfile covers any symbol whose sector isn't in
// sectorProfiles above, so an unrecognized instrument still gets sensible,
// non-empty content instead of an empty struct.
var defaultSectorProfile = sectorProfile{
	descriptionTemplate: "%s is a publicly listed company on the Indian stock exchanges.",
	primaryRole:         "Satellite Holding",
	secondaryRole:       "Diversification",
	strengths:           []string{"Listed on a major exchange", "Part of a diversified portfolio"},
	tradeOffs:           []string{"Limited company-specific data available"},
	whyGetFund:          []string{"Adds diversification to your equity holdings"},
	suitableFor:         []string{"Diversified equity allocation"},
	avoidIf:             []string{"You prefer only well-covered, large-cap names"},
	impactText:          "Adding this stock changes your sector diversification.",
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

func (p *MockProvider) GetProfile(ctx context.Context, exchange, tradingSymbol string) (*stocks.StockProfile, error) {
	quote, err := p.GetQuote(ctx, exchange, tradingSymbol)
	if err != nil {
		return nil, err
	}
	// GetQuote already validated the symbol via lookupInstrument, so this
	// is guaranteed to be present.
	inst, _ := lookupInstrument(tradingSymbol)

	companyName := inst.companyName
	if companyName == "" {
		companyName = fmt.Sprintf("%s Limited", quote.TradingSymbol)
	}
	sector := inst.sector
	if sector == "" {
		sector = "Diversified"
	}
	sp, ok := sectorProfiles[sector]
	if !ok {
		sp = defaultSectorProfile
	}

	// Generate some fake historical chart points based on the current price
	var points []stocks.ChartPoint
	now := time.Now().Unix()
	basePrice := quote.LastPrice
	for i := 180; i >= 0; i-- {
		// Mock a semi-random walk backwards
		dayPrice := basePrice * (1.0 + (float64(180-i-90)/1000.0)) // slight curve
		points = append(points, stocks.ChartPoint{
			Timestamp: now - int64(i*86400),
			Price:     dayPrice,
		})
	}

	// Deterministic-per-symbol fundamentals/shareholding, same hash pattern
	// used elsewhere in the mock providers (fnv over the symbol) — every
	// stock gets its own plausible-but-fixed numbers instead of the exact
	// same PE/PB/ROE/shareholding split for every single company.
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToUpper(tradingSymbol)))
	seed := h.Sum64()
	pct := func(offset uint64, base, spread float64) float64 {
		return round2(base + float64((seed>>offset)%1000)/1000.0*spread)
	}
	promoterPct := pct(0, 35.0, 30.0)   // 35-65%
	fiiPct := pct(8, 8.0, 20.0)         // 8-28%
	diiPct := pct(16, 8.0, 18.0)        // 8-26%
	publicPct := round2(math.Max(0, 100.0-promoterPct-fiiPct-diiPct))

	sectorLabel := strings.ToLower(sector)
	impactText := sp.impactText
	whatBuyingMore := fmt.Sprintf("Buying more will increase your concentration in %s, taking your sector exposure higher.", sector)

	return &stocks.StockProfile{
		Quote:       *quote,
		CompanyName: companyName,
		Sector:      sector,
		Description: fmt.Sprintf(sp.descriptionTemplate, companyName),
		ChartPoints: points,
		Fundamentals: stocks.Fundamentals{
			MarketCap: round2(float64(inst.lotSize) * quote.LastPrice * float64(500000+seed%4500000)),
			PERatio:   round2(10.0 + float64((seed>>24)%2500)/100.0),  // 10-35
			PBRatio:   round2(1.0 + float64((seed>>32)%400)/100.0),    // 1-5
			DivYield:  round2(float64((seed>>40)%400) / 100.0),        // 0-4%
			ROE:       round2(8.0 + float64((seed>>48)%2200)/100.0),   // 8-30%
			High52W:   round2(quote.LastPrice * 1.2),
			Low52W:    round2(quote.LastPrice * 0.8),
		},
		ShareholdingPattern: stocks.ShareholdingPattern{
			Promoter: []stocks.ShareholderInfo{{Title: "Promoters", Percentage: promoterPct}},
			FII:      []stocks.ShareholderInfo{{Title: "Foreign Inst.", Percentage: fiiPct}},
			DII:      []stocks.ShareholderInfo{{Title: "Domestic Inst.", Percentage: diiPct}},
			Public:   []stocks.ShareholderInfo{{Title: "Retail", Percentage: publicPct}},
		},
		InstrumentDeepDive: stocks.InstrumentDeepDive{
			PrimaryRole:   sp.primaryRole,
			SecondaryRole: sp.secondaryRole,
			Strengths:     sp.strengths,
			TradeOffs:     sp.tradeOffs,
		},
		PortfolioInsights: stocks.PortfolioInsights{
			IsPositiveImpact:     true,
			WhyGetFund:           sp.whyGetFund,
			SuitableFor:          sp.suitableFor,
			AvoidIf:              sp.avoidIf,
			ImpactText:           impactText,
			WhatItDoesRightNow:   fmt.Sprintf("It currently provides %s exposure within the %s sector, based on its role as a %s.", sectorLabel, sector, strings.ToLower(sp.primaryRole)),
			WhatBuyingMoreWillDo: whatBuyingMore,
		},
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
