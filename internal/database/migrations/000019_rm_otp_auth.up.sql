-- Switch RM/Admin console auth from email+password to OTP: staff log in
-- with their employee code (or email) and a one-time code sent to their
-- registered phone. password_hash is retained but no longer required or
-- used, so this migration is safe to apply on a deployment that already ran
-- 000017 with password rows.

ALTER TABLE rm_users ADD COLUMN IF NOT EXISTS employee_code VARCHAR(50);

-- Backfill any pre-existing rows so the NOT NULL + UNIQUE below can be added.
UPDATE rm_users
SET employee_code = 'EMP-' || upper(substr(replace(id::text, '-', ''), 1, 8))
WHERE employee_code IS NULL OR employee_code = '';

ALTER TABLE rm_users ALTER COLUMN employee_code SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rm_users_employee_code_key') THEN
        ALTER TABLE rm_users ADD CONSTRAINT rm_users_employee_code_key UNIQUE (employee_code);
    END IF;
END $$;

-- password_hash is now optional (OTP is the only login path).
ALTER TABLE rm_users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE rm_users ALTER COLUMN password_hash SET DEFAULT '';

-- One-time login codes. Only the SHA-256 hash of the code is stored; a code
-- is single-use (consumed_at) and capped at a few attempts before it must
-- be re-requested.
CREATE TABLE IF NOT EXISTS rm_otp_codes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rm_id       UUID NOT NULL REFERENCES rm_users(id) ON DELETE CASCADE,
    code_hash   VARCHAR(64) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rm_otp_codes_rm_id ON rm_otp_codes(rm_id, created_at DESC);
