-- Daily portfolio value snapshots, backing the Home screen's growth chart.
-- Written lazily (see DashboardService.Summary): each call "today" updates
-- today's row in place; once the calendar day rolls over, that row freezes
-- and a new one starts. There is no backfill — history only exists from the
-- point this was introduced onward, which is the honest tradeoff for not
-- having a real historical price feed to reconstruct the past from.
CREATE TABLE IF NOT EXISTS portfolio_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    snapshot_date DATE NOT NULL,
    total_wealth NUMERIC(18,2) NOT NULL,
    mutual_funds_value NUMERIC(18,2) NOT NULL DEFAULT 0,
    stocks_value NUMERIC(18,2) NOT NULL DEFAULT 0,
    fixed_deposits_value NUMERIC(18,2) NOT NULL DEFAULT 0,
    bank_balance_value NUMERIC(18,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, snapshot_date)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_snapshots_user_date ON portfolio_snapshots(user_id, snapshot_date);
