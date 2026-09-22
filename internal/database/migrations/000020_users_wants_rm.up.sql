-- Advisory opt-in captured on the user signup form. A Relationship Manager is
-- auto-assigned only when this is true; users who decline stay unassigned and
-- are never routed into an RM's book.
ALTER TABLE users ADD COLUMN IF NOT EXISTS wants_rm BOOLEAN NOT NULL DEFAULT false;

-- Any user who already has an RM must, by definition, have wanted one.
UPDATE users SET wants_rm = true WHERE assigned_rm_id IS NOT NULL;
