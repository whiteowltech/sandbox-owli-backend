-- name: UpsertFood :exec
INSERT INTO foods (
    id, name_es, name_en, search_text, source, source_ref,
    kcal, protein_g, carbs_g, fiber_g, fat_g,
    vitamin_c_mg, calcium_mg, iron_mg, magnesium_mg, potassium_mg, sodium_mg, zinc_mg, vitamin_b12_ug,
    animal_origin, plant_origin, contains_meat, contains_fish, contains_grain, contains_legume,
    contains_dairy, contains_added_sugar, contains_gluten, contains_lactose, allergens_verified, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
    $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, now()
)
ON CONFLICT (id) DO UPDATE SET
    name_es = EXCLUDED.name_es, name_en = EXCLUDED.name_en, search_text = EXCLUDED.search_text,
    source = EXCLUDED.source, source_ref = EXCLUDED.source_ref,
    kcal = EXCLUDED.kcal, protein_g = EXCLUDED.protein_g, carbs_g = EXCLUDED.carbs_g,
    fiber_g = EXCLUDED.fiber_g, fat_g = EXCLUDED.fat_g,
    vitamin_c_mg = EXCLUDED.vitamin_c_mg, calcium_mg = EXCLUDED.calcium_mg, iron_mg = EXCLUDED.iron_mg,
    magnesium_mg = EXCLUDED.magnesium_mg, potassium_mg = EXCLUDED.potassium_mg, sodium_mg = EXCLUDED.sodium_mg,
    zinc_mg = EXCLUDED.zinc_mg, vitamin_b12_ug = EXCLUDED.vitamin_b12_ug,
    animal_origin = EXCLUDED.animal_origin, plant_origin = EXCLUDED.plant_origin,
    contains_meat = EXCLUDED.contains_meat, contains_fish = EXCLUDED.contains_fish,
    contains_grain = EXCLUDED.contains_grain, contains_legume = EXCLUDED.contains_legume,
    contains_dairy = EXCLUDED.contains_dairy, contains_added_sugar = EXCLUDED.contains_added_sugar,
    contains_gluten = EXCLUDED.contains_gluten, contains_lactose = EXCLUDED.contains_lactose,
    allergens_verified = EXCLUDED.allergens_verified, updated_at = now();

-- name: GetFood :one
SELECT * FROM foods WHERE id = $1;

-- name: SearchFoods :many
-- search_text guarda cada nombre y sinónimo separado por " | ". Cada término se
-- puntúa por separado (mezclarlos hacía que "beef steak" coincidiera con la milanesa
-- por "breaded steak" + "beef milanesa"). Orden:
--   4 = igual a un término, 3 = un término empieza con lo buscado, 2 = lo contiene,
--   después lo parecido a alguna palabra (errores de tipeo: "pechga" → "pechuga").
-- Umbral 0.5 de similitud por palabra: medido con el catálogo, separa los errores
-- de tipeo (0.57–0.89) de las coincidencias falsas (≤ 0.5).
-- Con un catálogo de cientos de alimentos el recorrido completo es instantáneo.
SELECT foods.* FROM foods,
LATERAL (
    SELECT max(CASE
                   WHEN t = sqlc.arg(query)::text THEN 4
                   WHEN t LIKE sqlc.arg(query)::text || '%' THEN 3
                   WHEN t LIKE '%' || sqlc.arg(query)::text || '%' THEN 2
                   ELSE 0 END) AS rank,
           max(word_similarity(sqlc.arg(query)::text, t)) AS sim,
           max(similarity(sqlc.arg(query)::text, t)) AS whole -- desempate: el término más parecido entero
    FROM unnest(string_to_array(foods.search_text, ' | ')) AS t
) s
WHERE s.rank > 0 OR s.sim >= 0.5
ORDER BY s.rank DESC, s.sim DESC, s.whole DESC,
    length(foods.name_es), -- a igualdad, el más genérico ("Milanesa de pollo frita" antes que "… al horno")
    foods.name_es
LIMIT sqlc.arg(max_results);

-- name: MatchFood :many
-- El mejor alimento para alguno de los nombres que devolvió la IA (el genérico en
-- inglés y el que se muestra en español): gana la coincidencia más fuerte entre todos.
SELECT foods.* FROM foods, unnest(sqlc.arg(queries)::text[]) AS q,
LATERAL (
    SELECT max(CASE
                   WHEN t = q THEN 4
                   WHEN t LIKE q || '%' THEN 3
                   WHEN t LIKE '%' || q || '%' THEN 2
                   ELSE 0 END) AS rank,
           max(word_similarity(q, t)) AS sim,
           max(similarity(q, t)) AS whole
    FROM unnest(string_to_array(foods.search_text, ' | ')) AS t
) s
WHERE q <> '' AND (s.rank > 0 OR s.sim >= 0.5)
ORDER BY s.rank DESC, s.sim DESC, s.whole DESC, length(foods.name_es), foods.name_es
LIMIT 1;
