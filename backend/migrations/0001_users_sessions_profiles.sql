-- Usuarios, identidades externas, sesiones (refresh tokens) y perfiles.
-- Idempotente: se puede ejecutar más de una vez sin efectos.

CREATE TABLE IF NOT EXISTS users (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Una identidad de Apple, Google o correo pertenece a un solo usuario.
-- Un usuario sin identidades es una cuenta anónima.
CREATE TABLE IF NOT EXISTS identities (
    provider   text        NOT NULL CHECK (provider IN ('apple', 'google', 'email')),
    subject    text        NOT NULL CHECK (subject <> ''),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX IF NOT EXISTS identities_user_id_idx ON identities (user_id);

-- Refresh tokens: solo se guarda el hash (SHA-256), nunca el token.
-- Cada uso rota el token dentro de la misma familia; si un token ya usado
-- vuelve a aparecer (posible robo), se revoca la familia entera.
CREATE TABLE IF NOT EXISTS sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id  uuid        NOT NULL,
    token_hash bytea       NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_family_id_idx ON sessions (family_id);

CREATE TABLE IF NOT EXISTS profiles (
    user_id          uuid         PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    name             text         NOT NULL DEFAULT '' CHECK (char_length(name) <= 40),
    sex              text         NOT NULL CHECK (sex IN ('female', 'male')),
    age_years        smallint     NOT NULL CHECK (age_years BETWEEN 18 AND 120),
    height_cm        numeric(4,1) NOT NULL CHECK (height_cm BETWEEN 120 AND 250),
    weight_kg        numeric(4,1) NOT NULL CHECK (weight_kg BETWEEN 30 AND 350),
    activity         text         NOT NULL CHECK (activity IN ('sedentary', 'light', 'moderate', 'active', 'very_active')),
    goal             text         NOT NULL CHECK (goal IN ('cut', 'maintain', 'bulk', 'recomp')),
    pace             text         NOT NULL CHECK (pace IN ('slow', 'steady', 'fast')),
    target_weight_kg numeric(4,1) NOT NULL CHECK (target_weight_kg BETWEEN 30 AND 350),
    diet             text         NOT NULL CHECK (diet IN ('omnivore', 'vegetarian', 'pescetarian', 'vegan', 'keto', 'paleo', 'carnivore')),
    restrictions     text[]       NOT NULL DEFAULT '{}' CHECK (restrictions <@ ARRAY['gluten_free', 'lactose_free']),
    meals_per_day    smallint     NOT NULL CHECK (meals_per_day BETWEEN 1 AND 5),
    units            text         NOT NULL CHECK (units IN ('metric', 'imperial')),
    timezone         text         NOT NULL,
    updated_at       timestamptz  NOT NULL DEFAULT now()
);
