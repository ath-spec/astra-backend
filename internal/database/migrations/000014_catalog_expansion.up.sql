ALTER TABLE fund_catalog ALTER COLUMN scheme_code TYPE VARCHAR(50);
ALTER TABLE fund_catalog ALTER COLUMN category TYPE VARCHAR(100);
ALTER TABLE fund_catalog ALTER COLUMN risk_level TYPE VARCHAR(30);
ALTER TABLE nfo_listings ALTER COLUMN category TYPE VARCHAR(100);

-- Expands the fund catalog to cover the Explore screen's distinct sections
-- (Global US/Europe, Silver, REITs, thematic funds, Corporate Bonds, hybrid
-- risk buckets) which had no matching catalog data before this — same
-- static-reference-data pattern as migration 000003's original 8 funds.
INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('MOTILAL-NASDAQ100-G', 'Motilal Oswal Nasdaq 100 FOF - Growth', 'Motilal Oswal Mutual Fund', 'INF247L01AQ2', 'Equity - Global (US)', 'High', 28.4521, CURRENT_DATE, 0.2400, 4200000000.00, 500.00, 500.00, 22.40, 18.90, 16.20, 'Ajay Khandelwal', 'NASDAQ 100 TRI', '2018-11-29'),
    ('FRANKLIN-USOPP-G', 'Franklin India Feeder - US Opportunities Fund - Growth', 'Franklin Templeton Mutual Fund', 'INF090I01BQ6', 'Equity - Global (US)', 'High', 45.1023, CURRENT_DATE, 1.6800, 1850000000.00, 5000.00, 500.00, 19.80, 16.40, 15.10, 'Vishal Kapoor', 'Russell 1000 Growth Index', '2012-02-06'),
    ('EDEL-EUROPE-G', 'Edelweiss Europe Dynamic Equity Offshore Fund - Growth', 'Edelweiss Mutual Fund', 'INF754K01DZ9', 'Equity - Global (Europe)', 'High', 22.7789, CURRENT_DATE, 1.3500, 620000000.00, 5000.00, 1000.00, 11.20, 9.80, 8.40, 'Bhavesh Jain', 'STOXX Europe 600', '2014-06-20'),
    ('ICICI-SILVER-G', 'ICICI Prudential Silver ETF Fund of Fund - Growth', 'ICICI Prudential Mutual Fund', 'INF109KB1WQ4', 'Other - Silver', 'Medium', 15.6234, CURRENT_DATE, 0.4400, 980000000.00, 1000.00, 100.00, 21.70, 14.30, NULL, 'Manish Banthia', 'Domestic Price of Silver', '2022-01-25'),
    ('KOTAK-REIT-G', 'Kotak International REIT Fund of Fund - Growth', 'Kotak Mutual Fund', 'INF174KA1RT3', 'Other - REIT', 'Medium', 12.3456, CURRENT_DATE, 0.9800, 340000000.00, 1000.00, 100.00, 8.40, 7.10, 6.80, 'Arjun Khanna', 'S&P Asia Pacific ex-Japan REIT', '2020-09-15'),
    ('TATA-GREENENERGY-G', 'Tata Resources & Energy Fund - Growth', 'Tata Mutual Fund', 'INF277K01AT8', 'Equity - Thematic (Energy)', 'High', 34.2201, CURRENT_DATE, 0.8900, 1420000000.00, 5000.00, 500.00, 16.30, 19.70, NULL, 'Meeta Shetty', 'Nifty Energy TRI', '2021-04-08'),
    ('ICICI-TECH-G', 'ICICI Prudential Technology Fund - Growth', 'ICICI Prudential Mutual Fund', 'INF109K01VG1', 'Equity - Thematic (Technology)', 'High', 198.4521, CURRENT_DATE, 1.0200, 12800000000.00, 5000.00, 500.00, 26.80, 24.10, 20.30, 'Vaibhav Dusad', 'S&P BSE Teck', '1999-03-03'),
    ('MIRAE-SEMICON-G', 'Mirae Asset Semiconductor & AI ETF Fund of Fund - Growth', 'Mirae Asset Mutual Fund', 'INF769K01CX6', 'Equity - Thematic (Semiconductor/AI)', 'High', 18.9034, CURRENT_DATE, 0.6800, 890000000.00, 1000.00, 100.00, 31.20, NULL, NULL, 'Ekta Gala', 'iSTOXX Semiconductor Index', '2023-08-14'),
    ('ICICI-MANUF-G', 'ICICI Prudential Manufacturing Fund - Growth', 'ICICI Prudential Mutual Fund', 'INF109K01WL9', 'Equity - Thematic (Manufacturing)', 'High', 27.1145, CURRENT_DATE, 0.9100, 3400000000.00, 5000.00, 500.00, 20.40, 22.90, NULL, 'Lalit Kumar', 'Nifty India Manufacturing TRI', '2022-01-31'),
    ('MIRAE-EVMOB-G', 'Mirae Asset Nifty EV & New Age Automotive ETF Fund of Fund - Growth', 'Mirae Asset Mutual Fund', 'INF769K01DY3', 'Equity - Thematic (EV Mobility)', 'High', 9.8721, CURRENT_DATE, 0.4200, 410000000.00, 1000.00, 100.00, 12.60, NULL, NULL, 'Ekta Gala', 'Nifty EV & New Age Automotive TRI', '2022-09-20'),
    ('ICICI-AUTO-G', 'ICICI Prudential Nifty Auto ETF Fund of Fund - Growth', 'ICICI Prudential Mutual Fund', 'INF109K01XM6', 'Equity - Thematic (Auto & Ancillary)', 'High', 21.3390, CURRENT_DATE, 0.3600, 560000000.00, 1000.00, 100.00, 24.90, 18.20, NULL, 'Nishit Patel', 'Nifty Auto TRI', '2021-11-05'),
    ('HDFC-CORPBOND-G', 'HDFC Corporate Bond Fund - Growth', 'HDFC Mutual Fund', 'INF179K01BM3', 'Debt - Corporate Bond', 'Low', 32.5601, CURRENT_DATE, 0.2800, 28900000000.00, 5000.00, 500.00, 7.80, 6.90, 6.40, 'Anupam Joshi', 'CRISIL Corporate Bond A-II Index', '2014-06-29'),
    ('SBI-CONSHYBRID-G', 'SBI Conservative Hybrid Fund - Growth', 'SBI Mutual Fund', 'INF200K01582', 'Hybrid - Conservative', 'Low', 68.3345, CURRENT_DATE, 0.9800, 9800000000.00, 5000.00, 500.00, 9.60, 8.90, 8.20, 'Dinesh Ahuja', 'CRISIL Hybrid 85+15 Conservative', '2004-12-09'),
    ('AXIS-AGGRHYBRID-G', 'Axis Aggressive Hybrid Fund - Growth', 'Axis Mutual Fund', 'INF846K01DP2', 'Hybrid - Aggressive', 'High', 29.4471, CURRENT_DATE, 1.4500, 5600000000.00, 5000.00, 500.00, 17.80, 16.40, 14.90, 'Shreyash Devalkar', 'CRISIL Hybrid 35+65 Aggressive', '2017-08-14')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
('MOTILAL-NASDAQ100-G', 99.1, 0.2, 0.7,
 '[{"title":"Technology","percentage":52.4},{"title":"Consumer Discretionary","percentage":16.8},{"title":"Communication Services","percentage":11.2},{"title":"Healthcare","percentage":7.1},{"title":"Others","percentage":12.5}]',
 '[{"title":"Apple Inc","percentage":8.9},{"title":"Microsoft Corp","percentage":8.4},{"title":"NVIDIA Corp","percentage":7.6},{"title":"Amazon.com Inc","percentage":5.2}]'),
('FRANKLIN-USOPP-G', 97.6, 0.8, 1.6,
 '[{"title":"Technology","percentage":38.2},{"title":"Healthcare","percentage":15.6},{"title":"Financials","percentage":12.1},{"title":"Consumer Discretionary","percentage":10.8},{"title":"Others","percentage":23.3}]',
 '[{"title":"Microsoft Corp","percentage":6.8},{"title":"Alphabet Inc","percentage":5.9},{"title":"Visa Inc","percentage":4.1},{"title":"UnitedHealth Group","percentage":3.6}]'),
('EDEL-EUROPE-G', 96.8, 1.2, 2.0,
 '[{"title":"Financials","percentage":21.4},{"title":"Industrials","percentage":18.9},{"title":"Healthcare","percentage":14.2},{"title":"Consumer Staples","percentage":11.6},{"title":"Others","percentage":33.9}]',
 '[{"title":"ASML Holding","percentage":5.4},{"title":"LVMH","percentage":4.8},{"title":"Nestle SA","percentage":4.1},{"title":"SAP SE","percentage":3.7}]'),
('ICICI-SILVER-G', 0.0, 0.6, 99.4,
 '[{"title":"Silver ETF Units","percentage":99.4},{"title":"Cash & Equivalents","percentage":0.6}]',
 '[{"title":"ICICI Prudential Silver ETF","percentage":99.4}]'),
('KOTAK-REIT-G', 5.2, 2.1, 92.7,
 '[{"title":"REIT Units (Asia Pacific)","percentage":92.7},{"title":"Equity","percentage":5.2},{"title":"Cash & Equivalents","percentage":2.1}]',
 '[{"title":"Link REIT","percentage":9.8},{"title":"Goodman Group","percentage":8.1},{"title":"Nippon Building Fund","percentage":6.4}]'),
('TATA-GREENENERGY-G', 95.4, 1.6, 3.0,
 '[{"title":"Utilities","percentage":28.6},{"title":"Industrials","percentage":22.1},{"title":"Energy","percentage":19.4},{"title":"Materials","percentage":11.2},{"title":"Others","percentage":18.7}]',
 '[{"title":"NTPC Ltd","percentage":6.9},{"title":"Adani Green Energy","percentage":5.8},{"title":"Tata Power","percentage":5.1},{"title":"Waaree Energies","percentage":4.3}]'),
('ICICI-TECH-G', 96.9, 1.0, 2.1,
 '[{"title":"Technology","percentage":78.4},{"title":"Communication Services","percentage":11.6},{"title":"Consumer Discretionary","percentage":6.9},{"title":"Others","percentage":3.1}]',
 '[{"title":"Infosys","percentage":9.6},{"title":"TCS","percentage":8.8},{"title":"HCL Technologies","percentage":6.2},{"title":"Tech Mahindra","percentage":4.9}]'),
('MIRAE-SEMICON-G', 98.3, 0.4, 1.3,
 '[{"title":"Semiconductors","percentage":71.2},{"title":"Technology Hardware","percentage":18.4},{"title":"Others","percentage":10.4}]',
 '[{"title":"NVIDIA Corp","percentage":9.4},{"title":"Taiwan Semiconductor","percentage":8.7},{"title":"ASML Holding","percentage":6.9},{"title":"Broadcom Inc","percentage":5.8}]'),
('ICICI-MANUF-G', 96.1, 1.4, 2.5,
 '[{"title":"Industrials","percentage":34.8},{"title":"Materials","percentage":19.6},{"title":"Consumer Discretionary","percentage":16.2},{"title":"Energy","percentage":9.4},{"title":"Others","percentage":20.0}]',
 '[{"title":"Larsen & Toubro","percentage":7.2},{"title":"UltraTech Cement","percentage":5.6},{"title":"Bharat Electronics","percentage":4.8},{"title":"Siemens Ltd","percentage":4.1}]'),
('MIRAE-EVMOB-G', 97.2, 0.6, 2.2,
 '[{"title":"Automobiles","percentage":68.9},{"title":"Auto Components","percentage":22.4},{"title":"Others","percentage":8.7}]',
 '[{"title":"Tata Motors","percentage":11.8},{"title":"Mahindra & Mahindra","percentage":9.6},{"title":"Bajaj Auto","percentage":7.4},{"title":"Bharat Forge","percentage":5.2}]'),
('ICICI-AUTO-G', 97.8, 0.5, 1.7,
 '[{"title":"Automobiles","percentage":74.6},{"title":"Auto Components","percentage":21.2},{"title":"Others","percentage":4.2}]',
 '[{"title":"Maruti Suzuki","percentage":13.2},{"title":"Mahindra & Mahindra","percentage":10.9},{"title":"Tata Motors","percentage":9.4},{"title":"Bajaj Auto","percentage":8.1}]'),
('HDFC-CORPBOND-G', 0.0, 97.8, 2.2,
 '[{"title":"AAA Rated Corporate Bonds","percentage":68.4},{"title":"AA+ Rated Corporate Bonds","percentage":18.9},{"title":"Government Securities","percentage":10.5},{"title":"Cash & Equivalents","percentage":2.2}]',
 '[{"title":"REC Ltd","percentage":6.8},{"title":"HDFC Bank Bonds","percentage":5.9},{"title":"NABARD","percentage":5.1},{"title":"Power Finance Corp","percentage":4.6}]'),
('SBI-CONSHYBRID-G', 22.6, 71.4, 6.0,
 '[{"title":"Financial Services","percentage":8.2},{"title":"Government Securities","percentage":31.6},{"title":"AAA Corporate Bonds","percentage":39.8},{"title":"Others","percentage":20.4}]',
 '[{"title":"Government Securities","percentage":31.6},{"title":"HDFC Bank","percentage":3.1},{"title":"REC Ltd","percentage":2.8},{"title":"ICICI Bank","percentage":2.4}]'),
('AXIS-AGGRHYBRID-G', 74.8, 22.1, 3.1,
 '[{"title":"Financial Services","percentage":21.4},{"title":"Technology","percentage":11.8},{"title":"Government Securities","percentage":14.2},{"title":"Energy","percentage":8.6},{"title":"Others","percentage":44.0}]',
 '[{"title":"HDFC Bank","percentage":6.4},{"title":"ICICI Bank","percentage":5.2},{"title":"Government Securities","percentage":14.2},{"title":"Infosys","percentage":3.8}]')
ON CONFLICT (scheme_code) DO NOTHING;
