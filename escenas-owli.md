# Escenas de Owli generadas con IA

Cómo generar las escenas nuevas sin que Owli cambie de cara. Decidido el 9 de noviembre de 2026: imágenes generadas con IA en lugar de Rive.

## Estado

Las 6 escenas se generaron el 9 de noviembre de 2026 en Buzzy con **Nano Banana 2.1** (1K, 1:1, las 4 imágenes como referencia), sin retoques. El fondo se quita con [arte/owli/quitar-fondo.py](arte/owli/quitar-fondo.py) (`python arte/owli/quitar-fondo.py ORIGINAL SALIDA 600`, requiere Pillow y numpy): alfa = cuánto magenta tiene cada píxel, porque ningún color de Owli tiene magenta. Las imágenes finales miden 600 px de alto.

## Cómo se usa

1. Abrir el generador de imágenes (por ejemplo Gemini) y **adjuntar siempre las 4 imágenes actuales** como referencia: `app/src/assets/owli/owli-{neutral,energetic,tired,satisfied}.png`.
2. Pegar el **bloque de estilo** y, debajo, el prompt de la escena.
3. Generar 3–4 variantes y elegir la que más se parezca a las de referencia, comparándolas lado a lado. Si una cambia la forma de la cabeza, los ojos o la panza, se descarta aunque sea linda.
4. Guardar el original en `arte/owli/originales/` y pasármelo. Yo le saco el fondo magenta, lo recorto con el mismo encuadre, lo exporto a 512 px y lo conecto en la app.

## Bloque de estilo (siempre igual)

```
Use the attached images as the exact character reference. This is "Owli", a friendly owl mascot.
Keep the character IDENTICAL: same body shape (round, egg-shaped body, slightly wider at the bottom),
two pointed ear tufts, large beige facial disc split into two circles, big amber-orange eyes
with dark pupils and one small white highlight, small orange triangular beak, beige belly with
exactly two rows of brown scalloped feather marks, brown wings at the sides, four small orange toes
(two per foot) at the bottom.
Colors only: brown #7B6150, darker brown #6A5243 for shading, beige #D6C2A1, amber #F2B33D,
orange #F0A535, near-black pupils #1E2A26.
Style: flat vector illustration, no outlines, soft subtle paper grain, very simple shapes, no
gradients, no 3D, no realistic feathers, no text, no letters.
Framing: front view, the whole owl centered, feet touching the bottom of the canvas,
square 1:1 canvas, the owl fills about 85% of the height.
Background: solid flat magenta #FF00FF, nothing else in the background, no shadow on the ground.
Any prop must be small, simple and in the same flat style, using only these colors plus
water blue #9DB4D9 and leaf green #86B39F when needed.
```

## Escenas

### 1. Agua (`owli-water.png`)
Para la meta de agua en Hoy.
```
Owli holds a simple clear glass of water with both wings in front of the belly, looking pleased,
eyes open and bright. Water drawn as a flat light blue shape #9DB4D9 inside the glass.
```

### 2. Día cerrado (`owli-closed.png`)
Para el festejo al cerrar el día.
```
Owli celebrates: both wings raised up and open, eyes closed in happy upward arcs (like the
"satisfied" reference), beak slightly open. Four or five tiny flat sparkles around the head in
amber #F2B33D and leaf green #86B39F.
```

### 3. De noche (`owli-night.png`)
Después de las 22 h, cuando el día ya no se puede cerrar.
```
Owli is sleeping peacefully: eyes closed as two gentle downward arcs, wings folded, head slightly
tilted. A small flat crescent moon in beige #D6C2A1 floats beside the head. No "Z" letters.
```

### 4. Tu semana (`owli-week.png`)
Encabezado de "Tu semana".
```
Owli holds a small simple clipboard with one wing and points at it with the other wing tip,
attentive look, eyes open. The clipboard shows only three flat horizontal bars of different
lengths (no text, no numbers).
```

### 5. Sin análisis (`owli-no-credits.png`)
Cuando se acaban los análisis con IA. Tiene que transmitir "no pasa nada", no culpa.
```
Owli gives a small gentle shrug: wings slightly open to the sides, palms up, head a little tilted,
calm kind expression with eyes open and a tiny smile. Not sad, not crying.
```

### 6. Micros (`owli-micros.png`)
Para el panel de micronutrientes.
```
Owli looks through a small round magnifying glass held with one wing at a tiny flat green leaf
#86B39F and a small amber berry #F2B33D. Curious expression, one eye enlarged by the glass.
```

## Reglas

- No pedir texto dentro de la imagen: la IA lo deforma y además rompe el i18n.
- Una escena nueva no reemplaza a los 4 estados de ánimo: el ánimo se sigue mostrando con las imágenes actuales.
- El arte generado con IA tiene protección de copyright limitada. El logo y la marca registrada no deberían depender de estas imágenes.
