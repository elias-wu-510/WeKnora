-- Reverse of 000117: restore the historical 'public' spelling and default.
-- Visibility semantics are unchanged; only the stored/API value flips back.

ALTER TABLE knowledge_bases ALTER COLUMN scope SET DEFAULT 'public';

UPDATE knowledge_bases SET scope = 'public' WHERE scope = 'tenant';
