-- name: InsertEntry :one
INSERT INTO meal_entries (
    user_id, food_id, grams, eaten_at, source, kcal, protein_g, carbs_g, fiber_g, fat_g,
    vitamin_c_mg, calcium_mg, iron_mg, magnesium_mg, potassium_mg, sodium_mg, zinc_mg, vitamin_b12_ug
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
)
RETURNING id;

-- name: DeleteEntry :execrows
-- Filtra por user_id: un usuario nunca puede borrar comidas de otro.
DELETE FROM meal_entries WHERE id = $1 AND user_id = $2;

-- name: ListEntries :many
SELECT e.*, f.name_es, f.name_en
FROM meal_entries e
JOIN foods f ON f.id = e.food_id
WHERE e.user_id = $1 AND e.eaten_at >= sqlc.arg(from_time) AND e.eaten_at < sqlc.arg(to_time)
ORDER BY e.eaten_at, e.created_at;
