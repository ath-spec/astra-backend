DROP INDEX IF EXISTS idx_bank_accounts_user_active;
ALTER TABLE bank_accounts DROP COLUMN IF EXISTS unlinked_at;
