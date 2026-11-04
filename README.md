# Owli — backend

API de Owli: nutrición predictiva con registro por foto, voz o a mano, sugerencias para cerrar el día y un búho copiloto. Está escrita en Go con arquitectura hexagonal (ports & adapters) y router Chi. Usa PostgreSQL y Redis, Gemini para identificar alimentos, Claude Haiku de respaldo y Whisper (Groq) de respaldo para la voz.

La app (Ionic + Angular) está en [sandbox-owli-app](https://github.com/whiteowltech/sandbox-owli-app). Las pruebas de punta a punta de `e2e/` esperan las dos carpetas una al lado de la otra: `backend/` (este repo) y `app/`.

> Las reglas de negocio, las decisiones y los comandos completos están en [CLAUDE.md](CLAUDE.md). Las tareas abiertas, en [pendientes.md](pendientes.md).

## Estructura

```
backend/
  cmd/api/            # main: configuración, inyección de dependencias, arranque
  cmd/importfoods/    # arma el catálogo de alimentos desde USDA (data/foods_seed.json)
  cmd/benchmark/      # benchmark de modelos de visión (Nutrition5k)
  internal/domain/    # reglas puras (solo stdlib): plan, macros, dietas, créditos, sugerencias
  internal/ports/     # interfaces de los casos de uso y de los adapters
  internal/usecase/   # casos de uso
  internal/adapters/  # httpapi, postgres (sqlc), redisx, jwtauth, oidc, gemini, anthropic, groq…
  migrations/         # SQL idempotente, embebido y aplicado al arrancar
  data/               # catálogo curado de alimentos, recetas y platos (embebido)
e2e/                  # Playwright: levanta la API y la app con IA simulada
docker-compose.yml    # Postgres 16 + Redis 7 para desarrollo
```

## Desarrollo

Requisitos: Go, Docker Desktop y Node (para `e2e/`).

```bash
docker compose up -d                  # Postgres (puerto 5433) y Redis (6379)
cp backend/.env.example backend/.env  # completar JWT_SECRET y las claves de IA
cd backend
go run ./cmd/api                      # API en :8080 (GET /healthz)
go test ./...                         # pruebas
go vet ./... && gofmt -l .            # lint (gofmt no debe listar nada)
```

- **Pruebas de Postgres:** necesitan `TEST_DATABASE_URL="postgres://owli:owli@localhost:5433/owli_test?sslmode=disable"`. Sin la variable, se saltean.
- **IA simulada:** `AI_FAKE=1` usa una IA determinista y sin costo. La API se niega a arrancar con `AI_FAKE=1` en `APP_ENV=production`.

## Producción (Railway)

- La imagen se construye con el `Dockerfile` de `backend/`: multi-stage, distroless, binario de menos de 25 MB.
- Las migraciones y el catálogo de alimentos van embebidos y se aplican al arrancar.
- Los secretos solo van en variables de Railway: `JWT_SECRET`, `DATABASE_URL`, `REDIS_URL`, `GEMINI_API_KEY` (tier de pago), `ANTHROPIC_API_KEY` y `GROQ_API_KEY`. La lista completa con sus valores por defecto está en [backend/.env.example](backend/.env.example).
