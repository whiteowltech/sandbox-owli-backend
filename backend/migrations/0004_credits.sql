-- Libro de créditos (append-only). Idempotente.
-- Regla de CLAUDE.md: nunca UPDATE ni DELETE de filas; las correcciones y los
-- reembolsos son transacciones compensatorias nuevas.

CREATE TABLE IF NOT EXISTS credit_transactions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text        NOT NULL CHECK (kind IN ('grant', 'consume', 'refund')),
    bucket     text        NOT NULL CHECK (bucket IN ('daily_quota', 'subscription', 'welcome', 'pack')),
    -- Negativo solo en los consumos.
    delta      integer     NOT NULL CHECK (delta <> 0 AND ((kind = 'consume') = (delta < 0))),
    -- Clave de idempotencia: ID del análisis, del evento de RevenueCat o "welcome".
    reference  text        NOT NULL CHECK (reference <> ''),
    -- Día civil del usuario (su zona horaria) en que ocurrió: la cuota diaria se cuenta por este día.
    local_day  date        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind, reference)
);
CREATE INDEX IF NOT EXISTS credit_transactions_user_day_idx ON credit_transactions (user_id, local_day);

-- El ledger no se edita: un UPDATE es siempre un error de programación.
-- (DELETE solo ocurre en cascada al eliminar la cuenta, que borra todos los datos del usuario.)
CREATE OR REPLACE FUNCTION credit_transactions_no_update() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'credit_transactions es append-only: usa una transacción compensatoria';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS credit_transactions_no_update ON credit_transactions;
CREATE TRIGGER credit_transactions_no_update
    BEFORE UPDATE ON credit_transactions
    FOR EACH ROW EXECUTE FUNCTION credit_transactions_no_update();
