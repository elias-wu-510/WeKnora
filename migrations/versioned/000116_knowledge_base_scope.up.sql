-- Migration 000116: knowledge-base visibility scope and explicit members.
--
-- W3 introduces three scopes:
--   public   — every member of the workspace can read the KB (today's behaviour)
--   project  — only kb_members and the owner/creator can read it
--   personal — only the owner can read it
--
-- Existing KBs are backfilled to public so nothing disappears after upgrade.
-- kb_members carries per-KB roles (viewer/contributor/admin) and is the
-- authority for project-scope visibility; organisation shares remain a
-- separate path in access.ResolveKB.

ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'public';
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS owner_user_id VARCHAR(36);

UPDATE knowledge_bases
SET owner_user_id = creator_id
WHERE owner_user_id IS NULL AND creator_id <> '';

CREATE INDEX IF NOT EXISTS idx_knowledge_bases_tenant_scope
    ON knowledge_bases (tenant_id, scope);

CREATE TABLE IF NOT EXISTS kb_members (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    kb_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_members_kb_user_unique
    ON kb_members (kb_id, user_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_kb_members_user
    ON kb_members (user_id) WHERE deleted_at IS NULL;