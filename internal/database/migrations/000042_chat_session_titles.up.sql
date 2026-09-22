-- chat_sessions previously had no title, and the handler always resolved
-- "the most recent session for this user" rather than a specific one the
-- client asked for — so there was structurally only ever one growing
-- conversation per user, never distinct named threads. Adding a title
-- (auto-derived from the first user message, same truncation the frontend
-- already did locally in chat_session_provider.dart) so a real multi-thread
-- history list can be served from here instead of client-only mock data.
ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS title VARCHAR(255) NOT NULL DEFAULT 'New Chat';
