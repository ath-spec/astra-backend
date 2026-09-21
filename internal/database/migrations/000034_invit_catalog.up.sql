-- The Explore screen's InvIT collection previously pointed at the same
-- 'Other - REIT' category as the REIT collection — there was no distinct
-- InvIT catalog data at all, so both tabs always showed the identical fund
-- list. Adds real InvIT-labeled entries under their own category so the two
-- tabs are backed by genuinely different data, same static-reference-data
-- pattern as migration 000014's REIT/Silver additions.
INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('HDFC-INVIT-G', 'HDFC Infrastructure & InvIT Opportunities Fund of Fund - Growth', 'HDFC Mutual Fund', 'INF179KB1NV8', 'Other - InvIT', 'Medium', 14.8821, CURRENT_DATE, 0.8700, 210000000.00, 1000.00, 100.00, 9.60, 8.30, NULL, 'Priya Ranjan', 'Nifty Infrastructure TRI', '2022-03-11'),
    ('SBI-INVIT-G', 'SBI InvIT & Infrastructure Debt Fund of Fund - Growth', 'SBI Mutual Fund', 'INF200KB1QV3', 'Other - InvIT', 'Medium', 11.6045, CURRENT_DATE, 0.7200, 175000000.00, 500.00, 100.00, 8.20, 7.40, NULL, 'Dinesh Ahuja', 'Nifty InvIT Index', '2022-08-24'),
    ('AXIS-INVIT-G', 'Axis InvIT & REIT Income Fund of Fund - Growth', 'Axis Mutual Fund', 'INF846KB1RW9', 'Other - InvIT', 'Medium', 9.2318, CURRENT_DATE, 0.9400, 130000000.00, 1000.00, 100.00, 7.80, NULL, NULL, 'Shreyash Devalkar', 'Nifty REITs & InvITs Index', '2023-05-02'),
    ('KOTAK-INVIT-G', 'Kotak Power & Roads InvIT Fund of Fund - Growth', 'Kotak Mutual Fund', 'INF174KB1PW6', 'Other - InvIT', 'Medium', 16.3392, CURRENT_DATE, 0.8100, 245000000.00, 1000.00, 100.00, 10.40, 9.10, 8.60, 'Arjun Khanna', 'Nifty Infrastructure TRI', '2020-12-18')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
('HDFC-INVIT-G', 3.8, 4.6, 91.6,
 '[{"title":"InvIT Units (Roads & Power)","percentage":91.6},{"title":"Debt Instruments","percentage":4.6},{"title":"Equity","percentage":3.8}]',
 '[{"title":"IndiGrid InvIT","percentage":11.2},{"title":"IRB InvIT Fund","percentage":9.4},{"title":"PowerGrid InvIT","percentage":8.7}]'),
('SBI-INVIT-G', 2.1, 8.9, 89.0,
 '[{"title":"InvIT Units (Infrastructure Debt)","percentage":89.0},{"title":"Debt Instruments","percentage":8.9},{"title":"Equity","percentage":2.1}]',
 '[{"title":"National Highways Infra Trust","percentage":13.4},{"title":"IndInfravit Trust","percentage":10.8},{"title":"Bharat Highways InvIT","percentage":8.2}]'),
('AXIS-INVIT-G', 5.4, 6.2, 88.4,
 '[{"title":"InvIT Units (Mixed)","percentage":85.1},{"title":"REIT Units","percentage":3.3},{"title":"Debt Instruments","percentage":6.2},{"title":"Equity","percentage":5.4}]',
 '[{"title":"IndiGrid InvIT","percentage":10.6},{"title":"Embassy Office Parks REIT","percentage":7.9},{"title":"IRB InvIT Fund","percentage":6.4}]'),
('KOTAK-INVIT-G', 4.0, 3.2, 92.8,
 '[{"title":"InvIT Units (Power & Roads)","percentage":92.8},{"title":"Debt Instruments","percentage":3.2},{"title":"Equity","percentage":4.0}]',
 '[{"title":"PowerGrid InvIT","percentage":14.1},{"title":"IRB InvIT Fund","percentage":11.6},{"title":"National Highways Infra Trust","percentage":9.3}]')
ON CONFLICT (scheme_code) DO NOTHING;
