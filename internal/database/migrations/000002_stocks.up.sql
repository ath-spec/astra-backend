CREATE TABLE IF NOT EXISTS demat_holdings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    isin VARCHAR(12) NOT NULL,
    trading_symbol VARCHAR(20) NOT NULL,
    exchange VARCHAR(4) NOT NULL,
    product VARCHAR(10) NOT NULL DEFAULT 'CNC',
    quantity INTEGER NOT NULL DEFAULT 0,
    average_price NUMERIC(12,2) NOT NULL DEFAULT 0,
    last_price NUMERIC(12,2) NOT NULL DEFAULT 0,
    close_price NUMERIC(12,2) NOT NULL DEFAULT 0,
    authorized_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, isin, product)
);

CREATE TABLE IF NOT EXISTS stock_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id VARCHAR(20) NOT NULL UNIQUE,
    exchange_order_id VARCHAR(20),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exchange VARCHAR(4) NOT NULL,
    trading_symbol VARCHAR(20) NOT NULL,
    isin VARCHAR(12),
    transaction_type VARCHAR(4) NOT NULL,
    quantity INTEGER NOT NULL,
    product VARCHAR(10) NOT NULL,
    order_type VARCHAR(10) NOT NULL,
    price NUMERIC(12,2),
    trigger_price NUMERIC(12,2),
    disclosed_quantity INTEGER NOT NULL DEFAULT 0,
    validity VARCHAR(5) NOT NULL DEFAULT 'DAY',
    status VARCHAR(15) NOT NULL DEFAULT 'OPEN',
    status_message TEXT,
    filled_quantity INTEGER NOT NULL DEFAULT 0,
    pending_quantity INTEGER NOT NULL DEFAULT 0,
    cancelled_quantity INTEGER NOT NULL DEFAULT 0,
    average_price NUMERIC(12,2),
    order_timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    exchange_timestamp TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_demat_holdings_user_id ON demat_holdings(user_id);
CREATE INDEX IF NOT EXISTS idx_stock_orders_user_id ON stock_orders(user_id);
