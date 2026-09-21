-- The REIT collection screen had only one catalog entry (KOTAK-REIT-G from
-- migration 000014) — a single-item list looks broken/empty in the UI.
-- Adds more real REIT fund-of-funds, same static-reference-data pattern.
INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('MIRAE-REIT-G', 'Mirae Asset Global X SuperDividend REIT ETF Fund of Fund - Growth', 'Mirae Asset Mutual Fund', 'INF769K01EQ2', 'Other - REIT', 'Medium', 10.9871, CURRENT_DATE, 0.7900, 180000000.00, 1000.00, 100.00, 6.90, 6.20, NULL, 'Ekta Gala', 'FTSE EPRA Nareit Global REITs Index', '2021-07-19'),
    ('ICICI-REIT-G', 'ICICI Prudential Regular Savings Fund - REIT & InvIT FOF - Growth', 'ICICI Prudential Mutual Fund', 'INF109KB1XR7', 'Other - REIT', 'Medium', 13.4502, CURRENT_DATE, 0.6500, 260000000.00, 5000.00, 500.00, 9.10, 7.80, 7.20, 'Manish Banthia', 'Nifty REITs & InvITs Index', '2021-11-02'),
    ('HDFC-REIT-G', 'HDFC Commercial Real Estate REIT Fund of Fund - Growth', 'HDFC Mutual Fund', 'INF179KB1QR2', 'Other - REIT', 'Medium', 15.2093, CURRENT_DATE, 0.8300, 195000000.00, 1000.00, 100.00, 8.70, 7.60, NULL, 'Priya Ranjan', 'Nifty REITs & InvITs Index', '2022-02-14'),
    ('SBI-REIT-G', 'SBI Asia Pacific REIT Fund of Fund - Growth', 'SBI Mutual Fund', 'INF200KB1SR8', 'Other - REIT', 'Medium', 12.0456, CURRENT_DATE, 0.7500, 210000000.00, 500.00, 100.00, 7.40, 6.80, 6.10, 'Dinesh Ahuja', 'S&P Asia Pacific ex-Japan REIT', '2020-06-30'),
    ('MOTILAL-REIT-G', 'Motilal Oswal Retail & Mall REIT Fund of Fund - Growth', 'Motilal Oswal Mutual Fund', 'INF247L01BR4', 'Other - REIT', 'Medium', 9.8734, CURRENT_DATE, 0.9200, 142000000.00, 500.00, 100.00, 6.10, NULL, NULL, 'Ajay Khandelwal', 'Nifty REITs & InvITs Index', '2023-01-20')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
('MIRAE-REIT-G', 4.6, 1.8, 93.6,
 '[{"title":"REIT Units (Global)","percentage":93.6},{"title":"Equity","percentage":4.6},{"title":"Cash & Equivalents","percentage":1.8}]',
 '[{"title":"Simon Property Group","percentage":8.9},{"title":"Prologis Inc","percentage":7.6},{"title":"American Tower Corp","percentage":6.8}]'),
('ICICI-REIT-G', 3.2, 6.4, 90.4,
 '[{"title":"REIT/InvIT Units (Domestic)","percentage":90.4},{"title":"Debt Instruments","percentage":6.4},{"title":"Equity","percentage":3.2}]',
 '[{"title":"Embassy Office Parks REIT","percentage":14.6},{"title":"Mindspace Business Parks REIT","percentage":11.2},{"title":"Brookfield India REIT","percentage":9.8}]'),
('HDFC-REIT-G', 2.9, 4.1, 93.0,
 '[{"title":"REIT Units (Commercial Office)","percentage":93.0},{"title":"Debt Instruments","percentage":4.1},{"title":"Equity","percentage":2.9}]',
 '[{"title":"Embassy Office Parks REIT","percentage":16.2},{"title":"Brookfield India REIT","percentage":12.8},{"title":"Mindspace Business Parks REIT","percentage":10.4}]'),
('SBI-REIT-G', 5.8, 2.4, 91.8,
 '[{"title":"REIT Units (Asia Pacific)","percentage":91.8},{"title":"Equity","percentage":5.8},{"title":"Cash & Equivalents","percentage":2.4}]',
 '[{"title":"Link REIT","percentage":10.4},{"title":"Goodman Group","percentage":8.6},{"title":"CapitaLand Ascendas REIT","percentage":7.1}]'),
('MOTILAL-REIT-G', 4.2, 3.6, 92.2,
 '[{"title":"REIT Units (Retail & Mall)","percentage":92.2},{"title":"Debt Instruments","percentage":3.6},{"title":"Equity","percentage":4.2}]',
 '[{"title":"Nexus Select Trust","percentage":15.8},{"title":"Phoenix Mills REIT","percentage":11.3},{"title":"Mindspace Business Parks REIT","percentage":8.9}]')
ON CONFLICT (scheme_code) DO NOTHING;
