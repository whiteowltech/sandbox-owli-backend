import { expect, test, type Page } from '@playwright/test';

// Registro manual (gratis) contra el backend real, con el catálogo de USDA embebido.

type Diet = 'Omnívoro balanceado' | 'Vegano';

async function onboard(page: Page, diet: Diet, glutenFree = false): Promise<number> {
  await page.goto('/');
  await page.getByRole('button', { name: 'Empezar' }).click();
  await page.getByRole('button', { name: 'Hombre' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: new RegExp(`^${diet}`) }).click();
  if (glutenFree) await page.getByRole('button', { name: 'Sin gluten' }).click();
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();
  const today = page.locator('owli-today-page');
  await expect(today.getByText('Todavía no anotaste nada hoy')).toBeVisible();
  return Number((await today.locator('.status .big').innerText()).replace(/\D/g, ''));
}

async function openSearch(page: Page, query: string): Promise<void> {
  await page.getByRole('button', { name: 'Anotar a mano' }).click();
  await expect(page.getByText('Gratis. No usa análisis.')).toBeVisible();
  await page.getByRole('searchbox', { name: 'Buscar un alimento' }).fill(query);
}

test('anotar a mano: buscar con un error de tipeo, elegir porción y ver lo que queda', async ({ page }) => {
  const target = await onboard(page, 'Omnívoro balanceado');
  const today = page.locator('owli-today-page');

  await openSearch(page, 'pechga'); // error de tipeo a propósito
  await page.getByRole('button', { name: /Pechuga de pollo asada/ }).click();
  await page.getByRole('button', { name: '200 g' }).click();
  await expect(page.getByText('330 kcal · 62 g de proteína')).toBeVisible();
  await page.getByRole('button', { name: 'Agregar al día' }).click();

  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(today.locator('.status .big')).toHaveText(new RegExp(String(target - 330)));
  // Perfil por defecto (hombre, 75 kg, perder grasa): objetivo de proteína 2 g/kg = 150 g.
  await expect(today.locator('.macros div').first()).toContainText(/62\s*\/ 150 g/);
  // La frase depende de la hora (ánimo de Owli): "Te faltan 88 g de proteína." o "…: te faltan 88 g."
  await expect(today.locator('.owli-says')).toContainText(/[Tt]e faltan 88 g/);

  // Sinónimo regional: "aguacate" encuentra la palta.
  await openSearch(page, 'aguacate');
  await page.getByRole('button', { name: /^Palta/ }).click();
  await page.getByRole('button', { name: 'Más gramos' }).click(); // 110 g
  await page.getByRole('button', { name: 'Agregar al día' }).click();
  await expect(today.getByText('Palta')).toBeVisible();
  await expect(today.getByText(/110 g ·/)).toBeVisible();

  // Queda guardado en el servidor: al recargar sigue ahí.
  await page.reload();
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(today.getByText('Palta')).toBeVisible();

  // Borrar una comida devuelve las calorías, y se puede deshacer.
  const palta = today.locator('.entry', { hasText: 'Palta' });
  await today.getByRole('button', { name: 'Quitar Palta' }).click();
  await expect(palta).toHaveCount(0);
  await expect(today.getByText('Borraste Palta.')).toBeVisible();
  await expect(today.locator('.status .big')).toHaveText(new RegExp(String(target - 330)));
  await today.getByRole('button', { name: 'Deshacer' }).click();
  await expect(palta).toContainText('110 g');
  await page.reload();
  await expect(palta).toHaveCount(1);

  // Borrada de verdad: después de recargar no vuelve.
  await today.getByRole('button', { name: 'Quitar Palta' }).click();
  await expect(palta).toHaveCount(0);
  await expect(today.locator('.status .big')).toHaveText(new RegExp(String(target - 330)));
  await page.reload();
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(palta).toHaveCount(0);
});

test('avisa (sin bloquear) cuando un alimento no encaja con la dieta o la restricción', async ({ page }) => {
  await onboard(page, 'Vegano', true);

  await openSearch(page, 'pollo');
  await page.getByRole('button', { name: /Pechuga de pollo asada/ }).click();
  await expect(page.getByRole('status')).toHaveText('Es de origen animal: no entra en la dieta vegana.');
  await page.getByRole('button', { name: 'Cambiar' }).click();

  await page.getByRole('searchbox', { name: 'Buscar un alimento' }).fill('pan blanco');
  await page.getByRole('button', { name: /^Pan blanco/ }).click();
  await expect(page.getByRole('status')).toHaveText(/Contiene gluten/);
  // Se puede anotar igual: lo que comiste, se anota.
  await page.getByRole('button', { name: 'Agregar al día' }).click();
  await expect(page.locator('owli-today-page').getByText('Pan blanco')).toBeVisible();
});

test('una búsqueda sin resultados lo dice claro', async ({ page }) => {
  await onboard(page, 'Omnívoro balanceado');
  await openSearch(page, 'zzzzqx');
  await expect(page.getByText('No lo encontramos. Prueba con otro nombre.')).toBeVisible();
});
