# Pendientes de Owli

Última actualización: 3 de noviembre de 2026.

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

- [ ] **Owli reacciona a lo que haces**: salta al guardar una comida, se le iluminan los ojos al cerrar el día, bosteza cuando falta proteína.
- [ ] **Transiciones entre estados de ánimo**: pasar de cansado a satisfecho con animación, no de golpe.
- [ ] **Ilustración según el contexto**: como el mapache que duerme en la pantalla de ayuno o bucea en la de hidratación. Owli podría tener escenas para los micros, la semana o el cierre del día.
- [ ] **Celebración breve al cerrar el día**: una sola, corta, que se pueda saltar.
- [ ] **Microinteracciones**: barras que se llenan, el número de calorías que cuenta hacia abajo, la hoja de confirmación que sube.
- [ ] **Herramienta**: Rive (máquina de estados, liviano, funciona en iOS y Android dentro de Capacitor).
  - Requiere que un ilustrador arme a Owli por partes (ojos, alas, cuerpo) para animarlo.
  - Respetar `prefers-reduced-motion` y no usar animaciones lentas (regla de CLAUDE.md).

## Producto

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
- [ ] **Motor de sugerencias, fase 1.5** (visto con datos reales el 4 de noviembre de 2026: con carbos 2/192 g y grasa 97/53 g sugirió huevos con manteca):
  - Descartar platos que suman a un macro ya pasado (grasa pasada → nada con manteca, aceite, palta ni panceta).
  - Acompañamientos sin fuente de proteína para cuando faltan carbos o calorías (fruta, arroz, papa, pan, yogur con fruta).
  - Deducir qué comida es por las comidas hechas y el tiempo desde la última, no solo por la hora (de 16 a 19 h hoy asume merienda). En la última comida del día, no dividir.
  - Explicar arriba de la lista qué falta para esta comida y por qué.
- [ ] **Crear alimentos propios** en el registro manual cuando no están en la base.

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
