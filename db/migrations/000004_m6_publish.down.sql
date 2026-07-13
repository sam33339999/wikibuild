DROP TABLE IF EXISTS redirects;
DROP INDEX IF EXISTS articles_preview_token_uidx;
DROP INDEX IF EXISTS articles_publish_at_idx;
ALTER TABLE articles DROP COLUMN IF EXISTS preview_token;
ALTER TABLE articles DROP COLUMN IF EXISTS publish_at;
