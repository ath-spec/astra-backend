-- Unlinking a bank account previously did a hard DELETE, which violates the
-- FK from payments/mandates/fd_accounts (bank_account_id is NOT NULL
-- REFERENCES bank_accounts(id) with no ON DELETE clause, so it defaults to
-- RESTRICT) the moment the account has any transaction history — the delete
-- would fail outright for any real user. Real banking UX doesn't hard-delete
-- an account with history anyway; it delinks/closes it while preserving the
-- audit trail. Soft-delete via unlinked_at achieves both: FK integrity stays
-- intact, and every "list my accounts" / "sum my balance" query just filters
-- it out.
ALTER TABLE bank_accounts ADD COLUMN unlinked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_bank_accounts_user_active
    ON bank_accounts(user_id) WHERE unlinked_at IS NULL;
