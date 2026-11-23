ALTER TABLE knowledge_bases ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'public';
ALTER TABLE knowledge_bases ADD COLUMN owner_user_id VARCHAR(36);

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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_members_kb_user_unique
    ON kb_members (kb_id, user_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_kb_members_user
    ON kb_members (user_id) WHERE deleted_at IS NULL;
