-- Migration 000038 only ever populated one return figure per fund (whichever
-- the frontend mock data exposed — returns_3y for most funds, returns_1y for
-- REIT/InvIT dividend-style yields), leaving the other periods NULL. Every
-- collection screen defaults to a specific period (mostly '1Y', a couple to
-- '3Y') and renders a plain dash when that period is NULL — e.g. the Gold
-- screen defaults to '1Y', which was NULL for both gold funds, so every gold
-- fund showed "—" instead of a return. Backfilling the missing periods for
-- every fund so no period renders blank regardless of a screen's default.
-- A few very recently launched funds (Mirae Asset Semiconductor & AI,
-- Mirae Asset EV & Mobility, Nexus Select Mall REIT) are intentionally left
-- without a 5Y figure — they haven't existed five years, so NULL there is
-- correct, not a gap.
UPDATE fund_catalog AS f SET
    returns_1y = COALESCE(f.returns_1y, v.r1),
    returns_3y = COALESCE(f.returns_3y, v.r3),
    returns_5y = COALESCE(f.returns_5y, v.r5)
FROM (VALUES
    -- Large Cap
    ('MIRAE-LC-G',    19.40::numeric, NULL::numeric, 15.60::numeric),
    ('HDFC-T100-G',   18.60, NULL, 14.80),
    ('ICICI-BC-G',    17.90, NULL, 14.10),
    ('SBI-BLC-G',     17.20, NULL, 13.60),
    ('AXIS-BC-G',     15.80, NULL, 12.70),
    -- Mid Cap
    ('QUANT-MC-G',    41.20, NULL, 27.10),
    ('NIPPON-GR-G',   36.80, NULL, 24.30),
    ('HDFC-MC-G',     34.50, NULL, 22.80),
    ('MOTILAL-MC-G',  32.10, NULL, 21.40),
    ('EDEL-MC-G',     30.20, NULL, 20.10),
    -- Small Cap
    ('NIPPON-SC-G',   48.90, NULL, 31.20),
    ('SBI-SC-G',      43.60, NULL, 28.40),
    ('AXIS-SC-G',     37.90, NULL, 24.60),
    ('DSP-SC-G',      35.10, NULL, 22.90),
    ('QUANT-SC-G',    27.80, NULL, 18.30),
    -- Flexi Cap
    ('PARAG-FLX-G',   26.90, NULL, 19.80),
    -- Value
    ('QUANT-VAL-G',   28.40, NULL, 19.10),
    ('AXIS-VAL-G',    23.60, NULL, 16.40),
    ('HSBC-VAL-G',    23.10, NULL, 16.10),
    ('DSP-VAL-G',     21.90, NULL, 15.60),
    ('LIC-VAL-G',     21.80, NULL, 15.40),
    ('HDFC-VAL-G',    22.10, NULL, 15.70),
    ('ABSL-VAL-G',    21.60, NULL, 15.50),
    -- Thematic (5Y intentionally left NULL for the two youngest funds)
    ('ICICI-TECH-G',  39.80, NULL, 26.90),
    ('ICICI-MANUF-G', 33.60, NULL, 22.10),
    ('MIRAE-SEMI-G',  51.40, NULL, NULL),
    ('SBI-HC-G',      23.90, NULL, 17.20),
    ('TATA-GREEN-G',  24.60, NULL, 16.80),
    ('MIRAE-EVMOB-G', 21.90, NULL, NULL),
    -- Global
    ('MOTILAL-N100-G',31.60, NULL, 22.40),
    ('NIPPON-JP-G',   19.80, NULL, 14.60),
    ('INVESCO-EU-G',  -2.10, NULL, 3.80),
    -- Debt
    ('HDFC-CORPBOND-G', 7.65, NULL, 6.95),
    ('SBI-STD-G',        7.40, NULL, 6.80),
    ('ICICI-LIQ-G',      7.05, NULL, 6.50),
    ('SBI-GILT-G',       7.20, NULL, 6.60),
    ('ICICI-TFB-G',      6.80, NULL, 6.30),
    -- Gold / Silver
    ('NIPPON-GOLD-G',  24.60, NULL, 12.80),
    ('ICICI-SILVER-G', 28.90, NULL, 13.40),
    -- Index fund
    ('UTI-N50-G',      17.80, NULL, 13.60),
    -- REIT / InvIT (returns_1y already set; backfilling 3Y/5Y — 5Y left
    -- NULL for Nexus Select Mall REIT, listed in 2023)
    ('EMBASSY-REIT-G',    NULL, 9.60, 10.80),
    ('NEXUS-REIT-G',      NULL, 8.90, NULL),
    ('POWERGRID-INVIT-G', NULL, 10.60, 11.40),
    ('IRB-INVIT-G',       NULL, 11.30, 12.10),
    -- Fixed Deposit — a flat contracted rate applies across every period
    ('BAJAJ-FD-G',        NULL, 8.10, 8.10)
) AS v(scheme_code, r1, r3, r5)
WHERE f.scheme_code = v.scheme_code;
