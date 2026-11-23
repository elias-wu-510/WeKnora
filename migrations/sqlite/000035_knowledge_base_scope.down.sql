DROP INDEX IF EXISTS idx_kb_members_user;
DROP INDEX IF EXISTS idx_kb_members_kb_user_unique;
DROP TABLE IF EXISTS kb_members;
DROP INDEX IF EXISTS idx_knowledge_bases_tenant_scope;
ALTER TABLE knowledge_bases DROP COLUMN owner_user_id;
ALTER TABLE knowledge_bases DROP COLUMN scope;
