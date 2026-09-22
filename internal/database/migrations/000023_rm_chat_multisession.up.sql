-- Allow multiple chat sessions per RM by dropping unique (rm_id, scope) constraint
ALTER TABLE rm_chat_sessions DROP CONSTRAINT IF EXISTS rm_chat_sessions_rm_id_scope_key;

-- Add title column for identifying conversations in session list
ALTER TABLE rm_chat_sessions ADD COLUMN IF NOT EXISTS title VARCHAR(255) NOT NULL DEFAULT 'New Conversation';

-- Index for fast retrieval of an RM's chat history ordered by latest activity
CREATE INDEX IF NOT EXISTS idx_rm_chat_sessions_rm
    ON rm_chat_sessions(rm_id, scope, updated_at DESC);
