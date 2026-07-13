-- M6: scheduled publish, draft preview tokens, slug redirects.

ALTER TABLE articles ADD COLUMN publish_at TIMESTAMPTZ NULL;
ALTER TABLE articles ADD COLUMN preview_token TEXT NOT NULL DEFAULT '';

CREATE INDEX articles_publish_at_idx ON articles (publish_at)
    WHERE publish_at IS NOT NULL AND status = 'draft';

CREATE UNIQUE INDEX articles_preview_token_uidx ON articles (preview_token)
    WHERE preview_token <> '';

CREATE TABLE redirects (
    id         BIGSERIAL PRIMARY KEY,
    from_path  TEXT NOT NULL UNIQUE,
    to_path    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
