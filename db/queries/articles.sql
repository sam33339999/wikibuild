-- name: CreateArticle :one
INSERT INTO articles (
    slug, title, type, status, visibility, password, raw_mode, pinned,
    body, tags, created_at, updated_at, published_at
) VALUES (
    @slug, @title, @type, @status, @visibility, @password, @raw_mode, @pinned,
    @body, @tags,
    COALESCE(sqlc.narg('created_at'), now()),
    COALESCE(sqlc.narg('updated_at'), now()),
    sqlc.narg('published_at')
)
RETURNING *;

-- name: GetArticle :one
SELECT * FROM articles WHERE id = $1;

-- name: GetArticleBySlug :one
SELECT * FROM articles WHERE slug = $1;

-- name: UpdateArticle :one
UPDATE articles SET
    slug         = @slug,
    title        = @title,
    type         = @type,
    status       = @status,
    visibility   = @visibility,
    password     = @password,
    raw_mode     = @raw_mode,
    pinned       = @pinned,
    body         = @body,
    tags         = @tags,
    updated_at   = COALESCE(sqlc.narg('updated_at'), now()),
    published_at = sqlc.narg('published_at')
WHERE id = @id
RETURNING *;

-- name: DeleteArticle :exec
DELETE FROM articles WHERE id = $1;

-- Optional filters: pass NULL (Valid=false) to skip status/visibility/tag,
-- pass '' search to skip full-text search. limit=0 means "no limit".
-- name: ListArticles :many
SELECT * FROM articles
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('visibility')::text IS NULL OR visibility = sqlc.narg('visibility')::text)
  AND (sqlc.narg('tag')::text IS NULL OR sqlc.narg('tag')::text = ANY(tags))
  AND (@search = '' OR title ILIKE '%' || @search || '%' OR body ILIKE '%' || @search || '%')
ORDER BY pinned DESC, id DESC
LIMIT NULLIF(@max_rows, 0) OFFSET @skip;

-- name: CountArticles :one
SELECT count(*) FROM articles
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('visibility')::text IS NULL OR visibility = sqlc.narg('visibility')::text)
  AND (sqlc.narg('tag')::text IS NULL OR sqlc.narg('tag')::text = ANY(tags))
  AND (@search = '' OR title ILIKE '%' || @search || '%' OR body ILIKE '%' || @search || '%');
