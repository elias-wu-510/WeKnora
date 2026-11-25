-- Migration 000117: rename the knowledge-base scope value 'public' -> 'tenant'.
--
-- Why: 000116 introduced the scope column with three values, but 'public' was
-- only ever meant as "every member of this tenant workspace can read it", not
-- "visible on the open internet". The name invited that misreading, so the
-- canonical value is now 'tenant'. The semantics are unchanged:
--   tenant   — every member of the workspace may read the KB
--   project  — only explicit kb_members (and the owner/creator) may read it
--   personal — only the owner may read it
--
-- Clients built before this migration may still send 'public'; the API
-- normalizes it to 'tenant' and never persists the legacy value again.

UPDATE knowledge_bases SET scope = 'tenant' WHERE scope = 'public';

ALTER TABLE knowledge_bases ALTER COLUMN scope SET DEFAULT 'tenant';
