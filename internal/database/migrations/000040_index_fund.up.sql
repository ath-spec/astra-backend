-- The Allocation screen's "Equity Exposure" section computes both the
-- user's own index-fund percentage and the "Investors Like You" peer
-- benchmark (peerIndexFundPct in portfolio_analysis.go) by matching
-- scheme_name/category against '%index%'/'%nifty%'/'%sensex%'. The catalog
-- had zero funds matching that pattern (frontend mock data never included
-- one either), so both numbers always computed to 0% for every user —
-- including the peer benchmark, since every seeded user shares the same
-- Good Investor archetype. Adds one real, common passive index fund so this
-- section has real data to show.
INSERT INTO fund_catalog
    (scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date, expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y, fund_manager, benchmark_index, launch_date)
VALUES
    ('UTI-N50-G', 'UTI Nifty 50 Index Fund', 'UTI Mutual Fund', 'INF789F01XA1', 'Equity - Large Cap', 'Medium', 285.4210, CURRENT_DATE, 0.0021, 18600000000.00, 5000.00, 500.00, NULL, 14.90, NULL, 'Sharwan Kumar Goyal', 'Nifty 50 TRI', '2000-03-06')
ON CONFLICT (scheme_code) DO NOTHING;

INSERT INTO fund_allocation (scheme_code, equity_pct, debt_pct, other_pct, sectors, top_holdings) VALUES
    ('UTI-N50-G', 99.4, 0.0, 0.6,
     '[{"title":"Financial Services","percentage":33.1},{"title":"Technology","percentage":13.4},{"title":"Energy","percentage":11.8},{"title":"Consumer Staples","percentage":8.2},{"title":"Others","percentage":33.5}]',
     '[{"title":"HDFC Bank","percentage":9.8},{"title":"ICICI Bank","percentage":7.4},{"title":"Reliance Industries","percentage":6.9},{"title":"Infosys","percentage":5.1}]')
ON CONFLICT (scheme_code) DO NOTHING;
