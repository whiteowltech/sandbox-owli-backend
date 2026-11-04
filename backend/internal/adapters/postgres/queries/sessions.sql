-- name: InsertSession :exec
INSERT INTO sessions (user_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: LockSessionByHash :one
-- Bloquea la fila para que dos renovaciones simultáneas no usen el mismo token.
SELECT id, user_id, family_id, expires_at, used_at, revoked_at
FROM sessions
WHERE token_hash = $1
FOR UPDATE;

-- name: MarkSessionUsed :exec
UPDATE sessions SET used_at = $2 WHERE id = $1;

-- name: RevokeSessionFamily :exec
UPDATE sessions SET revoked_at = $2
WHERE family_id = $1 AND revoked_at IS NULL;
