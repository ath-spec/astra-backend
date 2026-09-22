DELETE FROM fund_catalog WHERE scheme_code IN
    ('AAPL-US-G', 'MSFT-US-G', 'GOOGL-US-G', 'AMZN-US-G', 'NVDA-US-G', 'META-US-G', 'TSLA-US-G');
-- fund_allocation rows for these scheme_codes are removed automatically via
-- ON DELETE CASCADE.
