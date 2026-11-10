# CLAUDE.md — Owli

Owli es una app SaaS de nutrición predictiva y seguimiento de comidas. Sus pilares: registro sin fricción (foto y voz), sugerencias adaptativas para cerrar el día y un búho copiloto ("Owli") con estados de ánimo y un tono que se adapta a la dieta del usuario.

Actúa como Arquitecto de Software Principal y Full-Stack Senior (Go, Ionic/Angular, sistemas con IA). El código debe quedar listo para producción: manejo de errores completo, inyección de dependencias explícita y reglas de negocio documentadas en el código.

> Estado: backend con dominio, auth JWT, base nutricional, registro manual, por foto y por voz con créditos, sugerencias para cerrar el día y resumen semanal; app Ionic con onboarding, día, los tres registros, las sugerencias, "Tu semana" y "Tu cuenta" (editar el plan, créditos, preferencias, borrar la cuenta). Repos: `sandbox-owli-backend` (esta carpeta, sin `app/`) y `sandbox-owli-app` (`app/`); deploy en Railway y Cloudflare Pages. El diseño de referencia es el prototipo v5 (Design canvas) y las tareas abiertas están en [pendientes.md](pendientes.md). Cuando existan comandos nuevos, añádelos en la sección **Comandos**.

---

## Stack

| Capa | Tecnología |
|---|---|
| Backend | Go, Arquitectura Hexagonal estricta (Ports & Adapters), router **Chi** |
| Frontend | Ionic + Angular (última versión, Standalone Components, Signal Store), Capacitor para iOS/Android |
| Datos | PostgreSQL (pgx + SQLC, migraciones SQL idempotentes) + Redis (caché de alimentos frecuentes, rate limiting) |
| IA visión | Gemini API con `response_schema` (JSON estructurado estricto). Ver **Selección de modelos de IA** |
| IA voz | Transcripción **en el dispositivo** (Web Speech API; en la app nativa, plugin de Capacitor), con Whisper vía **Groq** de respaldo → **Gemini en modo texto** (extracción estructurada). Pipeline desacoplado |
| Nutrición | Base nutricional propia en Postgres, alimentada desde USDA FoodData Central / Open Food Facts |
| Pagos | RevenueCat (SDK de Capacitor + webhooks en Go): suscripciones y packs consumibles de créditos |
| Deploy | Railway, Dockerfile multi-stage, binario final < 25 MB |
| E2E | Playwright con emulación móvil, debe poder ejecutarse con `--headed` |

## Estructura del repo (monorepo)

```
backend/
  cmd/api/              # main: cableado de DI, configuración, arranque del servidor
  internal/domain/      # modelos + lógica pura: User, Meal, Food, MacroBudget, DietProfile, OwliMood, CreditLedger
  internal/ports/       # interfaces: inbound (casos de uso) y outbound (repos, clientes IA, transcriptor, auth)
  internal/usecase/     # implementación de los casos de uso (ports inbound): orquesta dominio y ports outbound
  internal/adapters/    # httpapi (Chi), postgres (sqlc/pgx), redisx, jwtauth, oidc (Apple/Google), memory (tests), gemini, anthropic…
  internal/envfile/     # carga de .env solo en desarrollo
  migrations/
app/                    # Ionic + Angular + Capacitor
e2e/                    # Playwright
```

### Reglas de capas (no negociables)
- `internal/domain` **no importa** drivers SQL, `net/http`, Chi, Redis ni SDKs de IA. Solo la stdlib y sus propios paquetes.
- Los adapters dependen de los ports, nunca al revés. El dominio no conoce los adapters.
- Todo el cableado vive en `cmd/api`. No hay singletons globales ni `init()` con efectos secundarios.
- Los endpoints de IA reciben y devuelven structs de Go fuertemente tipados, serializados como JSON determinista. Nunca se pasa `map[string]any` ni texto libre del modelo hacia el dominio.

## Autenticación
- Todos los endpoints, salvo health y webhooks, requieren un usuario autenticado. La identidad se resuelve en un middleware del adapter HTTP y llega al dominio solo como `UserID`.
- Obligatorio en iOS: **Sign in with Apple** si se ofrece cualquier otro login social.
- **JWT propio** (decidido el 3 de noviembre de 2026), detrás del port `AuthVerifier`:
  - Identidad: Sign in with Apple, Google o código de 6 dígitos por correo. Sin contraseñas. Los tokens de Apple y Google se verifican contra sus claves públicas (JWKS) y su `aud`.
  - Sesión: access token JWT de 15 min firmado con `JWT_SECRET` (≥ 32 bytes; variable de Railway en producción, `.env` en desarrollo; con `kid` para rotar). Refresh token opaco de ~60 días: en Postgres solo su hash, rotación en cada uso y detección de reuso.
  - Cuenta anónima al terminar el onboarding, que luego se vincula a Apple o Google sin perder datos.
  - Al eliminar la cuenta se revoca el token de Sign in with Apple (exigencia de Apple).
  - Límite de intentos en Redis sobre los endpoints de autenticación.
- Autorización: cada query filtra por el `user_id` del token. Nunca se confía en un `user_id` que venga en el body o la URL.

## Pipeline de registro de comidas
1. **Captura**: foto comprimida en el cliente, o audio.
2. **Identificación (IA)**: Gemini (o Whisper → Gemini texto) devuelve los alimentos tal como son (con su preparación: "carne del estofado", no "bife"), los gramos, la confianza y **su composición estimada cada 100 g** (proteína, carbos, fibra, grasa, alcohol). **La IA no da calorías**: el dominio las calcula con Atwater (`domain.NutrientEstimate.Per100g`).
3. **Resolución nutricional** (decidido el 9 de noviembre de 2026; antes la base reemplazaba lo que veía la IA por el alimento más parecido, y un estofado terminaba como "bife a la parrilla, puré y ketchup"):
   - Si el nombre coincide con uno del catálogo con **las mismas palabras** (`domain.NamesMatch`: sin acentos, plurales ni "cocido"), se usan los valores de la base (USDA, más precisos). Compartir una palabra no alcanza.
   - Si no, se usa la estimación de la IA, guardada como **alimento privado del usuario** (`domain.NewEstimatedFood`, `foods.owner_user_id`, origen `ai_estimate`). Se reutiliza la próxima vez con los mismos valores. Sin micros y sin alérgenos verificados. No aparece en la búsqueda ni en las sugerencias, y solo su dueño puede anotarlo.
   - Si la estimación no cierra (suma más de 100 g, más fibra que carbos), el usuario elige el alimento a mano.
   - Los macros de la porción se calculan en el dominio a partir de los gramos.
   - Primero se buscan los **alimentos propios** del usuario (sus recetas y lo que ya estimó la IA), después el catálogo.
   - Lo que no está en el catálogo se cuenta en `food_misses` (solo el nombre, sin usuario) para sumar los más pedidos con datos de USDA. Las estimaciones de la IA nunca pasan solas al catálogo general.
4. **Unidades** ("2 huevos", "1 vaso", "3 cucharadas"): la IA da la cantidad, la unidad en singular y plural y el peso de una. Los gramos son siempre cantidad × peso (`RecognizedFood.NormalizeUnits`), y en la confirmación los botones suman o restan de a una unidad. Una cantidad vaga ("un poco") baja la confianza y se resalta.
5. **Recetas** ("hice unos waffles con…"): la IA marca `recipe` (nombre y porciones que rinde) y los ítems son los ingredientes con toda la cantidad.
   - La app pregunta "¿Cuánto comiste?": en porciones si se sabe cuántas rinde (empieza en una), o todo, ¾, la mitad o ¼.
   - `POST /v1/me/recipes` la guarda como **plato propio** (`domain.NewUserRecipe`, origen `user_recipe`, con el peso de una porción) y anota solo lo comido.
   - Volver a dictarla con el mismo nombre la actualiza. "Comí dos waffles proteicos" la reconoce: cada uno pesa una porción de la receta.
   - Los platos propios aparecen primero en la búsqueda manual, marcados.
6. **Confirmación**: el usuario revisa y puede corregir alimentos y gramos antes de guardar. Lo estimado por la IA se marca ("Estimado por IA") y se puede cambiar por un alimento de la base. Los ítems con baja confianza o sin alimento se resaltan.
7. **Persistencia**: solo se guardan las comidas confirmadas.
- **Fotos**: se descartan tras el análisis y no se almacenan.

## Selección de modelos de IA
Precios de referencia de noviembre de 2026. Costo estimado por análisis: ≈ $0.001–0.003.
- **Visión, principal:** `gemini-3.1-flash-lite`. **Confirmado por el benchmark** del 2 de noviembre de 2026 (100 platos de Nutrition5k):

  | Modelo | Error de masa (mediana) | Sesgo de gramos | Recall ponderado | Latencia p50 / p95 | USD / 1000 fotos |
  |---|---|---|---|---|---|
  | **3.1 Flash-Lite** | **21.6 %** | **−0.8 %** | 71 % | 2.4 s / 3.0 s | **$0.55** |
  | 3.5 Flash-Lite | 25.9 % | −19.9 % (subestima) | 72 % | 1.7 s / 2.3 s | $0.70 |
  | 3.8 Flash | 25.4 % | −9.0 % | 75 % | 6.7 s / 10.7 s | $3.01 |

  3.8 Flash cuesta 5,5 veces más y es 3 veces más lento, **sin mejorar la precisión**.

  Segundo benchmark, mismo día y mismas 100 fotos, contra otro proveedor:

  | Modelo | Error de masa (mediana) | Sesgo de gramos | Recall ponderado | Latencia p50 / p95 | USD / 1000 fotos |
  |---|---|---|---|---|---|
  | Gemini 3.1 Flash-Lite | **21.6 %** | −0.8 % | 71 % | 1.9 s / 2.6 s | $0.55 |
  | Claude Haiku 5.5 | 44.3 % | **+39 % (sobreestima)** | 68 % | 1.3 s / 1.9 s | $0.23 |

  Haiku sobreestima de forma sistemática. Aplicando un factor de ×0.71 (calculado con la mitad de los platos y validado con la otra mitad), su error baja a ~31 %, todavía peor que Gemini.
- **Visión, respaldo:** `claude-haiku-5-5` (Anthropic), **solo por disponibilidad**: cuando Gemini falla por cualquier motivo (429, 503, timeout, respuesta inválida) o está mal configurado (key inválida, sin facturación: se registra como error para corregirlo; decidido el 4 de noviembre de 2026, tras una key mal cargada en Railway). Se elige otro proveedor porque los 503 de Google afectan a todos sus modelos a la vez, así que un respaldo dentro de Google no protege. Los gramos de Haiku se multiplican por un **factor de calibración configurable (por defecto 0.71)**. El reintento **no cobra** un crédito extra.
  - Haiku 5.5 rechaza el parámetro `temperature`. La salida estructurada se fuerza con `tool_choice` (ver `adapters/anthropic`).
  - `gemini-3.8-flash` queda descartado: no mejora la precisión, es lento y no acepta `thinkingLevel: minimal`.
- **La `confidence` que devuelve el modelo no está calibrada.** En Gemini nunca bajó de 0.6. En Claude varía más, pero en ambos la correlación con el error real fue ~0 (≤ 0.10). No se usa para tomar decisiones (ni para reintentar ni para saltarse la confirmación). La pantalla de confirmación se muestra **siempre** y debe permitir ajustar los gramos rápido, porque el error típico es de ±20–25 %.
- **Errores transitorios:** Gemini puede devolver 503 ("high demand") o tardar más de 60 s. El adapter los expone como `gemini.ErrUnavailable` y `gemini.ErrRateLimited` (`gemini.IsRetryable`). En la app: timeout de ~15 s → respaldo → si todo falla, reembolso del crédito.
- Benchmark reproducible: `go run ./cmd/benchmark -n 100` (desde `backend/`). Usa Nutrition5k (CC BY 4.0, Thames et al., CVPR 2021). Hay que repetirlo cuando cambie el prompt o salga un modelo nuevo.
- **Voz** (decidido el 4 de noviembre de 2026): primero el **reconocimiento del dispositivo** (gratis, muestra el texto en vivo) → `POST /v1/me/analyses/text` → `gemini-3.1-flash-lite` en modo texto. Si el dispositivo no transcribe (Firefox, error, sin servicio), la app graba el audio → `POST /v1/me/analyses/voice` → Groq `whisper-large-v3-turbo` ($0.04/h, mínimo de 10 s facturados) → Gemini. Sin `GROQ_API_KEY` no hay respaldo; la app esconde el micrófono si no queda ningún camino (`voice` en `GET /v1/me/credits`).
  - En Chrome el reconocimiento del dispositivo lo procesa Google (servicio gratuito, otras condiciones); en Safari, Apple. La app nunca guarda el audio.
  - Whisper propio en Railway quedó descartado por ahora (≈ $15–20/mes de RAM fija, sin GPU y más lento que Groq): ver pendientes.md.
  - El pipeline desacoplado no ahorra dinero frente a mandar el audio directo a Gemini. Se mantiene porque la transcripción se muestra en la pantalla de confirmación y permite resolver frases frecuentes ("lo de siempre") desde la caché, sin llamar al LLM.
- **No usar** Gemini 2.5 Flash / Flash-Lite: están deprecados y desde el 1 de diciembre de 2026 se redirigen a otros modelos.
- **Usar siempre el tier de pago de Gemini**, nunca el gratuito: en el gratuito Google puede usar los datos para mejorar sus modelos, y aquí son datos de salud.
- Los IDs de modelo van en la configuración, no en el código.

## Reglas de negocio clave

### TDEE (Mifflin-St Jeor)
- Hombre: `BMR = 10·peso(kg) + 6.25·altura(cm) − 5·edad + 5`
- Mujer: `BMR = 10·peso(kg) + 6.25·altura(cm) − 5·edad − 161`
- `TDEE = BMR × factor de actividad` (sedentario 1.2, ligero 1.375, moderado 1.55, alto 1.725, muy alto 1.9).
- Ajuste por objetivo: implementado en `domain.GoalPolicy.PlanFor` ([plan.go](backend/internal/domain/plan.go)) con valores **provisionales** (`DefaultGoalPolicy`: 0,25/0,5/0,75 kg/sem para perder, 0,15/0,25/0,4 para ganar, 1100 kcal/día por kg/sem, recomposición −200). Están pendientes de validar con un profesional: no los cambies sin preguntar.
- Nunca por debajo del mínimo seguro; si se recorta, la fecha se recalcula con el ritmo real, y si queda por debajo de 0,1 kg/sem no se muestra fecha.
- **La app replica este cálculo** para mostrarlo en vivo ([plan.ts](app/src/app/core/nutrition/plan.ts)). Las dos implementaciones se prueban con los mismos casos: [plan_cases.json](backend/internal/domain/testdata/plan_cases.json). Si cambia una regla, cambian los dos lados y el archivo de casos.
- Reparto de macros (aprobado el 3 de noviembre de 2026, **provisional** hasta validarlo con un profesional): `domain.DefaultMacroSplit` en [macro_split.go](backend/internal/domain/macro_split.go).
  - Proteína: 2,0 g/kg perdiendo grasa o en recomposición, 1,8 ganando músculo, 1,6 manteniendo; nunca más del 35 % de las kcal.
  - Grasa: al menos el 25 % de las kcal; los carbos completan el resto.
  - Keto: 20 g de carbos netos y la grasa completa. Carnívoro: carbos en cero.
  - La app lo replica ([macros.ts](app/src/app/core/nutrition/macros.ts)) y los dos se prueban con [macro_cases.json](backend/internal/domain/testdata/macro_cases.json).
- Estado de ánimo de Owli: `EvaluateMood` con la proteína objetivo y el ritmo esperado del día según la hora local (`DayProgress`: 0 antes de las 7, 1 después de las 22). Un día pasado se evalúa completo.

### Límites de seguridad
- Nunca se recomienda un objetivo por debajo de un mínimo calórico. Valores por defecto, pendientes de validar con un profesional: 1200 kcal (mujer) y 1500 kcal (hombre).
- Edad mínima: 18 años. Se valida en el onboarding.
- La app muestra un aviso de que no sustituye el consejo médico o nutricional.

### Perfiles de dieta (exclusiones estrictas)
- **Carnívoro estricto**: solo alimentos de origen animal, más agua, sal y suplementos minerales. Nada vegetal, tampoco el café.
- **Keto**: menos de 25 g de carbohidratos netos al día.
- **Vegano**, **Vegetariano**, **Omnívoro balanceado**.
- **Pescetariano**: vegetariano más pescado y marisco.
- **Paleo**: sin cereales, legumbres, lácteos ni azúcar añadido.
- **Restricciones** (sin gluten, sin lactosa): se combinan con cualquier dieta. Si la base no verificó los alérgenos de un alimento, se trata como **no apto** ("no sabemos" equivale a "no"). El filtro único para sugerencias es `domain.Allows(dieta, restricciones, alimento)`.
- El ayuno intermitente no es una dieta: se cubre con la configuración de 1 a 5 comidas al día.
- Las exclusiones se validan en el dominio a partir de los atributos de cada alimento de la base nutricional. Ninguna sugerencia ni respuesta de la IA puede saltárselas. Ejemplo: nunca sugerir carbohidratos vegetales a un usuario carnívoro.

### Meal Closing Engine
- Se configuran de 1 (OMAD) a 5 comidas al día.
- Calcula en tiempo real lo que falta y sugiere platos concretos que lo cubren dentro de una tolerancia de **±5 % de calorías y ±5 g de proteína**, respetando la dieta.
- Implementado en [closing.go](backend/internal/domain/closing.go) (`PlanNextMeal`, `MealNeed`, `DayOver`, `SuggestDishes`) y expuesto en `GET /v1/me/suggestions`:
  - Lo que falta del día se reparte entre las comidas que quedan (anotaciones a menos de 90 min cuentan como una comida).
  - Los platos son plantillas curadas en `data/dishes_catalog.csv` (fuente de proteína + carbo + grasa, con porción mín./máx. por ingrediente). Los gramos se resuelven con un sistema 3×3 (proteína, carbos, grasa) o 2×2 (proteína, kcal).
  - Cada ingrediente pasa por `domain.Allows`. En keto se descartan ingredientes con más de 10 g de carbos netos por 100 g.
  - Hasta 10 platos, como mucho 2 por fuente de proteína. Con menos de 100 kcal pendientes, el día está cerrado.
  - Qué comida es (`PlanNextMeal`): quedan las configuradas menos las hechas, pero no más de las que entran antes de las 22 h con 2 h 30 entre comidas (provisional). Con una sola, no se divide nada. Desayuno: la primera comida antes de las 12 h; merienda: de 16 a 19 h con dos hechas y otra por delante. A esas horas suben los platos de desayuno.
  - Si el día ya pasó la grasa o los carbos (`DayOver`), se descartan los platos que suman más de 10 g de ese macro.
  - Acompañamientos (`kind=side`: fruta, arroz, pan…, sin fuente de proteína) solo cuando a la comida le faltan menos de 15 g de proteína; se escalan por calorías.
  - Anotar un plato sugerido es gratis (origen `suggestion`).

### Owli: Mood Engine y tono
- El estado de ánimo se calcula con reglas de dominio puras a partir del progreso de macros del día. Ejemplos: enérgico si va en rango, cansado ante un déficit severo de proteína, satisfecho al cerrar el balance calórico.
- Los comentarios salen de **plantillas** indexadas por (estado de ánimo × dieta × contexto): no llaman a la IA ni consumen créditos.
- El tono depende de la dieta: cazador/fuerza para Carnívoro, enfoque metabólico para Keto, apoyo estratégico para déficit calórico.

### "Tu semana"
- `GET /v1/me/week?end=AAAA-MM-DD`: los 7 días que terminan en `end` (por defecto, hoy; nunca en el futuro). Implementado en [week.go](backend/internal/domain/week.go) (`ReviewWeek`).
- Un día está **cerrado** con el mismo criterio que el estado "satisfecho" de Owli (calorías ±5 %, proteína a 5 g o más).
- Los días sin anotar no cuentan como "cortos" ni "pasados": son días sin datos. El día en curso no entra en los promedios ni en los cortos.
- Racha: días seguidos anotando; si hoy todavía no se anotó, cuenta hasta ayer.
- Todos los días se comparan con el objetivo **actual** del perfil: todavía no se guarda el historial del plan.

### "Día" del usuario
- Los totales diarios se cortan según la **zona horaria del usuario** (guardada en su perfil), no la del servidor. Los timestamps se almacenan en UTC (`timestamptz`).

### Créditos (Credit Ledger)
- `credit_transactions` es **append-only**: nunca se hace `UPDATE` ni `DELETE` de filas. Las correcciones y los reembolsos son nuevas transacciones compensatorias.
- Antes de cada llamada a Gemini o Whisper se verifica el saldo. Si no alcanza, la llamada no se hace.
- Toda operación que toque créditos va en una transacción atómica, con `SELECT ... FOR UPDATE` sobre la fila del balance o usuario para evitar condiciones de carrera.
- Flujo: reservar/debitar en transacción → llamar a la IA → si la IA falla, registrar un reembolso compensatorio.
- Los webhooks de RevenueCat deben ser **idempotentes**: deduplicar por el ID de evento y verificar la autenticación del webhook.
- Rate limiting en Redis, independiente de los créditos, contra bots y abusos.

### Modelo comercial
- **Principio:** el registro manual (buscar en la base nutricional y anotar) es **siempre gratis**. Solo cuestan los análisis con IA.
- **1 crédito = 1 análisis**, sea foto o voz.

| Plan | Incluye | Precio |
|---|---|---|
| Gratis | 15 créditos de bienvenida + **1 análisis IA al día** (cuota diaria, no acumulable) | $0 |
| Packs consumibles | 50 créditos / 200 créditos. **No caducan nunca** (Apple no permite que caduquen créditos comprados con IAP) | $1.99 / $4.99 |
| Owli Pro | IA **ilimitada con uso justo** (tope técnico de ~25 al día) + funciones premium | **$6.99/mes** · **$44.99/año** (~46 % de descuento) |

- **Por qué 1 al día y no 3** (decidido el 3 de noviembre de 2026): con 3 al día el plan gratis cubre las comidas de casi todos los usuarios y Pro deja de tener sentido. Los 15 de bienvenida dan ~5 días con IA en todas las comidas para generar el hábito. Después, la diferencia entre 1 y todas es el valor de Pro. Se puede probar 1 contra 2 con usuarios reales, porque el valor va en configuración.
- **Comunicación:** en el onboarding solo se anuncian los 15 de bienvenida. La cuota diaria se explica cuando se gasta el último crédito de bienvenida, para no prometer un número que puede cambiar.
- **Orden de consumo:** cuota diaria o suscripción Pro → créditos de bienvenida → créditos comprados.
- Cada movimiento queda en `credit_transactions` con dos campos:
  - `kind`: `grant`, `consume` o `refund`.
  - `bucket`: `daily_quota`, `subscription`, `welcome` o `pack`.
  - Un reembolso devuelve el crédito al mismo bucket del que salió. `reference` es la clave de idempotencia (ID del análisis o del evento de RevenueCat).
  - Implementado en [backend/internal/domain/credits.go](backend/internal/domain/credits.go).
- **No se cobra** cuando la IA falla, cuando no detecta comida, cuando se reintenta con el modelo de respaldo ni por ediciones manuales posteriores.
- Precios, cuotas, tope de uso justo y tamaños de pack van en configuración o en tabla, no fijos en el código. Los precios reales los gestionan App Store, Google Play y RevenueCat.
- El precio anual se validará después del lanzamiento con RevenueCat Experiments (A/B de $39.99 / $44.99 / $49.99). El código no debe asumir un precio concreto: las ofertas se leen de RevenueCat Offerings.

## Frontend / UX
- Modo oscuro de alta legibilidad con acentos cálidos.
- Microinteracciones rápidas, sin animaciones lentas.
- Cámara y micrófono accesibles en **≤ 2 toques** desde que se abre la app.
- Dashboard: barras de macros/calorías y estado de Owli derivados de Angular Signals (usar `computed`, no recalcular a mano).
- Onboarding en 3 pasos: biometría → objetivo → dieta.
- Idioma: español por defecto, con i18n preparado desde el inicio (sin textos fijos en los componentes).
- Unidades: sistema métrico internamente. La UI permite mostrar e introducir datos en imperial.

## Convenciones de código
- Go: errores envueltos con `fmt.Errorf("...: %w", err)`, errores de dominio tipados o centinela, y el mapeo a códigos HTTP solo en el adapter HTTP. Propagar siempre `context.Context`.
- SQL: migraciones idempotentes (`IF NOT EXISTS`, etc.) y queries en archivos `.sql` generados con SQLC. Nada de SQL concatenado.
- Secretos (Gemini, Groq, RevenueCat, DB, auth) solo por variables de entorno. Nunca en el repo.
- Rate limiting en Redis por usuario en los endpoints de IA.
- Tests: el dominio se prueba con tests unitarios puros (table-driven). Los adapters, con tests de integración. Los flujos críticos (onboarding, registro y confirmación de comida, compra de créditos), con Playwright.

## Comandos

Backend (desde `backend/`):
```bash
go test ./...                                   # tests
go vet ./... && gofmt -l .                      # lint (gofmt -l no debe listar nada)
go run ./cmd/api                                # servidor en :8080 (GET /healthz)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
docker build -t owli-api .                      # imagen de producción (distroless)
# Verificar que domain solo use stdlib (debe imprimir solo el propio paquete):
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./internal/domain
```

Entorno de desarrollo (desde la raíz, con Docker Desktop):
```bash
docker compose up -d              # Postgres 16 (puerto 5433) + Redis 7 (6379), credenciales owli/owli
docker compose --profile api up   # además, la API con el Dockerfile de producción
docker compose down -v            # apagar y borrar los datos
```
El Postgres instalado en Windows ocupa el 5432; el de Docker usa el 5433. La VM de Docker está limitada a 4 GB (`%USERPROFILE%\.wslconfig`). En la terminal Bash de Git el CLI de Docker falla: usar PowerShell. Producción: Railway (Postgres y Redis como servicios; sus credenciales nunca van al repo).

Tests de integración de Postgres (se saltean sin la variable; usan la base `owli_test`, que se vacía):
```bash
TEST_DATABASE_URL="postgres://owli:owli@localhost:5433/owli_test?sslmode=disable" go test ./...
```
Base nutricional (desde `backend/`):
- El catálogo curado está en `data/foods_catalog.csv`: nombres en español e inglés, sinónimos regionales, atributos de dieta y alérgenos. Los nutrientes salen de USDA FoodData Central (dominio público) por `fdc_id`.
- Para regenerarlo, descargar en `.fdc-cache/` los JSON de Foundation Foods y SR Legacy (https://fdc.nal.usda.gov/download-datasets) y correr:
  ```bash
  go run ./cmd/importfoods -search "chicken breast roasted"   # elegir la ficha de USDA de un alimento
  go run ./cmd/importfoods                                    # valida el catálogo y escribe data/foods_seed.json
  ```
- Platos que USDA no tiene (chimichurri, milanesas, empanadas, locro…) van en `data/recipes_catalog.csv`: ingredientes de USDA con sus gramos y, si hay cocción, el peso final. Los macros y micros se calculan con `domain.ComposeRecipe`; un micro solo se conoce si todos los ingredientes lo tienen. Un plato nunca lleva alérgenos verificados.
- `data/foods_seed.json` va embebido en el binario. La API lo carga al arrancar solo si cambió (versión por hash en `app_meta`): Railway no necesita los archivos de USDA.
- Un alérgeno solo se marca `verified` en alimentos simples de un ingrediente; los procesados quedan sin verificar.
- Búsqueda: texto normalizado (sin acentos) y similitud por palabra con pg_trgm (umbral 0,5), que tolera errores de tipeo.

SQL tipado con sqlc (sin instalar nada, desde `backend/`): `docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.30.0 generate`. Las migraciones (`migrations/*.sql`, idempotentes) van embebidas en el binario y se aplican al arrancar, con un advisory lock.

Variables de entorno actuales: `JWT_SECRET` (obligatoria, ≥ 32 bytes), `JWT_SECRET_PREVIOUS`, `APPLE_CLIENT_IDS`, `GOOGLE_CLIENT_IDS`, `CORS_ALLOWED_ORIGINS`, `RATE_LIMIT_ANON_PER_IP_HOUR` (10), `RATE_LIMIT_SIGNIN_PER_IP_HOUR` (60), `RATE_LIMIT_USER_PER_MINUTE` (120), `AI_DAILY_BUDGET_USD` (20), `DATABASE_URL`, `REDIS_URL`, `PORT` (8080), `CREDITS_FREE_DAILY_QUOTA` (1), `CREDITS_WELCOME` (15), `CREDITS_PRO_FAIR_USE_DAILY` (25), `APP_ENV`, `GEMINI_API_KEY`, `GEMINI_MODEL` (gemini-3.1-flash-lite), `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL` (claude-haiku-5-5), `AI_PRIMARY_TIMEOUT_MS` (15000), `AI_FALLBACK_TIMEOUT_MS` (20000), `AI_FALLBACK_GRAMS_FACTOR` (0.71), `AI_COST_MICRO_USD` (3000), `RATE_LIMIT_AI_PER_USER_HOUR` (30, foto y voz juntas), `GROQ_API_KEY`, `GROQ_MODEL` (whisper-large-v3-turbo), `AI_VOICE_COST_MICRO_USD` (1000), `AI_TRANSCRIBE_TIMEOUT_MS` (15000), `AI_TEXT_COST_MICRO_USD` (500), `AI_FAKE`.

IA simulada: con `AI_FAKE=1` la API usa `adapters/fakeai` (determinista, sin costo; la usan los e2e). La API se niega a arrancar con `AI_FAKE=1` y `APP_ENV=production`. Según el contenido de la imagen: `FAIL` → IA no disponible, `SLOW` → supera el timeout, `EMPTY` → sin comida; cualquier otra → pollo, arroz y un "glaseado" que no está en la base. En voz: audio `FAIL` → transcripción caída, `SILENCE` → nada que analizar; texto con `NADA` → sin comida.

Créditos: `credit_transactions` (migración 0004) rechaza `UPDATE` con un trigger; el débito bloquea la fila del usuario con `FOR UPDATE` y elige el bucket con `domain.CreditPolicy`. Endpoints: `GET /v1/me/credits`, `POST /v1/me/analyses/photo` (multipart `photo`, máx. 4 MB, jpeg/png/webp) y `POST /v1/me/analyses/voice` (multipart `audio`, máx. 5 MB, webm/ogg/mp4/mpeg/wav; la app corta a los 60 s). En voz no se aplica el factor de gramos del respaldo: se midió con fotos.

App (desde `app/`):
```bash
npx ng serve --port 8100        # la app en http://localhost:8100 (en desarrollo hay un botón de perfil de prueba)
npx ng test --watch=false       # tests unitarios (Vitest)
npm run lint                    # ESLint + Prettier (prefijo de componentes: owli-)
npx ng build                    # compilación de producción en app/www
```
- Estado: Signal Store (`@ngrx/signals`). Textos: Transloco, en `src/assets/i18n/{es,en}.json` (nunca texto fijo en los componentes).
- Tema: `src/theme/variables.scss` (tokens `--owli-*`). Modo oscuro siempre, estilo iOS en ambas plataformas.

Pruebas de punta a punta (desde `e2e/`, con `docker compose up -d`). Levantan su propia API (puerto 8090, base `owli_e2e`, Redis db 1) y su propia app (puerto 4310, `ng serve --configuration e2e`); nunca reutilizan un servidor existente:
```bash
npm test                        # Playwright en tamaño iPhone (Chromium) y Pixel 7
npm run test:headed             # viendo el navegador
```

