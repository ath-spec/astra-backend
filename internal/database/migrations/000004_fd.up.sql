CREATE TABLE IF NOT EXISTS fd_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fd_account_number VARCHAR(20) NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_account_id UUID NOT NULL REFERENCES bank_accounts(id),
    principal_amount NUMERIC(18,2) NOT NULL,
    interest_rate NUMERIC(5,2) NOT NULL,
    tenure_months INTEGER NOT NULL,
    interest_payout VARCHAR(15) NOT NULL,
    auto_renewal BOOLEAN NOT NULL DEFAULT false,
    nominee_name VARCHAR(100),
    booking_date DATE NOT NULL DEFAULT CURRENT_DATE,
    maturity_date DATE NOT NULL,
    maturity_amount NUMERIC(18,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fd_accounts_user_id ON fd_accounts(user_id);
