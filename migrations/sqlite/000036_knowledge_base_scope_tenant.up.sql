-- Migration 000036: rename the knowledge-base scope value 'public' -> 'tenant'
-- (SQLite mirror of versioned 000117).
--
-- Why: 'public' always meant "visible to every member of this tenant
-- workspace", never the open internet. The semantics stay tenant/project/
-- personal; only the stored value is renamed.
--
-- SQLite cannot ALTER COLUMN ... SET DEFAULT, so the column default installed
-- by 000035 stays 'public' here. That is harmless: the application always
-- writes an explicit scope at insert time (types.NormalizeScopeInput), and
-- this migration rewrites every existing row. The versioned (PostgreSQL)
-- migration updates the default for callers that omit the column.

UPDATE knowledge_bases SET scope = 'tenant' WHERE scope = 'public';
