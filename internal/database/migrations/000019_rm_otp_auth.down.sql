DROP TABLE IF EXISTS rm_otp_codes;
ALTER TABLE rm_users DROP CONSTRAINT IF EXISTS rm_users_employee_code_key;
ALTER TABLE rm_users DROP COLUMN IF EXISTS employee_code;
ALTER TABLE rm_users ALTER COLUMN password_hash SET NOT NULL;
