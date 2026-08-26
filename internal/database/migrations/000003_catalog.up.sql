-- Fund/scheme catalog and NFO listings are shared reference data (not
-- per-user), matching the sandbox spec's "Fund & Scheme Catalog / NFO"
-- domain. Seeded once below so /api/v1/catalog has realistic data from the
-- first request.
CREATE TABLE IF NOT EXISTS fund_catalog (
    scheme_code VARCHAR(20) PRIMARY KEY,
    scheme_name VARCHAR(150) NOT NULL,
    amc_name VARCHAR(100) NOT NULL,
    isin VARCHAR(12) NOT NULL,
    asset_class VARCHAR(15) NOT NULL DEFAULT 'MUTUAL_FUND',
    category VARCHAR(30) NOT NULL,
    risk_level VARCHAR(10) NOT NULL,
    nav NUMERIC(10,4) NOT NULL,
    nav_date DATE NOT NULL,
    expense_ratio NUMERIC(6,4) NOT NULL,
    aum NUMERIC(18,2) NOT NULL,
    min_investment NUMERIC(18,2) NOT NULL,
    min_sip_amount NUMERIC(18,2) NOT NULL,
    returns_1y NUMERIC(7,2),
    returns_3y NUMERIC(7,2),
    returns_5y NUMERIC(7,2),
    fund_manager VARCHAR(100),
    benchmark_index VARCHAR(100),
    launch_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nfo_listings (
    nfo_id VARCHAR(20) PRIMARY KEY,
    scheme_name VARCHAR(150) NOT NULL,
    amc_name VARCHAR(100) NOT NULL,
    category VARCHAR(30) NOT NULL,
    offer_open_date DATE NOT NULL,
    offer_close_date DATE NOT NULL,
    offer_price NUMERIC(10,4) NOT NULL DEFAULT 10.0000,
    min_investment NUMERIC(18,2) NOT NULL,
    allotment_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fund_catalog_category ON fund_catalog(category);
CREATE INDEX IF NOT EXISTS idx_fund_catalog_risk_level ON fund_catalog(risk_level);
CREATE INDEX IF NOT EXISTS idx_fund_catalog_min_investment ON fund_catalog(min_investment);

INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('HDFC-MC-G', 'HDFC Mid-Cap Opportunities Fund - Growth', 'HDFC Mutual Fund', 'INF179K01YV8', 'Equity - Mid Cap', 'Medium', 142.3821, CURRENT_DATE, 0.0185, 18450000000.00, 5000.00, 500.00, 18.40, 22.10, 16.75, 'Chirag Setalvad', 'NIFTY Midcap 150', '2007-06-25'),
    ('SBI-BLC-G', 'SBI Bluechip Fund - Growth', 'SBI Mutual Fund', 'INF200K01158', 'Equity - Large Cap', 'Medium', 78.4521, CURRENT_DATE, 0.0155, 32100000000.00, 5000.00, 500.00, 14.20, 15.80, 13.90, 'Sohini Andani', 'NIFTY 100', '2006-02-14'),
    ('AXIS-SC-G', 'Axis Small Cap Fund - Growth', 'Axis Mutual Fund', 'INF846K01EY7', 'Equity - Small Cap', 'High', 95.1032, CURRENT_DATE, 0.0198, 12800000000.00, 5000.00, 500.00, 24.60, 26.40, 19.10, 'Anupam Tiwari', 'NIFTY Smallcap 250', '2013-11-29'),
    ('ICICI-BAF-G', 'ICICI Prudential Balanced Advantage Fund - Growth', 'ICICI Prudential Mutual Fund', 'INF109K01AA1', 'Hybrid - Balanced Advantage', 'Low', 61.2290, CURRENT_DATE, 0.0142, 54200000000.00, 1000.00, 100.00, 11.80, 12.50, 11.30, 'Sankaran Naren', 'CRISIL Hybrid 50+50', '2006-12-30'),
    ('MIRAE-EM-G', 'Mirae Asset Emerging Bluechip Fund - Growth', 'Mirae Asset Mutual Fund', 'INF769K01AX1', 'Equity - Large & Mid Cap', 'Medium', 118.7745, CURRENT_DATE, 0.0165, 28900000000.00, 5000.00, 500.00, 19.90, 21.30, 17.60, 'Neelesh Surana', 'NIFTY Large Midcap 250', '2010-07-09'),
    ('PARAG-FLX-G', 'Parag Parikh Flexi Cap Fund - Growth', 'PPFAS Mutual Fund', 'INF879O01027', 'Equity - Flexi Cap', 'Medium', 74.9012, CURRENT_DATE, 0.0091, 61500000000.00, 1000.00, 1000.00, 20.10, 23.80, 18.90, 'Rajeev Thakkar', 'NIFTY 500', '2013-05-24'),
    ('HDFC-LIQ-G', 'HDFC Liquid Fund - Growth', 'HDFC Mutual Fund', 'INF179K01158', 'Debt - Liquid', 'Low', 4521.6634, CURRENT_DATE, 0.0035, 42300000000.00, 500.00, 500.00, 6.90, 5.80, 5.60, 'Anil Bamboli', 'CRISIL Liquid Fund Index', '2002-10-17'),
    ('KOTAK-GOLD-G', 'Kotak Gold Fund - Growth', 'Kotak Mutual Fund', 'INF174K01LS3', 'Other - Gold', 'Medium', 28.3345, CURRENT_DATE, 0.0059, 3200000000.00, 1000.00, 100.00, 15.30, 13.20, 10.80, 'Abhishek Bisen', 'Domestic Price of Gold', '2011-03-25')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO nfo_listings
    (nfo_id, scheme_name, amc_name, category, offer_open_date, offer_close_date, offer_price, min_investment, allotment_date)
VALUES
    ('NFO-2026-0341', 'Zenith Momentum Fund', 'Axis Mutual Fund', 'Equity - Thematic', CURRENT_DATE + 7, CURRENT_DATE + 21, 10.0000, 5000.00, CURRENT_DATE + 26),
    ('NFO-2026-0342', 'Quant Manufacturing Fund', 'Quant Mutual Fund', 'Equity - Sectoral', CURRENT_DATE + 14, CURRENT_DATE + 28, 10.0000, 5000.00, CURRENT_DATE + 33)
ON CONFLICT (nfo_id) DO NOTHING;
