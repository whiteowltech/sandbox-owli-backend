import { expect, test, type Page } from '@playwright/test';

// "Tu semana" contra el backend real.

async function onboard(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: 'Empezar' }).click();
  await page.getByRole('button', { name: 'Hombre' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: /^Omnívoro balanceado/ }).click();
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();
  await expect(page.locator('owli-today-page').getByText('Todavía no anotaste nada hoy')).toBeVisible();
}

test('semana vacía, después con una comida de hoy: resumen, detalle del día y tabla', async ({ page }) => {
  await onboard(page);
  const today = page.locator('owli-today-page');

  await today.getByRole('link', { name: 'Ver tu semana' }).click();
  const week = page.locator('owli-week-page');
  await expect(week.getByRole('heading', { name: 'Esta semana no hay días anotados' })).toBeVisible();
  await expect(week.getByRole('button', { name: 'Semana siguiente' })).toBeDisabled();

  // Anotar algo hoy y volver.
  await page.goBack();
  await today.getByRole('button', { name: 'Anotar a mano' }).click();
  await page.getByRole('searchbox', { name: 'Buscar un alimento' }).fill('pechuga de pollo');
  await page.getByRole('button', { name: /Pechuga de pollo asada/ }).click();
  await page.getByRole('button', { name: '200 g' }).click();
  await page.getByRole('button', { name: 'Agregar al día' }).click();
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();

  await today.getByRole('link', { name: 'Ver tu semana' }).click();
  await expect(week.getByRole('heading', { name: 'Cerraste 0 de 1 día anotado' })).toBeVisible();
  await expect(week.getByText('Promedio 330 kcal')).toBeVisible();
  await expect(week.getByText('Promedio 62 g')).toBeVisible();
  await page.waitForTimeout(600); // termina la animación de entrada
  await page.screenshot({ path: 'test-results/week-top.png' });

  // Tocar el día de hoy muestra el detalle.
  await week.getByRole('button', { name: /: hoy, en curso$/ }).click();
  await expect(week.getByRole('status')).toContainText('330 kcal');
  await expect(week.getByRole('status')).toContainText('El día sigue en curso.');

  await week.getByRole('button', { name: 'Ver como tabla' }).click();
  await expect(week.getByRole('table')).toBeVisible();
  await expect(week.getByRole('row')).toHaveCount(8); // encabezado + 7 días
  await page.screenshot({ path: 'test-results/week.png', fullPage: true });

  // La semana anterior está vacía y desde ahí se puede volver.
  await week.getByRole('button', { name: 'Semana anterior' }).click();
  await expect(week.getByRole('heading', { name: 'Esta semana no hay días anotados' })).toBeVisible();
  await week.getByRole('button', { name: 'Semana siguiente' }).click();
  await expect(week.getByRole('heading', { name: 'Cerraste 0 de 1 día anotado' })).toBeVisible();
});
