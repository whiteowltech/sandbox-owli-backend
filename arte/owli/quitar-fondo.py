"""Saca el fondo magenta de una escena de Owli, recorta al personaje y la exporta como PNG transparente.

Uso: python key.py ENTRADA SALIDA [lado_max]
"""
import sys

import numpy as np
from PIL import Image

KEY = np.array([255.0, 0.0, 255.0])

src, dst = sys.argv[1], sys.argv[2]
side = int(sys.argv[3]) if len(sys.argv) > 3 else 600

rgb = np.asarray(Image.open(src).convert("RGB")).astype(np.float64)
# "Cuánto magenta" tiene el píxel: min(R, B) − G. El fondo da ~255 y ningún color de
# Owli pasa de 0 (marrón, beige, ámbar, celeste, verde), así que un borde mezclado con
# el fondo da (1 − alfa)·255 y el alfa se despeja directo.
m = np.minimum(rgb[..., 0], rgb[..., 2]) - rgb[..., 1]
# El fondo generado tiene grano: se toma como "fondo puro" el valor bajo del margen
# superior y los laterales (abajo están las patas).
h = rgb.shape[0]
margin = np.concatenate([m[:24].ravel(), m[: h // 2, :24].ravel(), m[: h // 2, -24:].ravel()])
bg = np.percentile(margin, 1) - 4
a = np.clip((bg - m) / bg, 0.0, 1.0)
a[a < 0.12] = 0.0  # grano suelto del fondo

# Quitar el tinte magenta del borde: el color observado es a*c + (1-a)*key.
safe = np.maximum(a, 1e-3)[..., None]
col = np.clip((rgb - (1.0 - a[..., None]) * KEY) / safe, 0, 255)
col[a == 0] = 0

out = np.dstack([col, a * 255.0]).astype(np.uint8)
im = Image.fromarray(out, "RGBA")

alpha = im.split()[3].point(lambda v: 255 if v > 8 else 0)
box = alpha.getbbox()
pad = 6
box = (max(0, box[0] - pad), max(0, box[1] - pad), min(im.width, box[2] + pad), min(im.height, box[3] + pad))
im = im.crop(box)
scale = side / max(im.size)
if scale < 1:
    im = im.resize((round(im.width * scale), round(im.height * scale)), Image.LANCZOS)
im.save(dst, optimize=True)
print(dst, im.size)
