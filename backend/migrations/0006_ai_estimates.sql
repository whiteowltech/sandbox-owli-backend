-- Alimentos estimados por la IA (9 de noviembre de 2026): cuando un alimento de una
-- foto o un audio no está en el catálogo con coincidencia exacta, se guarda con la
-- composición que estimó la IA como alimento privado de ese usuario. El ID lo arma
-- el dominio (domain.NewEstimatedFood), así que el mismo alimento se reutiliza.

ALTER TABLE foods ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES users (id) ON DELETE CASCADE;

-- 'ai_estimate' como origen posible (el CHECK anterior es de 0005).
ALTER TABLE foods DROP CONSTRAINT IF EXISTS foods_source_check;
ALTER TABLE foods ADD CONSTRAINT foods_source_check
    CHECK (source IN ('usda_foundation', 'usda_sr_legacy', 'owli_recipe', 'custom', 'ai_estimate'));

-- Un alimento tiene dueño si y solo si lo estimó la IA: el catálogo es de todos.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'foods_owner_check') THEN
        ALTER TABLE foods ADD CONSTRAINT foods_owner_check
            CHECK ((source = 'ai_estimate') = (owner_user_id IS NOT NULL));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS foods_owner_idx ON foods (owner_user_id) WHERE owner_user_id IS NOT NULL;
