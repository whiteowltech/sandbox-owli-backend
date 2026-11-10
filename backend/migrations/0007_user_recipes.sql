-- Recetas propias y alimentos que faltan en el catálogo (9 de noviembre de 2026).

-- Recetas del usuario ("Waffles proteicos"): alimentos propios calculados con sus
-- ingredientes (domain.NewUserRecipe). serving_g es el peso de una porción, si se sabe.
ALTER TABLE foods ADD COLUMN IF NOT EXISTS serving_g numeric(7,1) CHECK (serving_g IS NULL OR serving_g > 0);

ALTER TABLE foods DROP CONSTRAINT IF EXISTS foods_source_check;
ALTER TABLE foods ADD CONSTRAINT foods_source_check
    CHECK (source IN ('usda_foundation', 'usda_sr_legacy', 'owli_recipe', 'custom', 'ai_estimate', 'user_recipe'));

-- Un alimento tiene dueño si y solo si es propio (lo estimó la IA o es una receta del usuario).
ALTER TABLE foods DROP CONSTRAINT IF EXISTS foods_owner_check;
ALTER TABLE foods ADD CONSTRAINT foods_owner_check
    CHECK ((source IN ('ai_estimate', 'user_recipe')) = (owner_user_id IS NOT NULL));

-- Lo que la IA identificó y no estaba en el catálogo, para sumar los más pedidos con
-- datos de USDA. Solo el nombre normalizado y un contador: no se guarda quién fue.
CREATE TABLE IF NOT EXISTS food_misses (
    name        text        PRIMARY KEY CHECK (name <> ''),
    misses      integer     NOT NULL DEFAULT 1 CHECK (misses > 0),
    first_seen  timestamptz NOT NULL DEFAULT now(),
    last_seen   timestamptz NOT NULL DEFAULT now()
);
