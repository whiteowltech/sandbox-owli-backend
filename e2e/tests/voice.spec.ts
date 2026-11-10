import { expect, test, type Page } from '@playwright/test';

// Registro por voz contra el backend real, con la IA simulada (AI_FAKE=1).
//   - Camino principal: el reconocimiento de voz del dispositivo (Web Speech API).
//     Se reemplaza por uno simulado que "entiende" una frase fija.
//   - Respaldo: sin reconocimiento en el navegador, se graba el audio y lo transcribe
//     el servidor (Whisper simulado). Chromium usa un micrófono falso (un tono).
test.use({
  permissions: ['microphone'],
  launchOptions: { args: ['--use-fake-ui-for-media-stream', '--use-fake-device-for-media-stream'] },
});

/** Reconocedor del dispositivo simulado: entiende `phrase` al empezar a escuchar. */
async function deviceSpeech(page: Page, phrase: string): Promise<void> {
  await page.addInitScript((text) => {
    class FakeRecognition {
      onresult: ((e: unknown) => void) | null = null;
      onend: (() => void) | null = null;
      onerror: ((e: unknown) => void) | null = null;
      start(): void {
        setTimeout(() => {
          const result = Object.assign([{ transcript: text }], { isFinal: true });
          this.onresult?.({ resultIndex: 0, results: [result] });
        }, 200);
      }
      stop(): void {
        setTimeout(() => this.onend?.(), 20);
      }
      abort(): void {}
    }
    Object.assign(window, { SpeechRecognition: FakeRecognition, webkitSpeechRecognition: FakeRecognition });
  }, phrase);
}

/** Navegador sin reconocimiento de voz (como Firefox): se usa el respaldo del servidor. */
async function noDeviceSpeech(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const w = window as unknown as Record<string, unknown>;
    delete w['SpeechRecognition'];
    delete w['webkitSpeechRecognition'];
  });
}

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

const credits = (page: Page) => page.locator('owli-today-page .credits');

test('voz en el dispositivo: texto en vivo → confirmación → guardar', async ({ page }) => {
  await deviceSpeech(page, 'Comí pechuga de pollo con arroz');
  await onboard(page);
  await expect(credits(page)).toHaveText(/16 análisis/);

  // Un toque en el micrófono empieza a escuchar; lo entendido aparece en vivo.
  await page.getByRole('button', { name: 'Contar lo que comiste' }).click();
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByRole('heading', { name: 'Te escucho…' })).toBeVisible();
  await expect(sheet.getByText('«Comí pechuga de pollo con arroz»')).toBeVisible();
  await sheet.getByRole('button', { name: 'Listo' }).click();

  await expect(sheet.getByRole('heading', { name: 'Revisa tu comida' })).toBeVisible();
  await expect(sheet.getByText('Escuché: «Comí pechuga de pollo con arroz»')).toBeVisible();
  await expect(sheet.getByText('Pechuga de pollo asada')).toBeVisible();

  await sheet.getByRole('button', { name: 'Guardar (2)' }).click();
  await expect(sheet).toHaveCount(0);
  const today = page.locator('owli-today-page');
  await expect(today.getByText('Pechuga de pollo asada')).toBeVisible();
  await expect(today.getByText('Arroz blanco cocido')).toBeVisible();
  await expect(credits(page)).toHaveText(/15 análisis/);
});

test('algo que no está en la base: lo estima la IA y se guarda sin elegir nada', async ({ page }) => {
  await deviceSpeech(page, 'Pancake proteico');
  await onboard(page);

  await page.getByRole('button', { name: 'Contar lo que comiste' }).click();
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByText('«Pancake proteico»')).toBeVisible();
  await sheet.getByRole('button', { name: 'Listo' }).click();

  // La IA simulada estima 221 kcal cada 100 g: 120 g = 265 kcal.
  await expect(sheet.getByRole('heading', { name: 'Revisa tu comida' })).toBeVisible();
  await expect(sheet.getByText('Pancake proteico', { exact: true })).toBeVisible();
  await expect(sheet.getByText('Estimado por IA.')).toBeVisible();
  await expect(sheet.getByText('265 kcal')).toBeVisible();
  await expect(sheet.getByText('elige uno parecido')).toHaveCount(0);

  await sheet.getByRole('button', { name: 'Guardar (1)' }).click();
  await expect(sheet).toHaveCount(0);
  const day = page.locator('owli-today-page .entries');
  await expect(day.getByText('Pancake proteico')).toBeVisible();
  await expect(day.getByText('265 kcal')).toBeVisible();

  // Queda guardado en el servidor como alimento propio.
  await page.reload();
  await expect(day.getByText('Pancake proteico')).toBeVisible();
});

test('receta: unidades, cuánto comiste y queda como plato propio', async ({ page }) => {
  await deviceSpeech(page, 'Hice unos waffles proteicos con dos huevos, un vaso de leche y tres cucharadas de cacao');
  await onboard(page);

  await page.getByRole('button', { name: 'Contar lo que comiste' }).click();
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByText(/waffles proteicos con dos huevos/)).toBeVisible();
  await sheet.getByRole('button', { name: 'Listo' }).click();

  // La receta: rinde 4, empieza en una porción; los ingredientes en sus unidades.
  await expect(sheet.getByText('Receta: Waffles proteicos')).toBeVisible();
  await expect(sheet.getByText(/Rinde 4 porciones/)).toBeVisible();
  await expect(sheet.getByText('1 de 4 porciones')).toBeVisible();
  await expect(sheet.getByText(/2\s+huevos/)).toBeVisible();
  await expect(sheet.getByText(/1\s+vaso\b/)).toBeVisible();
  await expect(sheet.getByText(/3\s+cucharadas/)).toBeVisible();

  // Un huevo más y uno menos: se mueve de a una unidad.
  await sheet.getByRole('button', { name: 'Más Huevo entero' }).click();
  await expect(sheet.getByText(/3\s+huevos/)).toBeVisible();
  await sheet.getByRole('button', { name: 'Menos Huevo entero' }).click();
  await expect(sheet.getByText(/2\s+huevos/)).toBeVisible();

  // Comí 2 de las 4 porciones: se anota la mitad de la receta (330 g → 165 g).
  await sheet.getByRole('button', { name: 'Una porción más' }).click();
  await expect(sheet.getByText('2 de 4 porciones')).toBeVisible();
  await sheet.getByRole('button', { name: 'Guardar receta y anotar' }).click();
  await expect(sheet).toHaveCount(0);
  const day = page.locator('owli-today-page .entries');
  await expect(day.getByText('Waffles proteicos')).toBeVisible();
  await expect(day.getByText(/165 g/)).toBeVisible();

  // Queda como plato propio: aparece en la búsqueda (primero, marcado).
  await page.getByRole('button', { name: 'Anotar a mano' }).click();
  await page.getByLabel('Buscar un alimento').fill('waffles');
  await expect(page.getByRole('button', { name: /Waffles proteicos\s+Tu receta/ })).toBeVisible();
});

test('respaldo: sin reconocimiento en el navegador, transcribe el servidor', async ({ page }) => {
  await noDeviceSpeech(page);
  await onboard(page);

  await page.getByRole('button', { name: 'Contar lo que comiste' }).click();
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByRole('heading', { name: 'Te escucho…' })).toBeVisible();
  await page.waitForTimeout(1000); // "hablar" un segundo
  await sheet.getByRole('button', { name: 'Listo' }).click();

  await expect(sheet.getByRole('heading', { name: 'Revisa tu comida' })).toBeVisible();
  await expect(sheet.getByText(/^Escuché: «Comí pechuga de pollo/)).toBeVisible();
  await expect(sheet.getByText('El audio no se guarda: se descarta después del análisis.')).toBeVisible();
  await sheet.getByRole('button', { name: 'Guardar (2)' }).click();
  await expect(sheet).toHaveCount(0);
  await expect(credits(page)).toHaveText(/15 análisis/);
});

test('cancelar la grabación no cobra', async ({ page }) => {
  await deviceSpeech(page, 'pollo');
  await onboard(page);
  await page.getByRole('button', { name: 'Contar lo que comiste' }).click();
  const sheet = page.locator('owli-photo-review-sheet');
  await expect(sheet.getByRole('heading', { name: 'Te escucho…' })).toBeVisible();
  await sheet.getByRole('button', { name: 'Cancelar' }).click();
  await expect(sheet).toHaveCount(0);
  await expect(credits(page)).toHaveText(/16 análisis/);
});
