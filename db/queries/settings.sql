-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: SetSetting :one
-- Upsert: insert the key or update its value if it already exists.
INSERT INTO settings (key, value, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, updated_at = now()
RETURNING value;
