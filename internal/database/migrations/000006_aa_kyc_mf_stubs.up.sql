-- Tables for domains that are routed but not yet implemented (see
-- internal/handler/{aa,kyc,mf}_handler.go, which currently return 501).
-- These depend on picking an external provider (an AA gateway, a KYC/PAN
-- verification vendor, a CAMS/KFintech-style RTA for MF Central) before real
-- logic makes sense; the schema is created now so wiring a provider later is
-- additive, not a migration event.

CREATE TABLE IF NOT EXISTS aa_bank_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id VARCHAR(40) NOT NULL,
    bank_name VARCHAR(100) NOT NULL,
    account_number_masked VARCHAR(20) NOT NULL,
    ifsc VARCHAR(11),
    account_type VARCHAR(20) NOT NULL,
    branch VARCHAR(100),
    current_balance NUMERIC(18,2),
    balance_as_of TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS aa_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    txn_id VARCHAR(40) NOT NULL,
    account_id UUID NOT NULL REFERENCES aa_bank_accounts(id) ON DELETE CASCADE,
    amount NUMERIC(18,2) NOT NULL,
    type VARCHAR(6) NOT NULL,
    narration VARCHAR(200),
    transaction_datetime TIMESTAMPTZ NOT NULL,
    reference_number VARCHAR(30),
    mode VARCHAR(15),
    balance_after_txn NUMERIC(18,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kyc_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pan_number VARCHAR(10) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    dob DATE NOT NULL,
    pan_status VARCHAR(15),
    registered_name VARCHAR(100),
    name_match BOOLEAN,
    aadhaar_seeding_status VARCHAR(20),
    registered_address VARCHAR(250),
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mf_folios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    folio_number VARCHAR(20) NOT NULL,
    amc_name VARCHAR(100) NOT NULL,
    scheme_code VARCHAR(20) NOT NULL,
    scheme_name VARCHAR(150) NOT NULL,
    isin VARCHAR(12),
    units_held NUMERIC(15,3) NOT NULL DEFAULT 0,
    nav NUMERIC(10,4),
    nav_date DATE,
    cost_value NUMERIC(18,2),
    category VARCHAR(30),
    plan_type VARCHAR(10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mf_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    folio_id UUID NOT NULL REFERENCES mf_folios(id) ON DELETE CASCADE,
    transaction_type VARCHAR(15) NOT NULL,
    transaction_date DATE NOT NULL,
    amount NUMERIC(18,2) NOT NULL,
    units NUMERIC(15,3) NOT NULL,
    price NUMERIC(10,4) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_aa_bank_accounts_user_id ON aa_bank_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_aa_transactions_account_id ON aa_transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_kyc_verifications_user_id ON kyc_verifications(user_id);
CREATE INDEX IF NOT EXISTS idx_mf_folios_user_id ON mf_folios(user_id);
CREATE INDEX IF NOT EXISTS idx_mf_transactions_folio_id ON mf_transactions(folio_id);
