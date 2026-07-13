-- name: CreateRedirect :one
INSERT INTO redirects (from_path, to_path, created_at)
VALUES (
    @from_path, @to_path,
    COALESCE(sqlc.narg('created_at'), now())
)
ON CONFLICT (from_path) DO UPDATE SET
    to_path = EXCLUDED.to_path
RETURNING *;

-- name: GetRedirect :one
SELECT * FROM redirects WHERE from_path = $1;

-- name: ListRedirects :many
SELECT * FROM redirects ORDER BY id DESC;

-- name: DeleteRedirect :exec
DELETE FROM redirects WHERE from_path = $1;
