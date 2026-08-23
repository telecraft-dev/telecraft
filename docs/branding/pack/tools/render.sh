#!/bin/sh -e
# PNG renders of the icon, at the three sizes other services demand: 512 for
# a forge avatar, 192 for a web-app manifest, 180 for a touch icon. The SVG
# is the source; a PNG is a render, and any other size is one command:
#
#   rsvg-convert -w <size> -h <size> telecraft-icon.svg -o out.png
#
# Requires rsvg-convert (librsvg).
cd "$(dirname "$0")/.."
mkdir -p png
for s in 180 192 512; do
  rsvg-convert -w "$s" -h "$s" telecraft-icon.svg -o "png/telecraft-icon-$s.png"
done
echo "rendered png/telecraft-icon-{180,192,512}.png"
