-- Replaces the backend's own independently-invented fund_catalog rows with
-- exactly the 38 funds the frontend already uses as its own mock data
-- (astra-frontend's lib/features/mf/screens/mf_explore/data/mf_mock_fund_data.dart
-- and mf_stocks_collection_screen.dart) — same names, same per-category
-- counts (5 Large Cap, 5 Mid Cap, 5 Small Cap, 7 Value, 1 each of Flexi Cap/
-- Gold/Silver/REIT/InvIT/FD, 4 Thematic, 3 Global, 3 Debt), not a padded-out
-- superset. Categories the frontend mock doesn't cover at all (Hybrid,
-- Emerging Markets) are intentionally left absent rather than invented.
-- Two more thematic funds (Renewable Energy, EV Mobility) are added beyond
-- that 38 — not from the mock data, but required so the live "Trending
-- Themes" screen's keyword filters aren't left pointing at zero funds.
-- fund_allocation and watchlist both FK scheme_code -> fund_catalog ON
-- DELETE CASCADE, so deleting every old catalog row also clears any demo
-- watchlist entries pointing at the old (now-gone) scheme codes —
-- acceptable for this still-pre-launch catalog reseed, not something to do
-- once this catalog has real user data pinned to it.
DELETE FROM fund_catalog;

INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    -- Large Cap
    ('MIRAE-LC-G',    'Mirae Asset Large Cap Fund',            'Mirae Asset Mutual Fund',              'INF769K01AA1', 'Equity - Large Cap', 'Medium', 118.7745, CURRENT_DATE, 0.0165, 28900000000.00, 5000.00, 500.00, NULL, 16.80, NULL, 'Neelesh Surana',   'Nifty 100', '2010-07-09'),
    ('HDFC-T100-G',   'HDFC Top 100 Fund',                     'HDFC Mutual Fund',                     'INF179K01AB2', 'Equity - Large Cap', 'Medium', 985.2200, CURRENT_DATE, 0.0158, 34200000000.00, 5000.00, 500.00, NULL, 15.90, NULL, 'Rahul Baijal',     'Nifty 100', '1996-10-11'),
    ('ICICI-BC-G',    'ICICI Pru Bluechip Fund',                'ICICI Prudential Mutual Fund',         'INF109K01AC3', 'Equity - Large Cap', 'Medium', 92.4410, CURRENT_DATE, 0.0152, 58600000000.00, 5000.00, 500.00, NULL, 15.20, NULL, 'Anish Tawakley',   'Nifty 100', '2008-05-23'),
    ('SBI-BLC-G',     'SBI Bluechip Fund',                      'SBI Mutual Fund',                      'INF200K01158', 'Equity - Large Cap', 'Medium', 78.4521, CURRENT_DATE, 0.0155, 32100000000.00, 5000.00, 500.00, NULL, 14.70, NULL, 'Sohini Andani',    'Nifty 100', '2006-02-14'),
    ('AXIS-BC-G',     'Axis Bluechip Fund',                     'Axis Mutual Fund',                     'INF846K01AD4', 'Equity - Large Cap', 'Medium', 56.1290, CURRENT_DATE, 0.0148, 24700000000.00, 5000.00, 500.00, NULL, 13.50, NULL, 'Shreyash Devalkar','Nifty 100', '2010-01-05'),

    -- Mid Cap
    ('QUANT-MC-G',    'Quant Mid Cap Fund',                     'Quant Mutual Fund',                    'INF846K01AE5', 'Equity - Mid Cap', 'High', 201.3390, CURRENT_DATE, 0.0189, 8900000000.00, 5000.00, 500.00, NULL, 32.40, NULL, 'Sanjeev Sharma', 'NIFTY Midcap 150', '2007-03-29'),
    ('NIPPON-GR-G',   'Nippon India Growth Fund',                'Nippon India Mutual Fund',              'INF204K01AF6', 'Equity - Mid Cap', 'High', 3412.6600, CURRENT_DATE, 0.0172, 26800000000.00, 5000.00, 500.00, NULL, 28.60, NULL, 'Rupesh Patel',    'NIFTY Midcap 150', '1995-10-08'),
    ('HDFC-MC-G',     'HDFC Mid-Cap Opportunities',              'HDFC Mutual Fund',                     'INF179K01YV8', 'Equity - Mid Cap', 'High', 142.3821, CURRENT_DATE, 0.0185, 18450000000.00, 5000.00, 500.00, NULL, 26.90, NULL, 'Chirag Setalvad', 'NIFTY Midcap 150', '2007-06-25'),
    ('MOTILAL-MC-G',  'Motilal Oswal Midcap Fund',                'Motilal Oswal Mutual Fund',             'INF247L01AG7', 'Equity - Mid Cap', 'High', 89.7710, CURRENT_DATE, 0.0176, 15300000000.00, 5000.00, 500.00, NULL, 25.30, NULL, 'Niket Shah',      'NIFTY Midcap 150', '2014-02-24'),
    ('EDEL-MC-G',     'Edelweiss Mid Cap Fund',                   'Edelweiss Mutual Fund',                 'INF754K01AH8', 'Equity - Mid Cap', 'High', 74.2290, CURRENT_DATE, 0.0181, 6200000000.00, 5000.00, 500.00, NULL, 23.80, NULL, 'Trideep Bhattacharya','NIFTY Midcap 150', '2008-12-26'),

    -- Small Cap
    ('NIPPON-SC-G',   'Nippon India Small Cap Fund',              'Nippon India Mutual Fund',              'INF204K01AI9', 'Equity - Small Cap', 'High', 168.4420, CURRENT_DATE, 0.0196, 49500000000.00, 5000.00, 500.00, NULL, 38.60, NULL, 'Samir Rachh',   'NIFTY Smallcap 250', '2010-09-16'),
    ('SBI-SC-G',      'SBI Small Cap Fund',                       'SBI Mutual Fund',                       'INF200K01AJ0', 'Equity - Small Cap', 'High', 154.9010, CURRENT_DATE, 0.0193, 32600000000.00, 5000.00, 500.00, NULL, 34.20, NULL, 'R. Srinivasan', 'NIFTY Smallcap 250', '2009-09-09'),
    ('AXIS-SC-G',     'Axis Small Cap Fund',                      'Axis Mutual Fund',                      'INF846K01EY7', 'Equity - Small Cap', 'High', 95.1032, CURRENT_DATE, 0.0198, 12800000000.00, 5000.00, 500.00, NULL, 29.80, NULL, 'Anupam Tiwari', 'NIFTY Smallcap 250', '2013-11-29'),
    ('DSP-SC-G',      'DSP Small Cap Fund',                       'DSP Mutual Fund',                       'INF740K01AK1', 'Equity - Small Cap', 'High', 132.0980, CURRENT_DATE, 0.0189, 14100000000.00, 5000.00, 500.00, NULL, 27.50, NULL, 'Vinit Sambre',  'NIFTY Smallcap 250', '2006-06-14'),
    ('QUANT-SC-G',    'Quant Small Cap Fund',                     'Quant Mutual Fund',                     'INF846K01AL2', 'Equity - Small Cap', 'High', 219.4460, CURRENT_DATE, 0.0192, 21400000000.00, 5000.00, 500.00, NULL, 21.40, NULL, 'Sanjeev Sharma','NIFTY Smallcap 250', '1996-04-01'),

    -- Flexi Cap
    ('PARAG-FLX-G',   'Parag Parikh Flexi Cap Fund',              'PPFAS Mutual Fund',                     'INF879O01027', 'Equity - Flexi Cap', 'Medium', 74.9012, CURRENT_DATE, 0.0091, 61500000000.00, 1000.00, 1000.00, NULL, 21.40, NULL, 'Rajeev Thakkar', 'NIFTY 500', '2013-05-24'),

    -- Value
    ('QUANT-VAL-G',   'Quant Value Growth Direct Plan',           'Quant Mutual Fund',                     'INF846K01AM3', 'Equity - Value', 'High', 88.5610, CURRENT_DATE, 0.0179, 7400000000.00, 5000.00, 500.00, NULL, 22.56, NULL, 'Ankit Pande', 'Nifty 50', '2000-10-12'),
    ('AXIS-VAL-G',    'Axis Value Fund Direct',                   'Axis Mutual Fund',                      'INF846K01AN4', 'Equity - Value', 'Medium', 24.8810, CURRENT_DATE, 0.0161, 1900000000.00, 5000.00, 500.00, NULL, 18.73, NULL, 'Jinesh Gopani', 'Nifty 50', '2021-03-11'),
    ('HSBC-VAL-G',    'HSBC Value Fund Direct Growth',            'HSBC Mutual Fund',                      'INF336L01AO5', 'Equity - Value', 'High', 112.3450, CURRENT_DATE, 0.0184, 16200000000.00, 5000.00, 500.00, NULL, 18.40, NULL, 'Venugopal Manghat', 'Nifty 50', '2010-01-08'),
    ('DSP-VAL-G',     'DSP Value Fund',                           'DSP Mutual Fund',                       'INF740K01AP6', 'Equity - Value', 'High', 33.6210, CURRENT_DATE, 0.0177, 3100000000.00, 5000.00, 500.00, NULL, 17.50, NULL, 'Abhishek Singh', 'Nifty 50', '2023-08-01'),
    ('LIC-VAL-G',     'LIC MF Value Fund',                        'LIC Mutual Fund',                       'INF767K01AQ7', 'Equity - Value', 'High', 61.0980, CURRENT_DATE, 0.0221, 480000000.00, 5000.00, 500.00, NULL, 17.50, NULL, 'Yogesh Patil', 'Nifty 50', '2004-06-27'),
    ('HDFC-VAL-G',    'HDFC Value Fund',                          'HDFC Mutual Fund',                      'INF179K01AR8', 'Equity - Value', 'High', 145.7760, CURRENT_DATE, 0.0169, 8600000000.00, 5000.00, 500.00, NULL, 17.50, NULL, 'Gopal Agrawal', 'Nifty 50', '2010-02-08'),
    ('ABSL-VAL-G',    'Aditya Birla Sun Life Value Fund',         'Aditya Birla Sun Life Mutual Fund',     'INF209K01AS9', 'Equity - Value', 'High', 118.4420, CURRENT_DATE, 0.0195, 4300000000.00, 5000.00, 500.00, NULL, 17.50, NULL, 'Dhaval Gala', 'Nifty 50', '2005-03-27'),

    -- Thematic / Sectoral
    ('ICICI-TECH-G',  'Tech & AI Opportunities Fund',             'ICICI Prudential Mutual Fund',          'INF109K01VG1', 'Equity - Thematic (Technology)', 'High', 198.4521, CURRENT_DATE, 0.0102, 12800000000.00, 5000.00, 500.00, NULL, 34.20, NULL, 'Vaibhav Dusad', 'Nifty IT', '1999-03-03'),
    ('ICICI-MANUF-G', 'India Manufacturing Growth Fund',          'ICICI Prudential Mutual Fund',          'INF109K01WL9', 'Equity - Thematic (Manufacturing)', 'High', 27.1145, CURRENT_DATE, 0.0091, 3400000000.00, 5000.00, 500.00, NULL, 28.40, NULL, 'Lalit Kumar', 'Nifty 50', '2022-01-31'),
    ('MIRAE-SEMI-G',  'Global Semiconductor Fund',                 'Mirae Asset Mutual Fund',                'INF769K01CX6', 'Equity - Thematic (Semiconductor/AI)', 'High', 18.9034, CURRENT_DATE, 0.0068, 890000000.00, 1000.00, 100.00, NULL, 42.10, NULL, 'Ekta Gala', 'Global Tech', '2023-08-14'),
    ('SBI-HC-G',      'SBI Healthcare Opportunities Fund',         'SBI Mutual Fund',                       'INF200K01AT0', 'Equity - Thematic (Healthcare)', 'High', 312.8870, CURRENT_DATE, 0.0184, 3200000000.00, 5000.00, 500.00, NULL, 19.50, NULL, 'Tanmaya Desai', 'Pharma Index', '1999-06-30'),

    -- Two extra thematic funds not present in the frontend mock data, added
    -- so the "Trending Themes" screen's keyword-filtered Renewable Energy
    -- and EV Mobility tiles (mf_new_trending_themes.dart) aren't empty.
    ('TATA-GREEN-G',  'Tata Resources & Energy Fund',              'Tata Mutual Fund',                       'INF277K01AT8', 'Equity - Thematic (Energy)', 'High', 34.2201, CURRENT_DATE, 0.0089, 1420000000.00, 5000.00, 500.00, NULL, 19.70, NULL, 'Meeta Shetty', 'Nifty Energy TRI', '2021-04-08'),
    ('MIRAE-EVMOB-G', 'Mirae Asset EV & Mobility Fund',            'Mirae Asset Mutual Fund',                 'INF769K01DY3', 'Equity - Thematic (EV Mobility)', 'High', 9.8721, CURRENT_DATE, 0.0042, 410000000.00, 1000.00, 100.00, NULL, 18.20, NULL, 'Ekta Gala', 'Nifty EV & New Age Automotive TRI', '2022-09-20'),

    -- Global
    ('MOTILAL-N100-G','Motilal Oswal Nasdaq 100 FOF',              'Motilal Oswal Mutual Fund',              'INF247L01AQ2', 'Equity - Global (US)', 'High', 28.4521, CURRENT_DATE, 0.2400, 4200000000.00, 500.00, 500.00, NULL, 26.80, NULL, 'Ajay Khandelwal', 'NASDAQ 100 TRI', '2018-11-29'),
    ('NIPPON-JP-G',   'Nippon India Japan Equity Fund',            'Nippon India Mutual Fund',                'INF204K01AU1', 'Equity - Global (Japan)', 'High', 19.6720, CURRENT_DATE, 0.7100, 640000000.00, 1000.00, 100.00, NULL, 17.20, NULL, 'Kinjal Desai', 'Nikkei 225', '2014-06-16'),
    ('INVESCO-EU-G',  'Invesco India Europe Fund',                 'Invesco Mutual Fund',                    'INF754K01DZ9', 'Equity - Global (Europe)', 'High', 22.7789, CURRENT_DATE, 1.3500, 620000000.00, 5000.00, 1000.00, NULL, -4.40, NULL, 'Bhavesh Jain', 'Euro Stoxx 50', '2014-06-20'),

    -- Debt / Liquid
    ('HDFC-CORPBOND-G','HDFC Corporate Bond Direct Plan',          'HDFC Mutual Fund',                       'INF179K01BM3', 'Debt - Corporate Bond', 'Low', 32.5601, CURRENT_DATE, 0.0028, 28900000000.00, 5000.00, 500.00, NULL, 7.28, NULL, 'Anupam Joshi', 'CRISIL Corporate Bond A-II Index', '2014-06-29'),
    ('SBI-STD-G',     'SBI Short Term Debt Fund',                  'SBI Mutual Fund',                        'INF200K01AV2', 'Debt - Short Term', 'Low', 28.9430, CURRENT_DATE, 0.0068, 15600000000.00, 5000.00, 500.00, NULL, 7.10, NULL, 'Rajeev Radhakrishnan', 'CRISIL Short Term Debt Index', '2000-07-27'),
    ('ICICI-LIQ-G',   'ICICI Pru Liquid Fund',                     'ICICI Prudential Mutual Fund',           'INF109K01AW3', 'Debt - Liquid', 'Low', 358.2210, CURRENT_DATE, 0.0035, 46700000000.00, 500.00, 500.00, NULL, 6.75, NULL, 'Nikhil Kabra', 'CRISIL Liquid Fund Index', '1998-06-05'),

    -- Two more Bonds-screen entries, tagged with the same 'Debt - Corporate
    -- Bond' category as HDFC-CORPBOND-G above (mf_bonds_collection_screen.dart
    -- fetches that exact category server-side, then re-splits client-side by
    -- name keyword into Government/Corporate/Tax-Free chips) — without these,
    -- the Government and Tax-Free chips had zero funds to show.
    ('SBI-GILT-G',     'SBI Government Securities Fund',           'SBI Mutual Fund',                        'INF200K01BN4', 'Debt - Corporate Bond', 'Low', 45.1230, CURRENT_DATE, 0.0055, 6800000000.00, 5000.00, 500.00, NULL, 6.90, NULL, 'Dinesh Ahuja', 'CRISIL Dynamic Gilt Index', '2005-12-30'),
    ('ICICI-TFB-G',    'ICICI Pru Long Term Tax Free Bond Fund',   'ICICI Prudential Mutual Fund',            'INF109K01BO5', 'Debt - Corporate Bond', 'Low', 38.7650, CURRENT_DATE, 0.0045, 3200000000.00, 5000.00, 500.00, NULL, 6.50, NULL, 'Rahul Goswami', 'CRISIL Long Term Bond Index', '2013-03-18'),

    -- Gold / Silver
    ('NIPPON-GOLD-G', 'Nippon India Gold Savings Fund',            'Nippon India Mutual Fund',                'INF204K01AX4', 'Other - Gold', 'Medium', 24.6610, CURRENT_DATE, 0.0059, 2800000000.00, 1000.00, 100.00, NULL, 14.20, NULL, 'Sanjay Doshi', 'Domestic Price of Gold', '2011-03-07'),
    ('ICICI-SILVER-G','ICICI Silver ETF Fund of Fund',             'ICICI Prudential Mutual Fund',           'INF109KB1WQ4', 'Other - Silver', 'High', 15.6234, CURRENT_DATE, 0.4400, 980000000.00, 1000.00, 100.00, NULL, 16.80, NULL, 'Manish Banthia', 'Domestic Price of Silver', '2022-01-25'),

    -- REIT (frontend mock names exactly one; a second, retail-flavored REIT
    -- is added below purely so mf_reits_collection_screen.dart's 'Retail'
    -- chip isn't empty next to 'Commercial')
    ('EMBASSY-REIT-G','Embassy Office Parks REIT',                 'Embassy Office Parks Management Services','INF1B7Y07020', 'Other - REIT', 'Medium', 342.5000, CURRENT_DATE, 0.0000, 340000000000.00, 10000.00, 10000.00, 8.45, NULL, NULL, NULL, 'Nifty REITs & InvITs Index', '2019-03-27'),
    ('NEXUS-REIT-G',  'Nexus Select Mall REIT',                    'Nexus Select Mall Management',            'INE0GSK23011', 'Other - REIT', 'Medium', 128.4000, CURRENT_DATE, 0.0000, 180000000000.00, 10000.00, 10000.00, 7.80, NULL, NULL, NULL, 'Nifty REITs & InvITs Index', '2023-05-19'),

    -- InvIT (frontend mock names exactly one; a second, roads-flavored InvIT
    -- is added below purely so mf_invits_collection_screen.dart's 'Roads'
    -- chip isn't empty next to 'Power')
    ('POWERGRID-INVIT-G','PowerGrid Infra INVIT',                  'PowerGrid InvIT Investment Manager',      'INE0DK801022', 'Other - InvIT', 'Medium', 101.8000, CURRENT_DATE, 0.0000, 76000000000.00, 10000.00, 10000.00, 9.50, NULL, NULL, NULL, 'Nifty REITs & InvITs Index', '2021-05-14'),
    ('IRB-INVIT-G',   'IRB National Highways InvIT',                'IRB Infrastructure Investment Manager',   'INE0BWF23011', 'Other - InvIT', 'Medium', 68.2000, CURRENT_DATE, 0.0000, 42000000000.00, 10000.00, 10000.00, 10.20, NULL, NULL, NULL, 'Nifty REITs & InvITs Index', '2017-05-16'),

    -- Fixed Deposit
    ('BAJAJ-FD-G',    'Bajaj Finance Fixed Deposit',                'Bajaj Finance Limited',                   'INE296A07TS6', 'Other - FD', 'Low', 1000.0000, CURRENT_DATE, 0.0000, 0.00, 25000.00, 25000.00, 8.10, NULL, NULL, NULL, NULL, '2024-01-01')
ON CONFLICT (scheme_code) DO NOTHING;

-- fund_allocation is keyed on scheme_code with ON DELETE CASCADE, so the
-- DELETE FROM fund_catalog above wiped every allocation row along with it —
-- none of the scheme_codes inserted above had a matching allocation row
-- afterwards, leaving every fund profile page's equity/debt/other split,
-- sector exposure and top-holdings sections empty. Re-seeding one row per
-- fund here so no fund in the catalog is left without this data.
INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
    -- Large Cap
    ('MIRAE-LC-G', 97.6, 0.8, 1.6,
     '[{"title":"Financial Services","percentage":32.4},{"title":"Technology","percentage":14.8},{"title":"Energy","percentage":10.2},{"title":"Consumer Staples","percentage":8.6},{"title":"Others","percentage":34.0}]',
     '[{"title":"HDFC Bank","percentage":9.1},{"title":"ICICI Bank","percentage":7.8},{"title":"Reliance Industries","percentage":6.4},{"title":"Infosys","percentage":5.2}]'),
    ('HDFC-T100-G', 97.2, 1.1, 1.7,
     '[{"title":"Financial Services","percentage":30.1},{"title":"Energy","percentage":12.4},{"title":"Technology","percentage":11.6},{"title":"Automobiles","percentage":8.9},{"title":"Others","percentage":37.0}]',
     '[{"title":"HDFC Bank","percentage":8.6},{"title":"ICICI Bank","percentage":7.1},{"title":"Reliance Industries","percentage":6.9},{"title":"L&T","percentage":4.8}]'),
    ('ICICI-BC-G', 98.0, 0.6, 1.4,
     '[{"title":"Financial Services","percentage":33.8},{"title":"Technology","percentage":13.1},{"title":"Consumer Staples","percentage":9.4},{"title":"Healthcare","percentage":7.8},{"title":"Others","percentage":35.9}]',
     '[{"title":"ICICI Bank","percentage":9.4},{"title":"HDFC Bank","percentage":8.2},{"title":"Infosys","percentage":5.6},{"title":"TCS","percentage":4.1}]'),
    ('SBI-BLC-G', 97.8, 0.7, 1.5,
     '[{"title":"Financial Services","percentage":34.6},{"title":"Technology","percentage":14.2},{"title":"Consumer Staples","percentage":11.8},{"title":"Energy","percentage":9.5},{"title":"Others","percentage":29.9}]',
     '[{"title":"HDFC Bank","percentage":8.9},{"title":"ICICI Bank","percentage":7.6},{"title":"Reliance Industries","percentage":6.2},{"title":"Infosys","percentage":5.1}]'),
    ('AXIS-BC-G', 96.9, 1.4, 1.7,
     '[{"title":"Financial Services","percentage":31.2},{"title":"Technology","percentage":12.6},{"title":"Consumer Discretionary","percentage":10.1},{"title":"Healthcare","percentage":8.3},{"title":"Others","percentage":37.8}]',
     '[{"title":"HDFC Bank","percentage":8.4},{"title":"Bajaj Finance","percentage":6.1},{"title":"ICICI Bank","percentage":5.9},{"title":"Infosys","percentage":4.7}]'),

    -- Mid Cap
    ('QUANT-MC-G', 95.4, 1.9, 2.7,
     '[{"title":"Industrials","percentage":24.6},{"title":"Financial Services","percentage":17.2},{"title":"Consumer Discretionary","percentage":14.8},{"title":"Materials","percentage":11.1},{"title":"Others","percentage":32.3}]',
     '[{"title":"Cummins India","percentage":4.1},{"title":"KEI Industries","percentage":3.6},{"title":"Persistent Systems","percentage":3.2},{"title":"Trent Ltd","percentage":2.9}]'),
    ('NIPPON-GR-G', 96.1, 1.5, 2.4,
     '[{"title":"Industrials","percentage":21.4},{"title":"Financial Services","percentage":18.6},{"title":"Healthcare","percentage":12.3},{"title":"Consumer Discretionary","percentage":10.7},{"title":"Others","percentage":37.0}]',
     '[{"title":"Max Healthcare","percentage":3.8},{"title":"Coforge Ltd","percentage":3.4},{"title":"Bharat Forge","percentage":3.0},{"title":"Voltas Ltd","percentage":2.7}]'),
    ('HDFC-MC-G', 96.5, 1.5, 2.0,
     '[{"title":"Financial Services","percentage":24.1},{"title":"Industrials","percentage":16.8},{"title":"Consumer Discretionary","percentage":13.4},{"title":"Healthcare","percentage":10.2},{"title":"Others","percentage":35.5}]',
     '[{"title":"Coforge Ltd","percentage":4.2},{"title":"Persistent Systems","percentage":3.8},{"title":"Cummins India","percentage":3.5},{"title":"Max Healthcare","percentage":3.1}]'),
    ('MOTILAL-MC-G', 95.8, 1.7, 2.5,
     '[{"title":"Financial Services","percentage":19.8},{"title":"Industrials","percentage":18.1},{"title":"Consumer Discretionary","percentage":15.6},{"title":"Technology","percentage":9.4},{"title":"Others","percentage":37.1}]',
     '[{"title":"Trent Ltd","percentage":4.6},{"title":"Polycab India","percentage":3.9},{"title":"Persistent Systems","percentage":3.3},{"title":"Voltas Ltd","percentage":2.8}]'),
    ('EDEL-MC-G', 95.0, 2.1, 2.9,
     '[{"title":"Industrials","percentage":20.9},{"title":"Financial Services","percentage":17.4},{"title":"Materials","percentage":13.2},{"title":"Consumer Discretionary","percentage":11.5},{"title":"Others","percentage":37.0}]',
     '[{"title":"Bharat Forge","percentage":3.7},{"title":"KEI Industries","percentage":3.2},{"title":"Cummins India","percentage":3.0},{"title":"Trent Ltd","percentage":2.6}]'),

    -- Small Cap
    ('NIPPON-SC-G', 94.8, 2.0, 3.2,
     '[{"title":"Industrials","percentage":22.6},{"title":"Consumer Discretionary","percentage":17.9},{"title":"Materials","percentage":13.4},{"title":"Financial Services","percentage":10.8},{"title":"Others","percentage":35.3}]',
     '[{"title":"PG Electroplast","percentage":3.3},{"title":"Century Plyboards","percentage":2.9},{"title":"Blue Star","percentage":2.7},{"title":"KEI Industries","percentage":2.4}]'),
    ('SBI-SC-G', 95.1, 1.8, 3.1,
     '[{"title":"Industrials","percentage":21.3},{"title":"Consumer Discretionary","percentage":16.4},{"title":"Financial Services","percentage":12.6},{"title":"Materials","percentage":11.2},{"title":"Others","percentage":38.5}]',
     '[{"title":"Blue Star","percentage":3.1},{"title":"Century Plyboards","percentage":2.8},{"title":"PG Electroplast","percentage":2.6},{"title":"KEI Industries","percentage":2.3}]'),
    ('AXIS-SC-G', 95.2, 1.8, 3.0,
     '[{"title":"Industrials","percentage":22.4},{"title":"Consumer Discretionary","percentage":18.1},{"title":"Financial Services","percentage":15.6},{"title":"Materials","percentage":12.3},{"title":"Others","percentage":31.6}]',
     '[{"title":"KEI Industries","percentage":3.4},{"title":"PG Electroplast","percentage":3.1},{"title":"Blue Star","percentage":2.9},{"title":"Century Plyboards","percentage":2.6}]'),
    ('DSP-SC-G', 94.3, 2.2, 3.5,
     '[{"title":"Industrials","percentage":20.7},{"title":"Materials","percentage":15.8},{"title":"Consumer Discretionary","percentage":14.1},{"title":"Financial Services","percentage":11.9},{"title":"Others","percentage":37.5}]',
     '[{"title":"Century Plyboards","percentage":3.0},{"title":"Blue Star","percentage":2.7},{"title":"PG Electroplast","percentage":2.5},{"title":"Bharat Forge","percentage":2.2}]'),
    ('QUANT-SC-G', 94.6, 2.0, 3.4,
     '[{"title":"Industrials","percentage":23.1},{"title":"Materials","percentage":14.6},{"title":"Consumer Discretionary","percentage":13.8},{"title":"Energy","percentage":9.7},{"title":"Others","percentage":38.8}]',
     '[{"title":"Suzlon Energy","percentage":3.2},{"title":"BSE Ltd","percentage":2.9},{"title":"PG Electroplast","percentage":2.6},{"title":"KEI Industries","percentage":2.3}]'),

    -- Flexi Cap
    ('PARAG-FLX-G', 90.1, 3.4, 6.5,
     '[{"title":"Technology","percentage":19.6},{"title":"Financial Services","percentage":17.2},{"title":"Consumer Staples","percentage":11.8},{"title":"Energy","percentage":8.9},{"title":"Others","percentage":42.6}]',
     '[{"title":"HDFC Bank","percentage":7.1},{"title":"Bajaj Holdings","percentage":6.4},{"title":"ITC Ltd","percentage":4.8},{"title":"Alphabet Inc (Global)","percentage":3.2}]'),

    -- Value
    ('QUANT-VAL-G', 96.4, 1.4, 2.2,
     '[{"title":"Financial Services","percentage":26.3},{"title":"Energy","percentage":15.8},{"title":"Materials","percentage":12.4},{"title":"Technology","percentage":9.6},{"title":"Others","percentage":35.9}]',
     '[{"title":"Reliance Industries","percentage":6.8},{"title":"SBI","percentage":5.4},{"title":"ONGC","percentage":4.1},{"title":"Coal India","percentage":3.6}]'),
    ('AXIS-VAL-G', 96.8, 1.2, 2.0,
     '[{"title":"Financial Services","percentage":28.7},{"title":"Energy","percentage":13.2},{"title":"Consumer Staples","percentage":10.9},{"title":"Technology","percentage":8.8},{"title":"Others","percentage":38.4}]',
     '[{"title":"HDFC Bank","percentage":6.4},{"title":"Reliance Industries","percentage":5.7},{"title":"ITC Ltd","percentage":4.3},{"title":"SBI","percentage":3.9}]'),
    ('HSBC-VAL-G', 95.9, 1.6, 2.5,
     '[{"title":"Financial Services","percentage":27.1},{"title":"Energy","percentage":14.6},{"title":"Materials","percentage":11.3},{"title":"Automobiles","percentage":8.4},{"title":"Others","percentage":38.6}]',
     '[{"title":"ICICI Bank","percentage":5.9},{"title":"Reliance Industries","percentage":5.2},{"title":"NTPC","percentage":3.8},{"title":"Tata Steel","percentage":3.3}]'),
    ('DSP-VAL-G', 96.2, 1.4, 2.4,
     '[{"title":"Financial Services","percentage":25.8},{"title":"Energy","percentage":13.9},{"title":"Consumer Staples","percentage":10.4},{"title":"Materials","percentage":9.7},{"title":"Others","percentage":40.2}]',
     '[{"title":"SBI","percentage":5.6},{"title":"Reliance Industries","percentage":5.0},{"title":"ITC Ltd","percentage":3.9},{"title":"Coal India","percentage":3.2}]'),
    ('LIC-VAL-G', 95.5, 1.8, 2.7,
     '[{"title":"Financial Services","percentage":24.9},{"title":"Energy","percentage":16.1},{"title":"Materials","percentage":12.8},{"title":"Utilities","percentage":8.2},{"title":"Others","percentage":38.0}]',
     '[{"title":"NTPC","percentage":4.8},{"title":"Coal India","percentage":4.1},{"title":"SBI","percentage":3.7},{"title":"ONGC","percentage":3.2}]'),
    ('HDFC-VAL-G', 96.7, 1.3, 2.0,
     '[{"title":"Financial Services","percentage":27.6},{"title":"Energy","percentage":14.2},{"title":"Technology","percentage":9.8},{"title":"Automobiles","percentage":8.1},{"title":"Others","percentage":40.3}]',
     '[{"title":"HDFC Bank","percentage":6.1},{"title":"Reliance Industries","percentage":5.4},{"title":"Infosys","percentage":3.9},{"title":"Tata Motors","percentage":3.3}]'),
    ('ABSL-VAL-G', 95.8, 1.7, 2.5,
     '[{"title":"Financial Services","percentage":26.4},{"title":"Energy","percentage":15.3},{"title":"Materials","percentage":10.6},{"title":"Consumer Discretionary","percentage":8.9},{"title":"Others","percentage":38.8}]',
     '[{"title":"ICICI Bank","percentage":5.8},{"title":"Reliance Industries","percentage":5.1},{"title":"Tata Steel","percentage":3.6},{"title":"SBI","percentage":3.1}]'),

    -- Thematic / Sectoral
    ('ICICI-TECH-G', 97.4, 0.6, 2.0,
     '[{"title":"Technology","percentage":68.9},{"title":"Communication Services","percentage":14.2},{"title":"Others","percentage":16.9}]',
     '[{"title":"Infosys","percentage":9.8},{"title":"TCS","percentage":8.6},{"title":"Persistent Systems","percentage":6.1},{"title":"Coforge Ltd","percentage":5.4}]'),
    ('ICICI-MANUF-G', 97.8, 0.5, 1.7,
     '[{"title":"Automobiles","percentage":26.4},{"title":"Industrials","percentage":22.4},{"title":"Materials","percentage":18.7},{"title":"Others","percentage":32.5}]',
     '[{"title":"Larsen & Toubro","percentage":6.8},{"title":"Tata Motors","percentage":5.9},{"title":"Bharat Forge","percentage":4.6},{"title":"Cummins India","percentage":3.9}]'),
    ('MIRAE-SEMI-G', 98.1, 0.3, 1.6,
     '[{"title":"Technology","percentage":72.6},{"title":"Communication Services","percentage":12.4},{"title":"Others","percentage":15.0}]',
     '[{"title":"NVIDIA Corp (Global)","percentage":11.2},{"title":"Taiwan Semiconductor (Global)","percentage":9.4},{"title":"ASML Holding (Global)","percentage":7.1},{"title":"AMD (Global)","percentage":5.8}]'),
    ('SBI-HC-G', 96.9, 0.8, 2.3,
     '[{"title":"Healthcare","percentage":74.1},{"title":"Pharmaceuticals","percentage":15.8},{"title":"Others","percentage":10.1}]',
     '[{"title":"Sun Pharma","percentage":8.9},{"title":"Dr Reddy''s Labs","percentage":6.7},{"title":"Cipla","percentage":5.4},{"title":"Max Healthcare","percentage":4.6}]'),
    ('TATA-GREEN-G', 95.4, 1.6, 3.0,
     '[{"title":"Utilities","percentage":38.6},{"title":"Energy","percentage":24.1},{"title":"Industrials","percentage":16.2},{"title":"Others","percentage":21.1}]',
     '[{"title":"NTPC Ltd","percentage":6.9},{"title":"Adani Green Energy","percentage":5.8},{"title":"Tata Power","percentage":5.1},{"title":"Waaree Energies","percentage":4.3}]'),
    ('MIRAE-EVMOB-G', 96.1, 1.0, 2.9,
     '[{"title":"Automobiles","percentage":68.9},{"title":"Auto Components","percentage":22.4},{"title":"Others","percentage":8.7}]',
     '[{"title":"Tata Motors","percentage":11.8},{"title":"Mahindra & Mahindra","percentage":9.6},{"title":"Bajaj Auto","percentage":7.4},{"title":"TVS Motor","percentage":5.2}]'),

    -- Global
    ('MOTILAL-N100-G', 97.9, 0.4, 1.7,
     '[{"title":"Technology","percentage":58.4},{"title":"Consumer Discretionary","percentage":16.8},{"title":"Communication Services","percentage":11.2},{"title":"Others","percentage":13.6}]',
     '[{"title":"Apple Inc (Global)","percentage":9.8},{"title":"Microsoft Corp (Global)","percentage":9.1},{"title":"NVIDIA Corp (Global)","percentage":8.4},{"title":"Amazon.com (Global)","percentage":5.6}]'),
    ('NIPPON-JP-G', 96.8, 0.9, 2.3,
     '[{"title":"Industrials","percentage":24.6},{"title":"Consumer Discretionary","percentage":18.9},{"title":"Technology","percentage":14.2},{"title":"Others","percentage":42.3}]',
     '[{"title":"Toyota Motor (Global)","percentage":6.4},{"title":"Sony Group (Global)","percentage":5.1},{"title":"Mitsubishi UFJ (Global)","percentage":4.2},{"title":"Keyence Corp (Global)","percentage":3.6}]'),
    ('INVESCO-EU-G', 95.6, 1.5, 2.9,
     '[{"title":"Financial Services","percentage":21.4},{"title":"Healthcare","percentage":16.8},{"title":"Consumer Staples","percentage":13.1},{"title":"Others","percentage":48.7}]',
     '[{"title":"ASML Holding (Global)","percentage":5.9},{"title":"Nestle SA (Global)","percentage":4.6},{"title":"LVMH (Global)","percentage":4.1},{"title":"SAP SE (Global)","percentage":3.4}]'),

    -- Debt / Liquid / Gilt / Tax-Free
    ('HDFC-CORPBOND-G', 0.0, 97.8, 2.2,
     '[{"title":"AAA Corporate Bonds","percentage":68.4},{"title":"AA+ Corporate Bonds","percentage":21.6},{"title":"Cash & Equivalents","percentage":10.0}]',
     '[{"title":"HDFC Ltd NCD","percentage":8.6},{"title":"REC Ltd NCD","percentage":7.1},{"title":"NABARD NCD","percentage":6.4},{"title":"Power Finance Corp NCD","percentage":5.8}]'),
    ('SBI-STD-G', 0.0, 98.1, 1.9,
     '[{"title":"Corporate Bonds","percentage":54.6},{"title":"Certificate of Deposit","percentage":28.4},{"title":"Cash & Equivalents","percentage":17.0}]',
     '[{"title":"SBI CD","percentage":9.2},{"title":"HDFC Bank CD","percentage":7.8},{"title":"NABARD NCD","percentage":6.1},{"title":"LIC Housing Finance NCD","percentage":5.3}]'),
    ('ICICI-LIQ-G', 0.0, 98.9, 1.1,
     '[{"title":"Treasury Bills","percentage":38.6},{"title":"Commercial Paper","percentage":36.2},{"title":"Certificate of Deposit","percentage":25.2}]',
     '[{"title":"91-Day T-Bill","percentage":16.8},{"title":"HDFC Ltd CP","percentage":9.4},{"title":"SBI CD","percentage":8.6},{"title":"NABARD CP","percentage":7.1}]'),
    ('SBI-GILT-G', 0.0, 98.6, 1.4,
     '[{"title":"Government Securities","percentage":91.4},{"title":"Cash & Equivalents","percentage":8.6}]',
     '[{"title":"7.18% GOI 2033","percentage":24.6},{"title":"7.26% GOI 2032","percentage":18.9},{"title":"7.10% GOI 2034","percentage":14.2},{"title":"State Development Loan","percentage":11.8}]'),
    ('ICICI-TFB-G', 0.0, 97.4, 2.6,
     '[{"title":"Tax-Free Bonds (AAA)","percentage":82.6},{"title":"Cash & Equivalents","percentage":17.4}]',
     '[{"title":"NHAI Tax Free Bond","percentage":22.4},{"title":"IRFC Tax Free Bond","percentage":19.6},{"title":"PFC Tax Free Bond","percentage":16.1},{"title":"HUDCO Tax Free Bond","percentage":12.8}]'),

    -- Gold / Silver
    ('NIPPON-GOLD-G', 0.0, 0.6, 99.4,
     '[{"title":"Gold ETF Units","percentage":99.4},{"title":"Cash & Equivalents","percentage":0.6}]',
     '[{"title":"Nippon India Gold ETF","percentage":99.4}]'),
    ('ICICI-SILVER-G', 0.0, 0.4, 99.6,
     '[{"title":"Silver ETF Units","percentage":99.6},{"title":"Cash & Equivalents","percentage":0.4}]',
     '[{"title":"ICICI Prudential Silver ETF","percentage":99.6}]'),

    -- REIT / InvIT
    ('EMBASSY-REIT-G', 0.0, 8.4, 91.6,
     '[{"title":"Commercial Office Space","percentage":86.2},{"title":"Hospitality","percentage":5.4},{"title":"Cash & Equivalents","percentage":8.4}]',
     '[{"title":"Embassy Manyata Business Park","percentage":22.6},{"title":"Embassy TechVillage","percentage":18.4},{"title":"Embassy Golf Links","percentage":14.1},{"title":"FIFC Mumbai","percentage":9.8}]'),
    ('NEXUS-REIT-G', 0.0, 6.9, 93.1,
     '[{"title":"Retail Malls","percentage":88.7},{"title":"Cash & Equivalents","percentage":11.3}]',
     '[{"title":"Nexus Elante Mall Chandigarh","percentage":16.4},{"title":"Nexus Seawoods Mall Mumbai","percentage":13.8},{"title":"Nexus Shantiniketan Bangalore","percentage":11.2},{"title":"Nexus Ahmedabad One Mall","percentage":9.6}]'),
    ('POWERGRID-INVIT-G', 0.0, 9.6, 90.4,
     '[{"title":"Power Transmission Assets","percentage":92.1},{"title":"Cash & Equivalents","percentage":7.9}]',
     '[{"title":"Warora Transmission Project","percentage":20.4},{"title":"Kala Amb Transmission Project","percentage":17.6},{"title":"Vizag Transmission Project","percentage":14.9},{"title":"Jabalpur Transmission Project","percentage":12.1}]'),
    ('IRB-INVIT-G', 0.0, 11.2, 88.8,
     '[{"title":"Toll Road Assets","percentage":89.6},{"title":"Cash & Equivalents","percentage":10.4}]',
     '[{"title":"Surat-Dahisar Highway","percentage":19.8},{"title":"Jaipur-Deoli Highway","percentage":16.4},{"title":"Talegaon-Amravati Highway","percentage":13.7},{"title":"Kaithal-Rajasthan Border Highway","percentage":11.2}]'),

    -- Fixed Deposit
    ('BAJAJ-FD-G', 0.0, 0.0, 100.0,
     '[{"title":"Corporate Fixed Deposit","percentage":100.0}]',
     '[{"title":"Bajaj Finance Fixed Deposit","percentage":100.0}]')
ON CONFLICT (scheme_code) DO NOTHING;

-- security_reference: add the frontend stock-collection mock's remaining
-- symbols (mf_stocks_collection_screen.dart) — Reliance/TCS/HDFC Bank
-- already exist here. Kept alongside the pre-existing INFY/ICICIBANK/
-- TATAMOTORS (still priced by the separate stocks mock provider) rather
-- than removing them, since this pass is about matching names, not
-- narrowing what's tradeable.
INSERT INTO security_reference (symbol, sector, market_cap_band) VALUES
    ('TRENT',    'Consumer Cyclical',   'Mid Cap'),
    ('TVSMOTOR', 'Automobiles',         'Mid Cap'),
    ('SUZLON',   'Energy',              'Small Cap'),
    ('BSE',      'Financial Services',  'Small Cap')
ON CONFLICT (symbol) DO NOTHING;
