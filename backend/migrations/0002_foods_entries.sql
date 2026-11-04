-- Catálogo de alimentos y comidas registradas. Idempotente.

-- Búsqueda tolerante a errores de tipeo ("pechga" encuentra "pechuga").
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Valores por 100 g. Los micros en NULL significan "sin dato", nunca cero.
CREATE TABLE IF NOT EXISTS foods (
    id                 text          PRIMARY KEY CHECK (id ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name_es            text          NOT NULL CHECK (name_es <> ''),
    name_en            text          NOT NULL CHECK (name_en <> ''),
    -- Nombres y sinónimos normalizados (minúsculas, sin acentos): lo que se busca.
    search_text        text          NOT NULL,
    source             text          NOT NULL CHECK (source IN ('usda_foundation', 'usda_sr_legacy', 'custom')),
    source_ref         text          NOT NULL DEFAULT '',
    kcal               numeric(7,2)  NOT NULL CHECK (kcal >= 0),
    protein_g          numeric(6,2)  NOT NULL CHECK (protein_g >= 0),
    carbs_g            numeric(6,2)  NOT NULL CHECK (carbs_g >= 0),
    fiber_g            numeric(6,2)  NOT NULL CHECK (fiber_g >= 0 AND fiber_g <= carbs_g),
    fat_g              numeric(6,2)  NOT NULL CHECK (fat_g >= 0),
    vitamin_c_mg       numeric(9,3),
    calcium_mg         numeric(9,3),
    iron_mg            numeric(9,3),
    magnesium_mg       numeric(9,3),
    potassium_mg       numeric(9,3),
    sodium_mg          numeric(9,3),
    zinc_mg            numeric(9,3),
    vitamin_b12_ug     numeric(9,3),
    animal_origin      boolean       NOT NULL DEFAULT false,
    plant_origin       boolean       NOT NULL DEFAULT false,
    contains_meat      boolean       NOT NULL DEFAULT false,
    contains_fish      boolean       NOT NULL DEFAULT false,
    contains_grain     boolean       NOT NULL DEFAULT false,
    contains_legume    boolean       NOT NULL DEFAULT false,
    contains_dairy     boolean       NOT NULL DEFAULT false,
    contains_added_sugar boolean     NOT NULL DEFAULT false,
    contains_gluten    boolean       NOT NULL DEFAULT false,
    contains_lactose   boolean       NOT NULL DEFAULT false,
    allergens_verified boolean       NOT NULL DEFAULT false,
    updated_at         timestamptz   NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS foods_search_trgm_idx ON foods USING gin (search_text gin_trgm_ops);

-- Cada fila guarda la porción ya calculada: si el catálogo se corrige, el
-- historial del usuario no cambia solo.
CREATE TABLE IF NOT EXISTS meal_entries (
    id             uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    food_id        text         NOT NULL REFERENCES foods (id),
    grams          numeric(6,1) NOT NULL CHECK (grams > 0 AND grams <= 3000),
    eaten_at       timestamptz  NOT NULL,
    source         text         NOT NULL CHECK (source IN ('manual', 'photo', 'voice', 'suggestion')),
    kcal           numeric(8,2) NOT NULL CHECK (kcal >= 0),
    protein_g      numeric(7,2) NOT NULL CHECK (protein_g >= 0),
    carbs_g        numeric(7,2) NOT NULL CHECK (carbs_g >= 0),
    fiber_g        numeric(7,2) NOT NULL CHECK (fiber_g >= 0),
    fat_g          numeric(7,2) NOT NULL CHECK (fat_g >= 0),
    vitamin_c_mg   numeric(10,3),
    calcium_mg     numeric(10,3),
    iron_mg        numeric(10,3),
    magnesium_mg   numeric(10,3),
    potassium_mg   numeric(10,3),
    sodium_mg      numeric(10,3),
    zinc_mg        numeric(10,3),
    vitamin_b12_ug numeric(10,3),
    created_at     timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS meal_entries_user_time_idx ON meal_entries (user_id, eaten_at);
