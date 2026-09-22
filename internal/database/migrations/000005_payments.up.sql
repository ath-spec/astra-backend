CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id VARCHAR(40) NOT NULL UNIQUE,
    txn_id VARCHAR(40) NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_account_id UUID NOT NULL REFERENCES bank_accounts(id),
    amount NUMERIC(18,2) NOT NULL,
    payment_mode VARCHAR(15) NOT NULL,
    upi_id VARCHAR(50),
    purpose VARCHAR(20) NOT NULL,
    status VARCHAR(15) NOT NULL DEFAULT 'PENDING',
    mode VARCHAR(15),
    bank_ref_num VARCHAR(30),
    error_code VARCHAR(10),
    error_message VARCHAR(150),
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mandates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mandate_id VARCHAR(30) NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_account_id UUID NOT NULL REFERENCES bank_accounts(id),
    mandate_type VARCHAR(20) NOT NULL DEFAULT 'UPI_AUTOPAY',
    upi_id VARCHAR(50),
    payee_name VARCHAR(100),
    payee_vpa_or_id VARCHAR(50),
    max_amount NUMERIC(18,2) NOT NULL,
    frequency VARCHAR(10) NOT NULL,
    mandate_start_date DATE NOT NULL,
    mandate_end_date DATE,
    next_debit_date DATE,
    status VARCHAR(15) NOT NULL DEFAULT 'PENDING',
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_mandates_user_id ON mandates(user_id);
