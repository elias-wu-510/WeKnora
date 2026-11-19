DROP INDEX IF EXISTS idx_users_feishu_open_id;
ALTER TABLE users DROP COLUMN feishu_open_id;
