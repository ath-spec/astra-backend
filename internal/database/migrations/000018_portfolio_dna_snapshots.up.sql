-- Historical record of a user's "portfolio DNA" — the equity/debt/other
-- split, risk level, 7-axis genome, sector exposure and volatility buckets
-- that PortfolioAnalysisService.Allocation computes live from current
-- holdings. That computation has no memory; this table gives it one, so the
-- RM console can show how a client's allocation and DNA have drifted over
-- time. Written lazily (same pattern as portfolio_snapshots, migration
-- 000013): each Allocation read "today" upserts today's row; once the day
-- rolls over the row freezes. No backfill — history starts from first read.
CREATE TABLE IF NOT EXISTS portfolio_dna_snapshots (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    snapshot_date  DATE NOT NULL,
    level          VARCHAR(30) NOT NULL,
    total_value    NUMERIC(18,2) NOT NULL DEFAULT 0,
    equity_pct     NUMERIC(6,2) NOT NULL DEFAULT 0,
    debt_pct       NUMERIC(6,2) NOT NULL DEFAULT 0,
    other_pct      NUMERIC(6,2) NOT NULL DEFAULT 0,
    genome             JSONB NOT NULL DEFAULT '{}',
    sector_exposure    JSONB NOT NULL DEFAULT '[]',
    volatility_buckets JSONB NOT NULL DEFAULT '[]',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, snapshot_date)
);

CREATE INDEX IF NOT EXISTS idx_portfolio_dna_snapshots_user_date
    ON portfolio_dna_snapshots(user_id, snapshot_date);
