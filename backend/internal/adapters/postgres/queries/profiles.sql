-- name: GetProfile :one
SELECT name, sex, age_years, height_cm, weight_kg, activity, goal, pace,
       target_weight_kg, diet, restrictions, meals_per_day, units, timezone
FROM profiles
WHERE user_id = $1;

-- name: UpsertProfile :exec
INSERT INTO profiles (
    user_id, name, sex, age_years, height_cm, weight_kg, activity, goal, pace,
    target_weight_kg, diet, restrictions, meals_per_day, units, timezone, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now()
)
ON CONFLICT (user_id) DO UPDATE SET
    name             = EXCLUDED.name,
    sex              = EXCLUDED.sex,
    age_years        = EXCLUDED.age_years,
    height_cm        = EXCLUDED.height_cm,
    weight_kg        = EXCLUDED.weight_kg,
    activity         = EXCLUDED.activity,
    goal             = EXCLUDED.goal,
    pace             = EXCLUDED.pace,
    target_weight_kg = EXCLUDED.target_weight_kg,
    diet             = EXCLUDED.diet,
    restrictions     = EXCLUDED.restrictions,
    meals_per_day    = EXCLUDED.meals_per_day,
    units            = EXCLUDED.units,
    timezone         = EXCLUDED.timezone,
    updated_at       = now();
