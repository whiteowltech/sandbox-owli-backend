-- Platos compuestos (recetas calculadas a partir de ingredientes de USDA, ver
-- domain.ComposeRecipe): se agrega 'owli_recipe' a los orígenes válidos.
-- Idempotente: se reemplaza la restricción completa.
ALTER TABLE foods DROP CONSTRAINT IF EXISTS foods_source_check;
ALTER TABLE foods ADD CONSTRAINT foods_source_check
    CHECK (source IN ('usda_foundation', 'usda_sr_legacy', 'owli_recipe', 'custom'));
