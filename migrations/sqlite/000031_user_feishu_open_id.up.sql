-- Mirrors versioned migration 000111_user_feishu_open_id.
-- Direct Feishu login binding: NULL means the account never used Feishu.
ALTER TABLE users ADD COLUMN feishu_open_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_feishu_open_id
    ON users (feishu_open_id)
    WHERE feishu_open_id IS NOT NULL AND feishu_open_id <> '';
