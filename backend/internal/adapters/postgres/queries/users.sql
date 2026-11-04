-- name: CreateUser :one
INSERT INTO users DEFAULT VALUES
RETURNING id;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: FindUserByIdentity :one
SELECT user_id FROM identities
WHERE provider = $1 AND subject = $2;

-- name: InsertIdentity :execrows
-- Devuelve 0 filas si la identidad ya existía (de este u otro usuario).
INSERT INTO identities (provider, subject, user_id, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (provider, subject) DO NOTHING;
