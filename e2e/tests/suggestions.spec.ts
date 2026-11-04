import { expect, test, type Page } from '@playwright/test';

// Meal Closing Engine contra el backend real y el catálogo real de platos.

async function onboard(page: Page, diet: RegExp): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: 'Empezar' }).click();
  await page.getByRole('button', { name: 'Hombre' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: diet }).click();
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();
  await expect(page.locator('owli-today-page').getByText('Todavía no anotaste nada hoy')).toBeVisible();
}

test('ver qué comer ahora y anotar un plato en un toque, gratis', async ({ page }) => {
  await onboard(page, /^Omnívoro balanceado/);
  const today = page.locator('owli-today-page');
  await expect(today.locator('.credits')).toHaveText(/16 análisis/);

  await today.getByRole('button', { name: 'Ver qué comer ahora' }).click();
  const sheet = page.locator('owli-suggestions-sheet');
  await expect(sheet.getByRole('heading', { name: 'Para tu próxima comida' })).toBeVisible();
  await expect(sheet.getByText(/Te faltan \d+ kcal/)).toBeVisible();
  const dishes = sheet.locator('.dish');
  await expect(dishes).toHaveCount(5); // primero 5; el resto, a pedido
  await expect(sheet.getByText('Cierra justo').first()).toBeVisible();
  await sheet.getByRole('button', { name: /^Ver \d+ opciones más$/ }).click();
  await expect(dishes.nth(5)).toBeVisible();
  await page.screenshot({ path: 'test-results/suggestions-sheet.png' });

  const first = dishes.first();
  const name = (await first.locator('h3').textContent())!.trim();
  await first.getByRole('button', { name: `Anotar ${name}` }).click();
  await expect(sheet).toHaveCount(0);
  await expect(today.getByText('Todavía no anotaste nada hoy')).toHaveCount(0);
  expect(await today.locator('.entry').count()).toBeGreaterThanOrEqual(2);
  // Anotar un plato sugerido no usa créditos.
  await expect(today.locator('.credits')).toHaveText(/16 análisis/);
});

test('las sugerencias respetan la dieta: vegano sin nada de origen animal', async ({ page }) => {
  await onboard(page, /^Vegano/);
  await page.locator('owli-today-page').getByRole('button', { name: 'Ver qué comer ahora' }).click();
  const sheet = page.locator('owli-suggestions-sheet');
  await expect(sheet.locator('.dish').first()).toBeVisible();
  await sheet.getByRole('button', { name: /^Ver \d+ opciones más$/ }).click();
  const text = (await sheet.locator('.dishes').textContent())!.toLowerCase();
  for (const animal of ['pollo', 'salmón', 'bife', 'huevo', 'yogur', 'atún', 'queso', 'manteca']) {
    expect(text, animal).not.toContain(animal);
  }
});
