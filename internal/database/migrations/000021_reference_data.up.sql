-- Instrument-master reference data, DB-backed from day one so switching to a
-- real feed (broker instruments dump, NSE/BSE, or a data vendor) is a data-sync
-- change, not a code change. The portfolio-analysis service always reads these.

-- Per-symbol sector + company-size band for directly-held equities.
CREATE TABLE IF NOT EXISTS security_reference (
    symbol          VARCHAR(20) PRIMARY KEY,
    isin            VARCHAR(12),
    sector          VARCHAR(60) NOT NULL DEFAULT 'Other Equity',
    market_cap_band VARCHAR(20) NOT NULL DEFAULT 'Large Cap'
        CHECK (market_cap_band IN ('Large Cap','Mid Cap','Small Cap','Micro Cap')),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO security_reference (symbol, sector, market_cap_band) VALUES
    ('RELIANCE',   'Energy',             'Large Cap'),
    ('TCS',        'Technology',         'Large Cap'),
    ('INFY',       'Technology',         'Large Cap'),
    ('HDFCBANK',   'Financial Services', 'Large Cap'),
    ('ICICIBANK',  'Financial Services', 'Large Cap'),
    ('TATAMOTORS', 'Automobiles',        'Large Cap')
ON CONFLICT (symbol) DO NOTHING;

-- Fund company-size band. Populated now from the catalog category; later
-- overwritten by real fund look-through (AMFI monthly portfolio / vendor).
-- 'Diversified' marks flexi/multi/large&mid funds the service splits across
-- bands; NULL leaves the service on its category heuristic.
ALTER TABLE fund_catalog ADD COLUMN IF NOT EXISTS market_cap_band VARCHAR(20);

UPDATE fund_catalog SET market_cap_band = CASE
    WHEN lower(category) LIKE '%micro cap%' THEN 'Micro Cap'
    WHEN lower(category) LIKE '%small cap%' THEN 'Small Cap'
    WHEN lower(category) LIKE '%large & mid%'
      OR lower(category) LIKE '%large and mid%'
      OR lower(category) LIKE '%flexi cap%'
      OR lower(category) LIKE '%multi cap%'      THEN 'Diversified'
    WHEN lower(category) LIKE '%mid cap%'        THEN 'Mid Cap'
    WHEN lower(category) LIKE '%large cap%'
      OR lower(category) LIKE '%bluechip%'       THEN 'Large Cap'
    WHEN lower(category) LIKE '%equity%'         THEN 'Large Cap'
    ELSE NULL
END
WHERE market_cap_band IS NULL;
