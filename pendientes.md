# Pendientes de Owli

Última actualización: 5 de noviembre de 2026.

## Decisiones abiertas

- [ ] **Beneficios de Owli Pro.** Propuesta actual (en el prototipo v5): IA en todas las comidas (uso justo de 25 al día), sugerencias para cerrar el día sin límite, historial completo y micronutrientes en promedio semanal. Falta confirmarla.
- [ ] **Micronutrientes: qué es gratis y qué es Pro.** Propuesta del prototipo:
  - Gratis: los micros clave de tu dieta, solo del día, y "Mis suplementos".
  - Pro: promedio de 7 días, panel completo y tendencias.
- [ ] **Validar con un profesional** el ajuste por objetivo (`DefaultGoalPolicy`) y el reparto de macros (`DefaultMacroSplit`). Ya están implementados en el dominio y en la app, marcados como provisionales.
- [ ] **Mínimos calóricos** (1200 / 1500 kcal): validar con un profesional.
- [x] **Autenticación**: JWT propio (ver CLAUDE.md). Falta elegir el servicio de correo para los códigos de acceso (Resend, Postmark…).
- [ ] **Referencias de micronutrientes**: NIH (Dietary Reference Intakes) o EFSA según el país del usuario.

## Animaciones (inspiradas en BitePal)

El video de referencia de BitePal (`parte 1.mp4` y `parte 2.mp4`, en la raíz y fuera de git) muestra una mascota que reacciona y cambia de escena según el tema. Para Owli:

- [x] **Fase A, movimiento y terminación** (5 de noviembre de 2026), sin librerías nuevas (`animate.enter`/`animate.leave` de Angular):
  - Hojas que suben con el fondo oscurecido y se cierran arrastrando desde la manija o el encabezado (`owliSheetDrag`).
  - Calorías que cuentan hasta el valor nuevo (`tweened`); el lector de pantalla anuncia solo el final.
  - Comida nueva resaltada; borrar ofrece "Deshacer" 5 s (vuelve a anotarla con su hora y su origen).
  - Owli se funde de un ánimo al otro y salta al anotar; vibración suave (`HapticsService`).
  - Celebración corta al cerrar el día (cartel y chispas, se cierra con un toque).
  - Presentación del plan al terminar el onboarding: barra de macros con gramos y curva de peso hasta la fecha.
  - Botones que se hunden al tocarlos.
- [x] **Escenas de Owli generadas con IA** (9 de noviembre de 2026, Buzzy con Nano Banana 2.1; guía y prompts en [escenas-owli.md](escenas-owli.md), originales en `arte/owli/originales/`). Las 6 están en `app/src/assets/owli/` y el componente de Owli acepta `scene`. Conectadas: día cerrado (festejo) y sin análisis. Falta decidir cuándo se muestran noche (¿después de las 22 h, aunque el ánimo sea cansado?) y semana (¿reemplaza al ánimo de la semana?); agua y micros esperan a sus funciones.
- [ ] **Herramienta**: Rive, descartado por ahora (9 de noviembre de 2026: se eligieron imágenes generadas con IA). Se retoma si se contrata un ilustrador que arme a Owli por partes.
  - Respetar `prefers-reduced-motion` y no usar animaciones lentas (regla de CLAUDE.md).

## Producto

- [ ] **Agua** (decidido el 5 de noviembre de 2026): meta diaria y botones rápidos (+250 ml, +500 ml) en Hoy, gratis. Falta definir la meta por defecto (p. ej. 35 ml/kg, validar con un profesional) y si cuenta el agua de los alimentos.
- [ ] **Fase B**: cambiar los gramos de una comida anotada, recientes y "repetir" en la búsqueda, agrupar en comidas, peso con su gráfico, aviso "Sin análisis" y micros con suplementos.
- [ ] **Agrupar los alimentos en comidas** (desayuno, almuerzo…). Hoy el ritmo del día para el ánimo de Owli sale de la hora; con comidas se puede usar "comidas hechas / comidas por día".

- [x] **Sugerencias de platos (top 10)**: Meal Closing Engine con 49 platos curados, en el backend y en la app ("Ver qué comer ahora").
- [ ] **Sugerencias, fase 2**: alimentos que el usuario come seguido y los que no le gustan; desayunos veganos (no hay leche vegetal en el catálogo); platos regionales como plantillas (milanesa con puré, empanadas); que Owli comente la mejor opción con su tono de dieta.
- [ ] **Micronutrientes**: reglas aprendidas en el prototipo, a llevar al dominio:
  - Solo se sugiere una fuente si aporta de verdad: al menos el 5 % de la referencia diaria cada 100 kcal. Si no hay ninguna en la dieta (B12 en vegano, vitamina C en carnívoro), Owli lo dice y deriva a un profesional; nunca inventa una fuente.
  - Fuentes poco absorbibles fuera de las sugerencias: por ejemplo, el calcio de la espinaca.
  - En keto y carnívoro el sodio no se marca como exceso.
- [ ] **Micronutrientes, fase 2**:
  - Panel completo y tendencias.
  - Que el cierre del día desempate por micros ("este plato te suma potasio").
- [x] **Tu semana**: 7 días con calorías y proteína contra el objetivo, días cerrados, racha y tabla accesible.
- [ ] **Historial del plan**: guardar el objetivo de cada día para que "Tu semana" compare cada día con el plan vigente ese día (hoy usa el actual).
- [x] **Recalcular el plan** desde la cuenta sin repetir el onboarding: "Tu cuenta" edita cuerpo, objetivo, dieta, comidas por día y nombre con el objetivo recalculado en vivo; también créditos, idioma, unidades y borrar la cuenta (4 de noviembre de 2026). Las comidas por día ahora se eligen en el onboarding (antes quedaban fijas en 4).
- [x] **Motor de sugerencias, fase 1.5** (caso real del 4 de noviembre de 2026: con carbos 2/192 g y grasa 97/53 g sugería huevos con manteca). Descarta platos que suman a un macro ya pasado, ofrece acompañamientos sin proteína, deduce la comida por lo anotado y la hora (la última no se divide) y explica arriba de la lista por qué.
- [ ] **Validar con un profesional** las reglas nuevas del motor: 22 h como última comida, 2 h 30 entre comidas, 10 g de margen para un macro pasado y 15 g de proteína para ofrecer acompañamientos.
- [x] **Alimentos que no están en la base** (9 de noviembre de 2026): en foto y voz, la IA estima la composición cada 100 g y se guarda como alimento privado del usuario; el catálogo solo se usa si el nombre coincide con las mismas palabras (ver CLAUDE.md, "Pipeline de registro").
- [ ] **Repetir el benchmark** (`go run ./cmd/benchmark -n 100`): el prompt cambió (preparaciones y `per_100g`) y puede mover el error de gramos.
- [x] **Unidades, recetas y platos propios** (10 de noviembre de 2026): "2 huevos" en unidades, recetas con "¿cuánto comiste?" guardadas como platos propios, que la app reconoce la próxima vez y muestra primero en la búsqueda. Lo que falta en el catálogo se cuenta en `food_misses`.
- [ ] **Sumar al catálogo los más pedidos** de `food_misses` con datos de USDA (revisar la tabla cada tanto; hoy no hay pantalla).
- [ ] **Editar mis platos**: corregir los valores de un alimento estimado, renombrar o borrar una receta, y ver sus ingredientes.
- [ ] **Crear alimentos propios** a mano en el registro manual cuando no están en la base.

## Cuentas y sesiones

- [x] **URL de la API en Railway** en `environment.prod.ts` (sandbox: `sandbox-owli-backend-production.up.railway.app`).
- [ ] **Guardar la sesión en el almacenamiento seguro** del teléfono (Keychain en iOS, Keystore en Android) en vez de `localStorage`, al armar la app nativa.
- [ ] **Revocar el token de Sign in with Apple al eliminar la cuenta** (exigencia de Apple). Necesita la clave privada de Sign in with Apple de la cuenta de desarrollador.
- [ ] **Botones de Apple y Google en la app** (pantalla "¿Guardo tu progreso?"). El backend ya verifica los tokens; falta configurar los client IDs en las consolas de Apple y Google.
- [ ] **Limpiar cuentas anónimas abandonadas** (sin perfil o sin actividad en N meses) con un job periódico.
- [ ] Si un usuario anónimo pierde su sesión (borra los datos de la app), su cuenta queda huérfana: vincular Apple o Google pronto lo evita. Ofrecerlo después de unos días de uso.

## Base nutricional

- [x] Catálogo inicial: 118 alimentos de USDA con nombres en español e inglés, sinónimos, macros, micros, atributos de dieta y alérgenos.
- [x] Platos rioplatenses: 22 recetas compuestas (chimichurri, milanesas, empanadas, choripán, provoleta, locro…) y 9 alimentos de USDA (asado de tira, matambre, pizza, medialuna, flan…).
- [ ] **Ampliar el catálogo**: mate y yerba (USDA no los tiene), facturas, más cortes de carne, verduras y frutas regionales, y platos de otros países de la región. Revisar las recetas con un nutricionista.
- [ ] **Open Food Facts** para productos envasados (código de barras), con la regla de que "sin dato" nunca es cero.
- [ ] **Caché en Redis** de los alimentos más buscados (CLAUDE.md lo prevé; con el catálogo actual Postgres responde al instante).
- [ ] **Alimentos propios**: crear uno con sus macros cuando no está en la base.
- [ ] **Anotar sin conexión**: guardar en el teléfono y subir cuando vuelva internet (hoy anotar a mano requiere conexión).
- [ ] **Recetas curadas** para las sugerencias (ver Producto).

## Técnico

- [x] Docker con Postgres y Redis para desarrollo local.
- [x] sqlc, migraciones y repositorios de Postgres.
- [x] App Ionic + Angular, a partir del prototipo v5.
- [x] Registro por foto: libro de créditos (15 de bienvenida + 1 al día), Gemini con respaldo en Haiku, pantalla de confirmación y tope de gasto diario.
- [ ] RevenueCat: suscripción, packs y webhooks idempotentes. Hoy `IsPro` es siempre `false` y los packs no se pueden comprar.
- [x] **Registro por voz**: Groq Whisper → Gemini texto (respaldo Haiku), con la transcripción en la confirmación.
- [ ] **Caché de frases frecuentes** en voz ("lo de siempre", "lo mismo que ayer"): resolverlas sin llamar al LLM.
- [x] **Voz en el dispositivo** (Web Speech API) como camino principal; Groq de respaldo.
- [ ] **Voz en la app nativa**: plugin `@capacitor-community/speech-recognition` detrás de `DeviceSpeech` (misma interfaz). En iOS, pedir el reconocimiento en el dispositivo.
- [ ] **Probar la voz en teléfonos reales**: Chrome en Android y Safari en iPhone (reconocimiento del navegador), y el respaldo con Groq real (falta `GROQ_API_KEY` en `.env` y en Railway).
- [ ] **Whisper propio en Railway** (opción evaluada el 4 de noviembre de 2026, descartada por ahora): faster-whisper en un servicio aparte detrás del port `Transcriber`. Cuesta ≈ $15–20/mes de RAM fija (1,5–2 GB), sin GPU tarda 2–10 s por frase. Revisarlo si el uso del respaldo crece tanto que Groq cueste más que eso.
- [ ] **Cámara nativa con Capacitor** (hoy es el `<input type=file capture>` del navegador) y permisos de cámara en iOS y Android.
- [ ] **Explicar la cuota diaria** cuando se gasta el último crédito de bienvenida (CLAUDE.md, Comunicación).
- [ ] Probar el análisis por foto con Gemini real desde el teléfono (las pruebas usan la IA simulada).
- [x] Playwright para onboarding, registro manual, foto, voz, sugerencias y semana (44 pruebas).
- [ ] Playwright para la compra (cuando esté RevenueCat).
- [ ] Repetir el benchmark con fotos reales de celular, no solo Nutrition5k.
- [ ] Ejecutar `/setup-ui-ux-skills` (respuestas recomendadas: estética propia, Owli exento de la regla de mascotas, WCAG AA, voz directa, Ionicons, pantallas de 375 y 430 px).

## Abuso y multicuenta

Un usuario normal no gana nada con varias cuentas (pierde su historial; < $0,05 de IA por cuenta). El riesgo real son los bots creando cuentas anónimas en cadena.

- [x] **Con la autenticación**: tope global de gasto diario en IA y límites de cuentas nuevas, de inicios de sesión y de análisis por IP y por usuario, en Redis.
- [ ] Límite por **dispositivo** (necesita la app nativa: Play Integrity / App Attest, abajo).
- [ ] **Con la app nativa**: Play Integrity (Android) y App Attest (iOS) al crear cuentas y en los endpoints de IA.
- [ ] **Créditos de bienvenida, una sola vez**: por dispositivo con DeviceCheck en iOS (sobrevive a reinstalar) y por identidad al vincular Apple o Google.
- [ ] **Correo**: bloquear dominios descartables y normalizar `nombre+algo@gmail.com`.
- No pedir teléfono ni documento para registrarse: es la fricción que no queremos.

## App Store

- [ ] Etiquetas de privacidad (Privacy Nutrition Labels): datos de salud y fotos que no se guardan.
- [x] Eliminar la cuenta desde la app (borra también la cuenta del servidor).
- [ ] Sign in with Apple.
- [ ] Pedir calificación solo con el aviso nativo, nunca en el onboarding.
- [ ] Textos de permisos de HealthKit, cámara, micrófono y notificaciones.

## Marca

- [ ] Ilustrador para la versión final de Owli (el arte generado con IA tiene protección de copyright limitada).
- [ ] Registro de la marca "Owli".
