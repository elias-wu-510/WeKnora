-- Direct Feishu (Lark) login for C-Knowledge: persist the Feishu open_id that
-- an account is bound to so repeat logins resolve to the same WeKnora user.
-- NULL means "never used Feishu login"; the partial index allows many NULLs
-- while still guaranteeing one account per open_id.
ALTER TABLE users ADD COLUMN feishu_open_id VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_feishu_open_id
    ON users (feishu_open_id)
    WHERE feishu_open_id IS NOT NULL AND feishu_open_id <> '';
