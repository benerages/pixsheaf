import sys, os
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.boundsPen import BoundsPen

OUT = sys.argv[1]
FONT = "/usr/share/fonts/opentype/inter/Inter-SemiBold.otf"
font = TTFont(FONT)
gs = font.getGlyphSet()
cmap = font.getBestCmap()
upm = font["head"].unitsPerEm

def text_path(text, size, x0, baseline, tracking=-0.01):
    scale = size / upm
    pen = SVGPathPen(gs)
    x = 0
    for ch in text:
        g = cmap[ord(ch)]
        tp = TransformPen(pen, (scale, 0, 0, -scale, x0 + x * scale, baseline))
        gs[g].draw(tp)
        x += gs[g].width + tracking * upm
    return pen.getCommands(), (x - tracking * upm) * scale

# Palette
TEAL_DARK = "#0F6E56"; TEAL = "#1D9E75"; TEAL_LIGHT = "#5DCAA5"; AMBER = "#EF9F27"
INK = "#12302A"; INK_DARK_BG = "#EAF5F1"
# Dark variant: lighter cards for contrast on dark backgrounds
D_FRONT = "#1D9E75"; D_MID = "#5DCAA5"; D_BACK = "#9FE1CB"

CARD = 'x="16" y="6" width="32" height="38" rx="4.5"'
MTN = '<path d="M20.5 30 L29 19.5 L34.5 25 L38.5 21 L44 30 Z" fill="#FFFFFF"/>'
MTN_SIMPLE = '<path d="M20 31 L29.5 18.5 L35.5 25.5 L39 22 L44.5 31 Z" fill="#FFFFFF"/>'
BAND = '<rect x="13" y="34" width="38" height="6.5" rx="3.25" fill="{c}"/>'

def mark(front, mid, back, band=AMBER, sun=AMBER, tx=0, ty=0, s=1.0, sun_on=True, simple=False):
    # 64x64 grid. A sheaf: cards fan out around the tie point (32,37);
    # the amber band binds them at the waist.
    g = f'<g transform="translate({tx} {ty}) scale({s})"><g transform="translate(0 6)">'
    g += f'<rect {CARD} fill="{back}" transform="rotate(-16 32 37)"/>'
    g += f'<rect {CARD} fill="{mid}" transform="rotate(16 32 37)"/>'
    g += f'<rect {CARD} fill="{front}"/>'
    if simple:
        g += MTN_SIMPLE
    else:
        g += MTN
        if sun_on:
            g += f'<circle cx="41.5" cy="12" r="2.8" fill="{sun}"/>'
    g += BAND.format(c=band)
    g += '</g></g>'
    return g

def write(name, w, h, body, title):
    svg = (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" role="img" aria-labelledby="t">'
           f'<title id="t">{title}</title>{body}</svg>\n')
    open(os.path.join(OUT, name), "w").write(svg)

# 1) Mark only
write("pixsheaf-mark.svg", 64, 64, mark(TEAL_DARK, TEAL, TEAL_LIGHT), "Pixsheaf")
write("pixsheaf-mark-dark.svg", 64, 64, mark(D_FRONT, D_MID, D_BACK), "Pixsheaf")

# 2) Favicon: same mark, switches colors with the browser color scheme
fav_body = ('<style>.f{fill:%s}.m{fill:%s}.b{fill:%s}@media (prefers-color-scheme:dark){.f{fill:%s}.m{fill:%s}.b{fill:%s}}</style>'
            % (TEAL_DARK, TEAL, TEAL_LIGHT, D_FRONT, D_MID, D_BACK))
fav_body += ('<g transform="translate(0 6)">'
             f'<rect {CARD} class="b" transform="rotate(-16 32 37)"/>'
             f'<rect {CARD} class="m" transform="rotate(16 32 37)"/>'
             f'<rect {CARD} class="f"/>' + MTN_SIMPLE + BAND.format(c=AMBER) + '</g>')
write("favicon.svg", 64, 64, fav_body, "Pixsheaf")
write("pixsheaf-mark-small.svg", 64, 64, mark(TEAL_DARK, TEAL, TEAL_LIGHT, simple=True), "Pixsheaf")
write("pixsheaf-mark-small-dark.svg", 64, 64, mark(D_FRONT, D_MID, D_BACK, simple=True), "Pixsheaf")

# 3) Horizontal logo (mark + wordmark)
H = 64
d, tw = text_path("pixsheaf", 40, 72, 42)
W = int(72 + tw + 6)
write("pixsheaf-logo.svg", W, H, mark(TEAL_DARK, TEAL, TEAL_LIGHT) + f'<path d="{d}" fill="{INK}"/>', "Pixsheaf")
write("pixsheaf-logo-dark.svg", W, H, mark(D_FRONT, D_MID, D_BACK) + f'<path d="{d}" fill="{INK_DARK_BG}"/>', "Pixsheaf")

# 4) Stacked logo with tagline (for README header / social)
d2, tw2 = text_path("pixsheaf", 44, 0, 0)
d3, tw3 = text_path("convert, resize, optimize · self-hosted", 14, 0, 0, tracking=0.0)
SW = int(max(tw2, tw3) + 40)
def stacked(front, mid, back, ink, sub):
    b = mark(front, mid, back, tx=(SW-96)/2, ty=0, s=1.5)
    b += f'<g transform="translate({(SW-tw2)/2} 136)"><path d="{d2}" fill="{ink}"/></g>'
    b += f'<g transform="translate({(SW-tw3)/2} 162)"><path d="{d3}" fill="{sub}"/></g>'
    return b
write("pixsheaf-logo-stacked.svg", SW, 176, stacked(TEAL_DARK, TEAL, TEAL_LIGHT, INK, "#4A6B62"), "Pixsheaf — convert, resize, optimize · self-hosted")
write("pixsheaf-logo-stacked-dark.svg", SW, 176, stacked(D_FRONT, D_MID, D_BACK, INK_DARK_BG, "#A7C4BB"), "Pixsheaf — convert, resize, optimize · self-hosted")
print("ok", W, SW)
