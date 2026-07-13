-- name: CreateUser :one
INSERT INTO users (username, password_hash, created_at)
VALUES (@username, @password_hash, COALESCE(sqlc.narg('created_at'), now()))
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;
