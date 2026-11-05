import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

import { API_URL } from '../playwright.config';

// "Tu cuenta": ver y corregir el plan después del onboarding, contra el backend real.

async function onboard(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: 'Empezar' }).click();
  await page.getByRole('button', { name: 'Hombre' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: /^Omnívoro balanceado/ }).click();
  // Comidas por día: se eligen en el onboarding (antes quedaban fijas en 4).
  await page.getByRole('group', { name: '¿Cuántas comidas haces al día?' }).getByRole('button', { name: '4' }).click();
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();
  await expect(page).toHaveURL(/\/today$/);
  await expect.poll(() => page.evaluate(() => localStorage.getItem('owli:profile-sync:pending'))).toBeNull();
}

async function serverProfile(page: Page, request: APIRequestContext) {
  const token = await page.evaluate(() => JSON.parse(localStorage.getItem('owli:session:v1')!).accessToken as string);
  const res = await request.get(`${API_URL}/v1/me/profile`, { headers: { Authorization: `Bearer ${token}` } });
  expect(res.ok()).toBe(true);
  return res.json();
}

test('el plan se ve y se corrige desde la cuenta, y el día usa el objetivo nuevo', async ({ page, request }) => {
  await onboard(page);
  expect((await serverProfile(page, request)).profile.mealsPerDay).toBe(4);
  const before = (await page.locator('owli-today-page .status .of').innerText()).replace(/\D/g, '').slice(1);

  await page.getByRole('link', { name: 'Ver tu cuenta' }).click();
  const account = page.locator('owli-account-page');
  await expect(account.getByRole('button', { name: /Comidas por día\s*4 comidas/ })).toBeVisible();
  await expect(account.getByText('Gratis de hoy', { exact: true })).toBeVisible();

  // Comidas por día: 4 → 2.
  await account.getByRole('button', { name: /Comidas por día/ }).click();
  await account.getByRole('group', { name: 'Comidas por día' }).getByRole('button', { name: '2' }).click();
  await expect(account.getByText('Dos comidas, por ejemplo almuerzo y cena.')).toBeVisible();

  // Actividad: el objetivo se recalcula en vivo y muestra el anterior.
  await account.getByRole('button', { name: /Tu cuerpo/ }).click();
  await account.getByRole('button', { name: 'Muy activo: trabajo físico o doble sesión' }).click();
  await expect(account.getByText(/^Antes: /)).toBeVisible();
  await page.screenshot({ path: 'test-results/account-editing.png', fullPage: true });

  await account.getByRole('button', { name: 'Guardar cambios' }).click();
  await expect(account.getByRole('status').filter({ hasText: 'Listo: tu plan quedó actualizado.' })).toBeVisible();
  const saved = await serverProfile(page, request);
  expect(saved.profile).toMatchObject({ mealsPerDay: 2, activity: 'very_active' });

  // Hoy muestra el objetivo nuevo, igual al que calculó el servidor.
  await account.getByRole('link', { name: 'Volver' }).click();
  const of = page.locator('owli-today-page .status .of');
  await expect.poll(async () => (await of.innerText()).replace(/\D/g, '').slice(1)).toBe(
    String(saved.plan.dailyTarget),
  );
  expect(String(saved.plan.dailyTarget)).not.toBe(before);
});

test('no deja guardar un peso meta en la dirección contraria al objetivo, y descartar vuelve atrás', async ({
  page,
}) => {
  await onboard(page);
  await page.getByRole('link', { name: 'Ver tu cuenta' }).click();
  const account = page.locator('owli-account-page');

  await account.getByRole('button', { name: /^Objetivo/ }).click();
  const more = account.getByRole('button', { name: /^Más peso meta$/i });
  for (let i = 0; i < 30; i++) await more.click(); // +15 kg: por encima del peso actual
  await expect(account.getByRole('alert')).toBeVisible();
  await expect(account.getByRole('button', { name: 'Guardar cambios' })).toBeDisabled();

  await account.getByRole('button', { name: 'Descartar' }).click();
  await expect(account.getByRole('button', { name: 'Guardar cambios' })).toHaveCount(0);
});

test('las unidades se cambian al instante', async ({ page, request }) => {
  await onboard(page);
  await page.getByRole('link', { name: 'Ver tu cuenta' }).click();
  const account = page.locator('owli-account-page');
  await account.getByRole('button', { name: /^lb/ }).click();
  await expect(account.getByRole('button', { name: /Tu cuerpo/ })).toContainText('lb');
  await expect(account.getByRole('button', { name: 'Guardar cambios' })).toHaveCount(0);
  await expect.poll(async () => (await serverProfile(page, request)).profile.units).toBe('imperial');
});
