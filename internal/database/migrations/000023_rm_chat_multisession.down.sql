DROP INDEX IF EXISTS idx_rm_chat_sessions_rm;
ALTER TABLE rm_chat_sessions DROP COLUMN IF EXISTS title;
ALTER TABLE rm_chat_sessions ADD CONSTRAINT rm_chat_sessions_rm_id_scope_key UNIQUE (rm_id, scope);
