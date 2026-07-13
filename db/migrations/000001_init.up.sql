CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT      NOT NULL UNIQUE,
    password_hash TEXT      NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE articles (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT      NOT NULL UNIQUE,
    title        TEXT      NOT NULL,
    type         TEXT      NOT NULL CHECK (type IN ('markdown', 'html_upload')),
    status       TEXT      NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    visibility   TEXT      NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'protected', 'private')),
    password     TEXT      NOT NULL DEFAULT '',
    raw_mode     BOOLEAN   NOT NULL DEFAULT false,
    body         TEXT      NOT NULL DEFAULT '',
    tags         TEXT[]    NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX articles_status_idx     ON articles (status);
CREATE INDEX articles_published_idx  ON articles (published_at DESC);
CREATE INDEX articles_tags_idx       ON articles USING GIN (tags);
