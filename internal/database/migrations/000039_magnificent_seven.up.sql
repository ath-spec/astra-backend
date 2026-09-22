-- The "Magnificent 7" card on the Global Invest screen
-- (mf_global_invest_collections_screen.dart) was only ever matching one fund
-- (Motilal Oswal Nasdaq 100 FOF, via its broad "cat.contains('us')" filter)
-- because the catalog had no individual mega-cap US company entries. Adds
-- the actual seven Magnificent 7 companies as direct international stock
-- investment items, tagged with a distinct category so the frontend filter
-- can target exactly these seven instead of the whole Global (US) bucket
-- (which still correctly includes Motilal Oswal Nasdaq 100 FOF for the
-- "United States" geography card and the Global Funds screen's "US Equity"
-- chip).
INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('AAPL-US-G',  'Apple Inc.',              'Astra Global Direct Stocks', 'US0378331005', 'Equity - US Mega Cap', 'High', 21540.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 14.80, 22.40, 26.10, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('MSFT-US-G',  'Microsoft Corporation',   'Astra Global Direct Stocks', 'US5949181045', 'Equity - US Mega Cap', 'High', 37200.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 19.60, 24.80, 28.90, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('GOOGL-US-G', 'Alphabet Inc.',           'Astra Global Direct Stocks', 'US02079K3059', 'Equity - US Mega Cap', 'High', 14680.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 25.30, 21.60, 24.20, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('AMZN-US-G',  'Amazon.com Inc.',         'Astra Global Direct Stocks', 'US0231351067', 'Equity - US Mega Cap', 'High', 15920.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 31.40, 28.10, 25.60, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('NVDA-US-G',  'NVIDIA Corporation',      'Astra Global Direct Stocks', 'US67066G1040', 'Equity - US Mega Cap', 'High', 11280.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 78.90, 92.40, 68.30, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('META-US-G',  'Meta Platforms Inc.',     'Astra Global Direct Stocks', 'US30303M1027', 'Equity - US Mega Cap', 'High', 48360.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00, 41.60, 38.20, 22.70, NULL, 'NASDAQ 100 TRI', '2024-01-01'),
    ('TSLA-US-G',  'Tesla Inc.',              'Astra Global Direct Stocks', 'US88160R1014', 'Equity - US Mega Cap', 'High', 20740.00, CURRENT_DATE, 0.0000, 0.00, 100.00, 100.00,  8.40, 18.90, 32.50, NULL, 'NASDAQ 100 TRI', '2024-01-01')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
    ('AAPL-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Technology Hardware","percentage":100.0}]',
     '[{"title":"Apple Inc. (Direct Stock)","percentage":100.0}]'),
    ('MSFT-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Software","percentage":100.0}]',
     '[{"title":"Microsoft Corporation (Direct Stock)","percentage":100.0}]'),
    ('GOOGL-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Internet & Media","percentage":100.0}]',
     '[{"title":"Alphabet Inc. (Direct Stock)","percentage":100.0}]'),
    ('AMZN-US-G', 100.0, 0.0, 0.0,
     '[{"title":"E-Commerce & Cloud","percentage":100.0}]',
     '[{"title":"Amazon.com Inc. (Direct Stock)","percentage":100.0}]'),
    ('NVDA-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Semiconductors","percentage":100.0}]',
     '[{"title":"NVIDIA Corporation (Direct Stock)","percentage":100.0}]'),
    ('META-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Internet & Media","percentage":100.0}]',
     '[{"title":"Meta Platforms Inc. (Direct Stock)","percentage":100.0}]'),
    ('TSLA-US-G', 100.0, 0.0, 0.0,
     '[{"title":"Automobiles","percentage":100.0}]',
     '[{"title":"Tesla Inc. (Direct Stock)","percentage":100.0}]')
ON CONFLICT (scheme_code) DO NOTHING;
