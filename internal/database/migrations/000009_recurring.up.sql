ALTER TABLE mandates ADD COLUMN IF NOT EXISTS category VARCHAR(30) NOT NULL DEFAULT 'OTHER';

CREATE TABLE IF NOT EXISTS mandate_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mandate_id VARCHAR(30) NOT NULL REFERENCES mandates(mandate_id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scheduled_date DATE NOT NULL,
    amount NUMERIC(18,2) NOT NULL,
    status VARCHAR(15) NOT NULL,
    failure_reason VARCHAR(150),
    executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mandate_executions_user_id ON mandate_executions(user_id);
CREATE INDEX IF NOT EXISTS idx_mandate_executions_mandate_id ON mandate_executions(mandate_id);
