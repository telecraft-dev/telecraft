#!/bin/sh -e
# PNG renders of the icon, and the browser icon set the console ships.
#
# Two groups come out of this, from one source. The pack's own renders are
# the three sizes other services demand: 512 for a forge avatar, 192 for a
# web-app manifest, 180 for a touch icon. Any other size is one command:
#
#   rsvg-convert -w <size> -h <size> telecraft-icon.svg -o out.png
#
# The second group is `console/public/`, which is what a browser is actually
# offered. An SVG favicon alone is not enough: Safari has not reliably drawn
# one, and anything that probes `/favicon.ico` gets a 404 from a static host,
# so the reader sees the browser's default rather than the mark. The set here
# covers every browser in support, and `console/index.html` names all of it.
#
# The SVG is the source in both groups. A PNG is a render, so nothing here is
# a second copy of the artwork: `build.py` draws the icon, this rasterises it.
#
# Requires rsvg-convert (librsvg). Everything else is the standard library.
cd "$(dirname "$0")/.."
mkdir -p png

render() { rsvg-convert -w "$1" -h "$1" telecraft-icon.svg -o "$2"; }

# The pack's published renders.
for s in 180 192 512; do
  render "$s" "png/telecraft-icon-$s.png"
done
echo "rendered png/telecraft-icon-{180,192,512}.png"

# The console's browser icon set. 180 is the touch icon iOS asks for by name;
# 192 and 512 are the two a manifest wants; 16, 32 and 48 are the classic
# favicon sizes and go into the .ico rather than shipping loose.
CONSOLE="../../../console/public"
cp png/telecraft-icon-180.png "$CONSOLE/apple-touch-icon.png"
cp png/telecraft-icon-192.png "$CONSOLE/icon-192.png"
cp png/telecraft-icon-512.png "$CONSOLE/icon-512.png"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
for s in 16 32 48; do
  render "$s" "$TMP/$s.png"
done
python3 tools/ico.py "$CONSOLE/favicon.ico" "$TMP/16.png" "$TMP/32.png" "$TMP/48.png"
echo "wrote the console's browser icon set into console/public/"
