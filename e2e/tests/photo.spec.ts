import { expect, test, type Page } from '@playwright/test';

// Registro por foto contra el backend real, con la IA simulada (AI_FAKE=1):
//   - foto normal → pollo, arroz y un glaseado que no está en la base
//   - "FAIL" → la IA no responde (no se cobra)
//   - "EMPTY" → no hay comida (no se cobra)

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

async function takePhoto(page: Page, content: string): Promise<void> {
  await page
    .locator('owli-today-page input[type=file]')
    .setInputFiles({ name: 'plato.jpg', mimeType: 'image/jpeg', buffer: Buffer.from(content) });
}

const credits = (page: Page) => page.locator('owli-today-page .credits');

test('foto → confirmación → guardar: cobra un análisis y suma las comidas', async ({ page }) => {
  await onboard(page);
  // 1 gratis de hoy + 15 de bienvenida.
  await expect(credits(page)).toHaveText(/16 análisis/);

  await takePhoto(page, 'plato');
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByRole('heading', { name: 'Revisa tu comida' })).toBeVisible();
  await expect(sheet.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(sheet.getByText('Arroz blanco cocido')).toBeVisible();
  // El glaseado no está en la base: queda marcada y no se guarda sin elegir un alimento.
  await expect(sheet.getByText('Todavía no está en la base: elige uno parecido o quítalo.')).toBeVisible();
  await expect(sheet.getByText('1 sin alimento elegido: no se guardará.')).toBeVisible();

  // Corregir: +10 % al pollo y quitar el glaseado.
  await sheet.getByRole('button', { name: 'Más Pechuga de pollo asada' }).click();
  await expect(sheet.getByText('165 g')).toBeVisible();
  await sheet.getByRole('button', { name: 'Quitar Glaseado' }).click();
  await expect(sheet.getByText('Quitaste Glaseado.')).toBeVisible();
  await expect(sheet.getByText('La foto no se guarda: se descarta después del análisis.')).toBeVisible();

  await sheet.getByRole('button', { name: 'Guardar (2)' }).click();
  const today = page.locator('owli-today-page');
  await expect(sheet).toHaveCount(0);
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(today.getByText('Arroz blanco cocido')).toBeVisible();
  await expect(today.getByText(/165 g ·/)).toBeVisible();
  await expect(credits(page)).toHaveText(/15 análisis/);

  // Quedó guardado en el servidor.
  await page.reload();
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
});

test('elegir a mano el alimento que la IA no encontró', async ({ page }) => {
  await onboard(page);
  await takePhoto(page, 'plato');
  const sheet = page.locator('owli-photo-review-sheet');
  await sheet.getByRole('button', { name: 'Elegir el alimento' }).click();
  await sheet.getByRole('searchbox').fill('mayonesa');
  await sheet.getByRole('button', { name: /^Mayonesa/ }).click();
  await expect(sheet.getByText('Mayonesa')).toBeVisible();
  await sheet.getByRole('button', { name: 'Guardar (3)' }).click();
  await expect(page.locator('owli-today-page').getByText('Mayonesa')).toBeVisible();
});

test('si la IA falla o no hay comida, no se cobra', async ({ page }) => {
  await onboard(page);
  await expect(credits(page)).toHaveText(/16 análisis/);
  const sheet = page.locator('owli-photo-review-sheet');

  await takePhoto(page, 'FAIL');
  await expect(sheet.getByRole('alert')).toHaveText(/No se usó ningún crédito/);
  await sheet.getByRole('button', { name: 'Cancelar' }).click();
  await expect(credits(page)).toHaveText(/16 análisis/);

  await takePhoto(page, 'EMPTY');
  await expect(sheet.getByRole('heading', { name: 'No vimos comida en la foto' })).toBeVisible();
  await sheet.getByRole('button', { name: 'Entendido' }).click();
  await expect(credits(page)).toHaveText(/16 análisis/);
});
