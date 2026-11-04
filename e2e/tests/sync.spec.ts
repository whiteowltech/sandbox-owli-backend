import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

import { API_URL } from '../playwright.config';

// El perfil del onboarding llega al servidor (Postgres real) con una cuenta anónima.

async function finishOnboarding(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Tu nombre' }).fill('Ana');
  await page.getByRole('button', { name: 'Hola, Owli. Soy Ana' }).click();
  await page.getByRole('button', { name: 'Mujer' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: 'Seguir' }).click();
  await page.getByRole('button', { name: /^Vegano/ }).click();
  await page.getByRole('button', { name: 'Sin lactosa' }).click();
  await page.getByRole('button', { name: 'Listo' }).click();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();
  await expect(page).toHaveURL(/\/today$/);
}

async function session(page: Page): Promise<{ accessToken: string; userId: string } | null> {
  return page.evaluate(() => {
    const raw = localStorage.getItem('owli:session:v1');
    return raw ? JSON.parse(raw) : null;
  });
}

async function serverProfile(request: APIRequestContext, token: string) {
  const res = await request.get(`${API_URL}/v1/me/profile`, { headers: { Authorization: `Bearer ${token}` } });
  return { status: res.status(), body: res.ok() ? await res.json() : null };
}

test('al terminar el onboarding, el perfil queda guardado en el servidor', async ({ page, request }) => {
  await finishOnboarding(page);
  await expect.poll(async () => (await session(page))?.userId ?? '').not.toBe('');
  const s = await session(page);

  await expect.poll(async () => (await serverProfile(request, s!.accessToken)).status).toBe(200);
  const { body } = await serverProfile(request, s!.accessToken);
  expect(body.profile).toMatchObject({
    name: 'Ana',
    sex: 'female',
    diet: 'vegan',
    restrictions: ['lactose_free'],
  });
  // La zona horaria del dispositivo (Chromium usa el alias corto America/Buenos_Aires).
  expect(body.profile.timezone).toMatch(/Buenos_Aires$/);
  // El servidor calcula el mismo objetivo que muestra la app.
  const shown = await page.locator('owli-today-page .status .big').innerText();
  expect(shown.replace(/\D/g, '')).toBe(String(body.plan.dailyTarget));
});

test('sin conexión, el usuario entra igual y el perfil se sube al volver', async ({ page, request }) => {
  await page.route(`${API_URL}/**`, (route) => route.abort('internetdisconnected'));
  await finishOnboarding(page);
  await expect(page.locator('owli-today-page').getByText('Hola, Ana')).toBeVisible();
  await expect.poll(() => page.evaluate(() => localStorage.getItem('owli:profile-sync:pending'))).toBe('1');
  expect(await session(page)).toBeNull();

  // Vuelve la conexión: al abrir la app de nuevo, se reintenta solo.
  await page.unroute(`${API_URL}/**`);
  await page.reload();
  await expect.poll(() => page.evaluate(() => localStorage.getItem('owli:profile-sync:pending'))).toBeNull();
  const s = await session(page);
  expect((await serverProfile(request, s!.accessToken)).body.profile.diet).toBe('vegan');
});

test('borrar el perfil desde la cuenta elimina también la cuenta del servidor', async ({ page, request }) => {
  await finishOnboarding(page);
  await expect.poll(() => page.evaluate(() => localStorage.getItem('owli:profile-sync:pending'))).toBeNull();
  const s = await session(page);
  await expect.poll(async () => (await serverProfile(request, s!.accessToken)).status).toBe(200);

  await page.getByRole('link', { name: 'Ver tu cuenta' }).click();
  await page.getByRole('button', { name: 'Borrar el perfil y volver a empezar' }).click();
  await expect(page).toHaveURL(/\/onboarding$/);
  expect((await serverProfile(request, s!.accessToken)).status).toBe(404);
  expect(await session(page)).toBeNull();
});
