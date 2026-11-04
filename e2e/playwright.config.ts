import { defineConfig, devices } from '@playwright/test';

// Flujos críticos de Owli con emulación móvil, contra el backend real.
// Levanta su propia API (8090, base owli_e2e) y su propia app (4310) para no
// chocar con lo que esté corriendo en desarrollo. Requiere `docker compose up -d`.
//   npm test               → sin ventana
//   npm run test:headed    → viendo el navegador
const APP_PORT = 4310;
export const API_URL = 'http://localhost:8090';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env['CI'],
  retries: process.env['CI'] ? 2 : 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://localhost:${APP_PORT}`,
    locale: 'es-AR',
    timezoneId: 'America/Argentina/Buenos_Aires',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    // Tamaño de iPhone (390×844) con Chromium; WebKit se suma cuando haya CI en macOS.
    { name: 'iphone', use: { ...devices['iPhone 13'], browserName: 'chromium', defaultBrowserType: 'chromium' } },
    { name: 'android', use: { ...devices['Pixel 7'] } },
  ],
  webServer: [
    {
      command: 'go run ./cmd/api',
      cwd: '../backend',
      url: `${API_URL}/healthz`,
      env: {
        PORT: '8090',
        DATABASE_URL: 'postgres://owli:owli@localhost:5433/owli_e2e?sslmode=disable',
        REDIS_URL: 'redis://localhost:6379/1',
        CORS_ALLOWED_ORIGINS: `http://localhost:${APP_PORT}`,
        RATE_LIMIT_ANON_PER_IP_HOUR: '100000',
        RATE_LIMIT_SIGNIN_PER_IP_HOUR: '100000',
        RATE_LIMIT_AI_PER_USER_HOUR: '100000',
        // IA simulada: determinista y sin costo. La API la rechaza con APP_ENV=production.
        APP_ENV: 'test',
        AI_FAKE: '1',
      },
      // Nunca reutilizar: si el puerto está ocupado por otra app, mejor fallar que probar contra ella.
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: `npx ng serve --configuration e2e --port ${APP_PORT}`,
      cwd: '../app',
      url: `http://localhost:${APP_PORT}`,
      reuseExistingServer: false,
      timeout: 180_000,
    },
  ],
});
