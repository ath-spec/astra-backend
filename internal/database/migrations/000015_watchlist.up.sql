-- Per-user saved/bookmarked funds, backing the "bookmark" button on the fund
-- profile screen and the Watchlist screen's list. Real data from day one —
-- there's no external vendor concept for a watchlist, it's purely this
-- app's own state, so unlike Stocks/MF/FD there's no "mock vs real
-- provider" split needed here.
CREATE TABLE IF NOT EXISTS watchlist_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scheme_code VARCHAR(50) NOT NULL REFERENCES fund_catalog(scheme_code) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, scheme_code)
);

CREATE INDEX IF NOT EXISTS idx_watchlist_items_user_id ON watchlist_items(user_id);
