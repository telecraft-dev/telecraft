# Telecraft design artefacts

The token and base sheets, the two self-hosted families, and the
brand mark, as consumed by the console, this repository's
documentation, and the telecraft.dev site.

- `tokens.css` carries values only, no selectors. Load it first.
- `base.css` carries typography, links, code, tables, controls and
  focus rings, and reads the tokens. It is absent until it is
  written.
- `fonts/fonts.css` declares the two families and reaches the
  `.woff2` files beside it by relative URL, so keep the directory
  whole.
- `fonts/*-OFL.txt` are the licences the faces ship under. They
  travel with the faces, and cover the faces only.
- `icons/` is the brand mark in the five formats a page has to
  offer. Nothing in the directory points at anything else, so put
  each file wherever your markup names it.
- `LICENSE` is Elastic License 2.0, which covers the stylesheets
  and the mark. You may use, modify and redistribute them; if you
  pass any part of this on, pass the licence on with it.

## The icon set

One drawing, five files. Every one of them is rasterised or copied
from `icons/favicon.svg`, so no format is a second version of the
artwork.

| File | Offered to |
|---|---|
| `favicon.svg` | Every current browser that accepts a vector tab icon. It scales, and it is the only file that carries the drawing rather than a render of it |
| `favicon.ico` | Safari, and any client that probes `/favicon.ico` at the site root without reading your markup |
| `icon-192.png` | A raster tab icon, and the size a launcher or a bookmark tile asks for |
| `icon-512.png` | The same at the size a splash screen or an app listing asks for |
| `apple-touch-icon.png` | The iOS home screen, which takes neither the SVG nor a `rel="icon"` PNG |

Five rather than one because the SVG alone is not enough in
practice. A page that offers only `favicon.svg` shows the mark in
most browsers and the browser's own default in Safari, and shows
the default again to anything that asks for `/favicon.ico`
directly. Both were real: the console shipped the SVG alone until
v0.4.0. Serve `favicon.ico` from the site root, whether or not
your markup links it.

The console's own `index.html` at this tag is the worked example
of offering all five.

Nothing here reaches another origin. Serve these files from your
own host: the console is built to run on a network with no route
out, so none of these files may be fetched from somewhere else,
and a site that uses the same sheets keeps that property.
