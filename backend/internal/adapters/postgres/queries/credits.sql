-- name: LockUser :one
-- Bloquea la fila del usuario: dos análisis simultáneos no pueden gastar el mismo crédito.
SELECT id FROM users WHERE id = $1 FOR UPDATE;

-- name: InsertCreditTx :execrows
-- 0 filas = ya existía (misma referencia): la operación es idempotente.
INSERT INTO credit_transactions (user_id, kind, bucket, delta, reference, local_day)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, kind, reference) DO NOTHING;

-- name: WalletSums :one
-- Saldo de cada bucket y uso del día (consumos netos de reembolsos).
SELECT
    COALESCE(SUM(delta) FILTER (WHERE bucket = 'welcome'), 0)::int AS welcome_balance,
    COALESCE(SUM(delta) FILTER (WHERE bucket = 'pack'), 0)::int    AS pack_balance,
    (-COALESCE(SUM(delta) FILTER (
        WHERE bucket IN ('daily_quota', 'subscription') AND local_day = sqlc.arg(local_day)::date
    ), 0))::int AS used_today
FROM credit_transactions
WHERE user_id = $1;

-- name: GetConsume :one
SELECT bucket, local_day FROM credit_transactions
WHERE user_id = $1 AND kind = 'consume' AND reference = $2;
