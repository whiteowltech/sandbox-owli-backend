import { expect, test, type Page } from '@playwright/test';

// Onboarding v5: saludo → tu cuerpo → objetivo → dieta → listo → Hoy.
// Cada test arranca con el almacenamiento vacío (contexto nuevo de Playwright).

async function start(page: Page): Promise<void> {
  await page.goto('/');
  await expect(page).toHaveURL(/\/onboarding$/);
}

async function goToBody(page: Page, name = ''): Promise<void> {
  if (name) await page.getByRole('textbox', { name: 'Tu nombre' }).fill(name);
  await page.getByRole('button', { name: name ? `Hola, Owli. Soy ${name}` : 'Empezar' }).click();
  await expect(page.getByText('1 de 3')).toBeVisible();
}

const seguir = (page: Page) => page.getByRole('button', { name: 'Seguir' });

test('recorrido completo: el objetivo calculado llega a Hoy', async ({ page }) => {
  await start(page);
  await goToBody(page, 'Leandro');
  await expect(page.getByRole('heading', { name: /Leandro, cuéntame de ti/ })).toBeVisible();

  // Sin sexo no avanza, y el error se anuncia.
  await seguir(page).click();
  await expect(page.getByRole('alert')).toHaveText(/la fórmula del gasto diario la necesita/);
  await expect(page.getByText('Elige tu sexo biológico y lo calculo.')).toBeVisible();

  // Valores por defecto: 30 años, 170 cm, 75 kg, actividad media. +1 kg dos veces → 76 kg.
  await page.getByRole('button', { name: 'Hombre' }).click();
  await page.getByRole('button', { name: 'Más Peso' }).click();
  await page.getByRole('button', { name: 'Más Peso' }).click();
  await expect(page.getByText('76 kg')).toBeVisible();
  // (10·76 + 6,25·170 − 5·30 + 5) × 1,55 = 2600
  await expect(page.locator('.burn-value')).toHaveText(/2600 kcal/);
  await seguir(page).click();

  // Objetivo: perder grasa (por defecto), peso meta sugerido 7 % menos, ritmo exigente.
  await expect(page.getByText('2 de 3')).toBeVisible();
  await expect(page.getByRole('button', { name: /Perder grasa/ })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('70,5 kg')).toBeVisible();
  await page.getByRole('button', { name: 'Exigente' }).click();
  await expect(page.getByText('−0,8 kg por semana')).toBeVisible();
  await expect(page.locator('.burn-value')).toHaveText(/1775/); // 2600 − 825
  await expect(page.getByText(/Llegarías alrededor del/)).toBeVisible();
  await seguir(page).click();

  // Dieta: obligatoria; las restricciones se combinan.
  await expect(page.getByText('3 de 3')).toBeVisible();
  await page.getByRole('button', { name: 'Listo' }).click();
  await expect(page.getByRole('alert')).toHaveText('Elige una dieta para seguir.');
  await page.getByRole('button', { name: /^Paleo/ }).click();
  await page.getByRole('button', { name: 'Sin gluten' }).click();
  await expect(page.getByText('Anotado. Comida real, como antes.')).toBeVisible();
  await page.getByRole('button', { name: 'Listo' }).click();

  // Resultado
  await expect(page.getByText('Listo, Leandro. Ya puedes empezar tu día.')).toBeVisible();
  await expect(page.getByText(/Perder grasa · Paleo\s+· Sin gluten/)).toBeVisible();
  await expect(page.getByText(/15 análisis con IA son de regalo/)).toBeVisible();
  await page.getByRole('button', { name: 'Ir a mi día' }).click();

  await expect(page).toHaveURL(/\/today$/);
  const today = page.locator('owli-today-page');
  await expect(today.getByText('Hola, Leandro')).toBeVisible();
  await expect(today.locator('.status .big')).toHaveText(/1775/);

  // El perfil queda guardado: al recargar o volver al onboarding, se queda en Hoy.
  await page.reload();
  await expect(page.getByText('Hola, Leandro')).toBeVisible();
  await page.goto('/onboarding');
  await expect(page).toHaveURL(/\/today$/);
});

test('menores de 18 no pueden seguir', async ({ page }) => {
  await start(page);
  await goToBody(page);
  await page.getByRole('button', { name: 'Mujer' }).click();
  await page.getByRole('slider', { name: 'Edad' }).fill('16');
  await expect(page.getByRole('alert')).toHaveText('Owli es para mayores de 18 años.');
  await seguir(page).click();
  await expect(page.getByText('1 de 3')).toBeVisible();
});

test('nunca baja del mínimo seguro y no muestra fechas absurdas', async ({ page }) => {
  await start(page);
  await goToBody(page);
  await page.getByRole('button', { name: 'Mujer' }).click();
  await page.getByRole('slider', { name: 'Edad' }).fill('60');
  await page.getByRole('slider', { name: 'Altura' }).fill('155');
  await page.getByRole('slider', { name: 'Peso' }).fill('50');
  await page.getByRole('button', { name: 'Sedentario: poco o nada de ejercicio' }).click();
  await seguir(page).click();
  await page.getByRole('button', { name: 'Exigente' }).click();

  await expect(page.locator('.burn-value')).toHaveText(/1200/);
  await expect(page.getByText('No bajo de 1200 kcal: es el mínimo seguro.')).toBeVisible();
  await expect(page.getByText(/Sumar actividad te ayudará más que comer menos/)).toBeVisible();
  await expect(page.getByText(/Llegarías alrededor del/)).toHaveCount(0);
});

test('peso meta en la dirección equivocada bloquea el avance', async ({ page }) => {
  await start(page);
  await goToBody(page);
  await page.getByRole('button', { name: 'Hombre' }).click();
  await seguir(page).click();
  await page.getByRole('slider', { name: 'Peso meta' }).fill('90');
  await expect(page.getByRole('alert')).toHaveText(/el peso meta tiene que ser menor que el actual/);
  await seguir(page).click();
  await expect(page.getByText('2 de 3')).toBeVisible();
});

test('retoma donde quedó si se cierra la app', async ({ page }) => {
  await start(page);
  await goToBody(page, 'Ana');
  await page.getByRole('button', { name: 'Mujer' }).click();
  await page.getByRole('slider', { name: 'Peso' }).fill('62');
  await page.reload();
  await expect(page.getByText('1 de 3')).toBeVisible();
  await expect(page.getByText('62 kg')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Mujer' })).toHaveAttribute('aria-pressed', 'true');
});

test('inglés y unidades imperiales', async ({ page }) => {
  await start(page);
  await page.getByRole('button', { name: 'EN' }).click();
  await expect(page.getByText(/Hi, I’m Owli/)).toBeVisible();
  await page.getByRole('button', { name: 'Start' }).click();
  await page.getByRole('button', { name: 'Male', exact: true }).click();
  await page.getByRole('button', { name: 'lb · ft' }).click();
  await expect(page.getByText('165 lb')).toBeVisible(); // 75 kg
  await expect(page.getByText('5′ 7″')).toBeVisible(); // 170 cm
  await expect(page.getByText('You burn per day, approx.')).toBeVisible();
});

test('el perfil de prueba de desarrollo salta directo a Hoy', async ({ page }) => {
  await start(page);
  await page.getByRole('button', { name: 'Usar un perfil de prueba' }).click();
  await expect(page).toHaveURL(/\/today$/);
});
