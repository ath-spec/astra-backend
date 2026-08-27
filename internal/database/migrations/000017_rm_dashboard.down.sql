DROP TABLE IF EXISTS rm_queue_state;
DROP TABLE IF EXISTS rm_assignment_history;
ALTER TABLE users DROP COLUMN IF EXISTS assigned_rm_id;
DROP TABLE IF EXISTS rm_refresh_tokens;
DROP TABLE IF EXISTS rm_users;
