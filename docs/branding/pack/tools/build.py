"""Generate the Telecraft logo pack from its construction.

Every SVG in `docs/branding/pack/` is written by this script, never edited by
hand: the geometry below is the source of truth, the files are its render.
The wordmark's letterforms are pulled from the vendored brand face
(`console/src/fonts/AtkinsonHyperlegibleNext.woff2`) and shaped by HarfBuzz,
so the outlines in the pack are exactly the ones the console renders: no
system font, no substitute, no `<text>` element left to the reader's machine.

Requires: fonttools, brotli, uharfbuzz (pip; used only at build time).
Run from the repository root:

    python3 docs/branding/pack/tools/build.py

Colour values are the tokens in `console/src/tokens.css`. If the tokens move,
re-run this script; the pack must never disagree with the stylesheet.
"""

import io
import json
import re
from pathlib import Path

import uharfbuzz as hb
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.boundsPen import BoundsPen

ROOT = Path(__file__).resolve().parents[4]
FONT = ROOT / "console/src/fonts/AtkinsonHyperlegibleNext.woff2"
PACK = ROOT / "docs/branding/pack"

# ── Construction ────────────────────────────────────────────────────────────
# The mark: three reading bands measured against a datum, on a 32-unit grid.
# The bands are the card face's three readings (ADR-0004, ADR-0041 §2),
# decreasing in length and in tone; the datum is the vertical line they are
# all read from, in the brand amber. A reading measured against a datum is
# the product's whole thesis (docs/branding/identity.md).
DATUM = dict(x=7, y=8, w=2, h=16, rx=1)
BANDS = [  # x, y, width; height 3, rx 1.5; tone ink → muted → faint
    dict(x=12, y=8, w=13),
    dict(x=12, y=14.5, w=10),
    dict(x=12, y=21, w=7),
]
BAND_H, BAND_RX = 3, 1.5
PLATE_RX = 7  # plates, not pills (design-system.md)

# The wordmark: "telecraft", Atkinson Hyperlegible Next at wght 600,
# tracked -12/1000. One drawn intervention: the crossbars of "f" and "t" in
# "craft" join into a single continuous bar: a craftsman's join, placed on
# the syllable that says craft.
WORDMARK_TEXT = "telecraft"
WGHT = 600
TRACKING = -12
TELE_GLYPHS = 4  # "tele", the first syllable, carries --brand on marketing surfaces

# Tokens (console/src/tokens.css). ink/muted/faint are --colour-text{,-muted,-faint}.
DARKG = dict(ground="#0f1518", ink="#e9efee", muted="#a0aeae", faint="#8b9a99",
             brand="#ffd164")
LIGHTG = dict(ground="#f3f5f4", ink="#101718", muted="#4e5a5b", faint="#616d6e",
              brand="#8a5a12", rule="#d3dbd9")


# ── Type ────────────────────────────────────────────────────────────────────

def shape_wordmark():
    font = TTFont(FONT)
    font.flavor = None
    instantiateVariableFont(font, {"wght": WGHT}, inplace=True)
    buf = io.BytesIO()
    font.save(buf)

    face = hb.Face(buf.getvalue())
    hbfont = hb.Font(face)
    hbbuf = hb.Buffer()
    hbbuf.add_str(WORDMARK_TEXT)
    hbbuf.guess_segment_properties()
    hb.shape(hbfont, hbbuf, {"kern": True, "liga": True})

    glyph_set = font.getGlyphSet()
    order = font.getGlyphOrder()
    glyphs, x = [], 0
    for info, pos in zip(hbbuf.glyph_infos, hbbuf.glyph_positions):
        gname = order[info.codepoint]
        pen = SVGPathPen(glyph_set)
        glyph_set[gname].draw(pen)
        bp = BoundsPen(glyph_set)
        glyph_set[gname].draw(bp)
        glyphs.append(dict(glyph=gname, path=pen.getCommands(),
                           x=x + pos.x_offset, bounds=bp.bounds))
        x += pos.x_advance + TRACKING
    return glyphs, x - TRACKING


def round1(path):
    return re.sub(r"-?\d+\.?\d*(?:e-?\d+)?",
                  lambda m: f"{float(m.group()):.1f}".rstrip("0").rstrip("."), path)


class Wordmark:
    """The composed wordmark in y-down coordinates, baseline at y=0."""

    def __init__(self):
        self.glyphs, self.width = shape_wordmark()
        xs0, ys0, xs1, ys1 = zip(*[
            (g["x"] + g["bounds"][0], -g["bounds"][3],
             g["x"] + g["bounds"][2], -g["bounds"][1]) for g in self.glyphs])
        self.top, self.bottom = min(ys0), max(ys1)  # ink extent around baseline
        # The f-t join in "craft": both crossbars span y 401.3 to 496 in font
        # units; the bridge overlaps 6 units into each so no seam renders.
        f, t = self.glyphs[7], self.glyphs[8]
        self.bridge = dict(x=f["x"] + 323.4 - 6, w=(t["x"] + 14.5) - (f["x"] + 323.4) + 12,
                           y=-496, h=496 - 401.3)

    def paths(self, fill_tele, fill_craft, dx=0, dy=0):
        out = []
        for i, g in enumerate(self.glyphs):
            fill = fill_tele if i < TELE_GLYPHS else fill_craft
            out.append(f'<path fill="{fill}" transform="translate({g["x"] + dx:.1f},{dy:.1f}) '
                       f'scale(1,-1)" d="{round1(g["path"])}"/>')
        b = self.bridge
        out.append(f'<rect fill="{fill_craft}" x="{b["x"] + dx:.1f}" y="{b["y"] + dy:.1f}" '
                   f'width="{b["w"]:.1f}" height="{b["h"]:.1f}"/>')
        return "".join(out)


# ── The mark ────────────────────────────────────────────────────────────────

def mark_rects(c, mono=False, ox=0.0, oy=0.0, s=1.0):
    """The bare mark, origin at the datum's top-left, 18×16 units at s=1."""
    tones = ("ink", "ink", "ink") if mono else ("ink", "muted", "faint")
    d = DATUM
    out = [f'<rect x="{ox:.10g}" y="{oy:.10g}" width="{d["w"] * s:.10g}" '
           f'height="{d["h"] * s:.10g}" rx="{d["rx"] * s:.10g}" '
           f'fill="{c["ink"] if mono else c["brand"]}"/>']
    for band, tone in zip(BANDS, tones):
        out.append(f'<rect x="{ox + (band["x"] - d["x"]) * s:.10g}" '
                   f'y="{oy + (band["y"] - d["y"]) * s:.10g}" '
                   f'width="{band["w"] * s:.10g}" height="{BAND_H * s:.10g}" '
                   f'rx="{BAND_RX * s:.10g}" fill="{c[tone]}"/>')
    return "".join(out)


def svg(viewbox, body, label, comment):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{viewbox}" '
            f'role="img" aria-label="{label}">\n'
            f'  <title>{label}</title>\n'
            f'  <!-- {comment}\n       Generated by docs/branding/pack/tools/build.py. Do not edit by hand. -->\n'
            f'  {body}\n</svg>\n')


MARK_COMMENT = ("Three reading bands measured against a brand-amber datum "
                "(ADR-0004; docs/branding/identity.md). Colours are tokens "
                "from console/src/tokens.css.")


def write(name, content):
    (PACK / name).write_text(content)
    print(f"  {name}")


def main():
    PACK.mkdir(parents=True, exist_ok=True)
    wm = Wordmark()

    # Icons: the mark on its own plate. The dark plate is the canonical icon
    # and the favicon; it silhouettes on a light tab bar. The light plate
    # carries a hairline rule so it keeps an edge on white.
    icon_dark = (f'<rect width="32" height="32" rx="{PLATE_RX}" fill="{DARKG["ground"]}"/>'
                 + mark_rects(DARKG, ox=7, oy=8))
    write("telecraft-icon.svg",
          svg("0 0 32 32", icon_dark, "Telecraft", MARK_COMMENT))
    icon_light = (f'<rect width="32" height="32" rx="{PLATE_RX}" fill="#ffffff"/>'
                  f'<rect x="0.5" y="0.5" width="31" height="31" rx="{PLATE_RX - 0.5}" '
                  f'fill="none" stroke="{LIGHTG["rule"]}"/>'
                  + mark_rects(LIGHTG, ox=7, oy=8))
    write("telecraft-icon-light.svg",
          svg("0 0 32 32", icon_light, "Telecraft", MARK_COMMENT))

    # The bare mark, for placing on a ground the pack does not control.
    for name, c, mono in (("telecraft-mark-on-dark.svg", DARKG, False),
                          ("telecraft-mark-on-light.svg", LIGHTG, False),
                          ("telecraft-mark-mono.svg", dict(ink="currentColor"), True)):
        write(name, svg("0 0 18 16", mark_rects(c, mono=mono), "Telecraft", MARK_COMMENT))

    # Wordmarks. Baseline sits at y=0 in the construction; the viewBox is the
    # ink's tight bounds. The brand variants set "tele" in the amber, which is
    # the marketing surfaces' use of --brand (design-system.md, "Brand").
    wm_vb = f'0 {wm.top:.1f} {wm.width:.1f} {wm.bottom - wm.top:.1f}'
    wm_comment = ("Atkinson Hyperlegible Next wght 600, tracked -12/1000, outlined from "
                  "the vendored face. The f-t crossbars in 'craft' join into one bar.")
    for name, tele, craft in (
            ("telecraft-wordmark-on-dark.svg", DARKG["ink"], DARKG["ink"]),
            ("telecraft-wordmark-on-light.svg", LIGHTG["ink"], LIGHTG["ink"]),
            ("telecraft-wordmark-mono.svg", "currentColor", "currentColor"),
            ("telecraft-wordmark-brand-on-dark.svg", DARKG["brand"], DARKG["ink"]),
            ("telecraft-wordmark-brand-on-light.svg", LIGHTG["brand"], LIGHTG["ink"])):
        write(name, svg(wm_vb, wm.paths(tele, craft), "Telecraft", wm_comment))

    # Horizontal lockup: the mark at ascender height (718 units), sat on the
    # baseline, a 260-unit gap to the leading t.
    MH = 718.0
    s = MH / DATUM["h"]
    mw = 18 * s
    gap = 260.0
    lk_comment = MARK_COMMENT + " Mark at ascender height beside the wordmark."

    def lockup(c, mono, tele, craft):
        body = mark_rects(c, mono=mono, ox=0, oy=-MH, s=s) + wm.paths(tele, craft, dx=mw + gap)
        w = mw + gap + wm.width
        vb = f'0 {min(-MH, wm.top):.1f} {w:.1f} {wm.bottom - min(-MH, wm.top):.1f}'
        return svg(vb, body, "Telecraft", lk_comment)

    write("telecraft-lockup-on-dark.svg", lockup(DARKG, False, DARKG["ink"], DARKG["ink"]))
    write("telecraft-lockup-on-light.svg", lockup(LIGHTG, False, LIGHTG["ink"], LIGHTG["ink"]))
    write("telecraft-lockup-mono.svg",
          lockup(dict(ink="currentColor"), True, "currentColor", "currentColor"))
    write("telecraft-lockup-brand-on-dark.svg", lockup(DARKG, False, DARKG["brand"], DARKG["ink"]))
    write("telecraft-lockup-brand-on-light.svg", lockup(LIGHTG, False, LIGHTG["brand"], LIGHTG["ink"]))

    # Stacked lockup: the mark enlarged and centred over the wordmark.
    SH = 900.0
    ss = SH / DATUM["h"]
    smw = 18 * ss
    sgap = 300.0

    def stacked(c, tele, craft):
        mx = (wm.width - smw) / 2
        wy = SH + sgap - wm.top  # wordmark baseline position
        body = mark_rects(c, ox=mx, oy=0, s=ss) + wm.paths(tele, craft, dy=wy)
        h = SH + sgap + (wm.bottom - wm.top)
        return svg(f'0 0 {wm.width:.1f} {h:.1f}', body, "Telecraft",
                   MARK_COMMENT + " Mark centred over the wordmark.")

    write("telecraft-lockup-stacked-on-dark.svg", stacked(DARKG, DARKG["ink"], DARKG["ink"]))
    write("telecraft-lockup-stacked-on-light.svg", stacked(LIGHTG, LIGHTG["ink"], LIGHTG["ink"]))

    # The console's favicon is the canonical icon, copied verbatim.
    (ROOT / "console/public/favicon.svg").write_text(
        svg("0 0 32 32", icon_dark, "Telecraft", MARK_COMMENT))
    print("  console/public/favicon.svg")


if __name__ == "__main__":
    main()
