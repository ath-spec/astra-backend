-- Static allocation/sector/holdings breakdown per catalog fund, backing the
-- Explore screen's fund-profile page (equity/debt/other split, sector
-- exposure, top holdings). This is reference data of the same kind already
-- seeded into fund_catalog — not derived from any live source — since no
-- real RTA/AMC holdings-disclosure feed is wired in yet.
CREATE TABLE IF NOT EXISTS fund_allocation (
    scheme_code VARCHAR(20) PRIMARY KEY REFERENCES fund_catalog(scheme_code) ON DELETE CASCADE,
    equity_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    debt_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    other_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    sectors JSONB NOT NULL DEFAULT '[]',
    top_holdings JSONB NOT NULL DEFAULT '[]'
);

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
('HDFC-MC-G', 96.5, 1.5, 2.0,
 '[{"title":"Financial Services","percentage":24.1},{"title":"Industrials","percentage":16.8},{"title":"Consumer Discretionary","percentage":13.4},{"title":"Healthcare","percentage":10.2},{"title":"Others","percentage":35.5}]',
 '[{"title":"Coforge Ltd","percentage":4.2},{"title":"Persistent Systems","percentage":3.8},{"title":"Cummins India","percentage":3.5},{"title":"Max Healthcare","percentage":3.1}]'),
('SBI-BLC-G', 97.8, 0.7, 1.5,
 '[{"title":"Financial Services","percentage":34.6},{"title":"Technology","percentage":14.2},{"title":"Consumer Staples","percentage":11.8},{"title":"Energy","percentage":9.5},{"title":"Others","percentage":29.9}]',
 '[{"title":"HDFC Bank","percentage":8.9},{"title":"ICICI Bank","percentage":7.6},{"title":"Reliance Industries","percentage":6.2},{"title":"Infosys","percentage":5.1}]'),
('AXIS-SC-G', 95.2, 1.8, 3.0,
 '[{"title":"Industrials","percentage":22.4},{"title":"Consumer Discretionary","percentage":18.1},{"title":"Financial Services","percentage":15.6},{"title":"Materials","percentage":12.3},{"title":"Others","percentage":31.6}]',
 '[{"title":"KEI Industries","percentage":3.4},{"title":"PG Electroplast","percentage":3.1},{"title":"Blue Star","percentage":2.9},{"title":"Century Plyboards","percentage":2.6}]'),
('ICICI-BAF-G', 42.5, 48.0, 9.5,
 '[{"title":"Financial Services","percentage":11.2},{"title":"Technology","percentage":6.8},{"title":"Energy","percentage":5.4},{"title":"Others","percentage":19.1}]',
 '[{"title":"Government Securities","percentage":22.3},{"title":"HDFC Bank","percentage":4.1},{"title":"ICICI Bank","percentage":3.6},{"title":"Reliance Industries","percentage":3.0}]'),
('MIRAE-EM-G', 96.9, 1.1, 2.0,
 '[{"title":"Financial Services","percentage":27.8},{"title":"Technology","percentage":13.5},{"title":"Consumer Discretionary","percentage":12.1},{"title":"Healthcare","percentage":9.4},{"title":"Others","percentage":34.1}]',
 '[{"title":"HDFC Bank","percentage":6.8},{"title":"ICICI Bank","percentage":5.9},{"title":"Trent Ltd","percentage":4.3},{"title":"Max Healthcare","percentage":3.7}]'),
('PARAG-FLX-G', 90.1, 3.4, 6.5,
 '[{"title":"Technology","percentage":19.6},{"title":"Financial Services","percentage":17.2},{"title":"Consumer Staples","percentage":11.8},{"title":"Energy","percentage":8.9},{"title":"Others","percentage":42.6}]',
 '[{"title":"HDFC Bank","percentage":7.1},{"title":"Bajaj Holdings","percentage":6.4},{"title":"ITC Ltd","percentage":4.8},{"title":"Alphabet Inc (Global)","percentage":3.2}]'),
('HDFC-LIQ-G', 0.0, 98.5, 1.5,
 '[{"title":"Treasury Bills","percentage":41.2},{"title":"Commercial Paper","percentage":33.6},{"title":"Certificate of Deposit","percentage":23.7}]',
 '[{"title":"91-Day T-Bill","percentage":18.4},{"title":"NABARD CP","percentage":9.6},{"title":"SBI CD","percentage":8.1},{"title":"HDFC Ltd CP","percentage":7.3}]'),
('KOTAK-GOLD-G', 0.0, 0.5, 99.5,
 '[{"title":"Gold ETF Units","percentage":99.5},{"title":"Cash & Equivalents","percentage":0.5}]',
 '[{"title":"Kotak Gold ETF","percentage":99.5}]')
ON CONFLICT (scheme_code) DO NOTHING;
