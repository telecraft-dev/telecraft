# Vendored typefaces

Two families, self-hosted (ADR-0019, ADR-0047 §3). No font is fetched from
another origin, and `console/tools/check-zero-cdn.mjs` fails the build if one
is.

| File | Family | Licence |
|---|---|---|
| `AtkinsonHyperlegibleNext.woff2` | Atkinson Hyperlegible Next, upright, variable `wght` 400 to 700 | [OFL 1.1](AtkinsonHyperlegibleNext-OFL.txt) |
| `AtkinsonHyperlegibleNext-Italic.woff2` | Atkinson Hyperlegible Next, italic, variable `wght` 400 to 700 | [OFL 1.1](AtkinsonHyperlegibleNext-OFL.txt) |
| `JetBrainsMono.woff2` | JetBrains Mono, variable `wght` 400 to 700 | [OFL 1.1](JetBrainsMono-OFL.txt) |

## Why Next, and why variable

ADR-0047 §3 names Atkinson Hyperlegible. The shipped family is Atkinson
Hyperlegible **Next**: the same design, redrawn by the same foundry, and the
one of the two published as a variable font. The console asks for weights
400, 500, 600 and 700; the original ships only 400 and 700, so 500 and 600
would be synthesised by the browser. Next answers all four from one file.

Range-instancing to `wght` 400 to 700 and subsetting to Latin costs 70 KB for
three faces, against 426 KB for the three unsubset sources.

## Regenerating them

Sources are the Google Fonts upstream, `ofl/atkinsonhyperlegiblenext` and
`ofl/jetbrainsmono`. With `fonttools` and `brotli` installed:

```sh
LATIN='U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+2074,U+20AC,U+2122,U+2212,U+2215,U+FEFF,U+FFFD'

for f in AtkinsonHyperlegibleNext AtkinsonHyperlegibleNext-Italic JetBrainsMono; do
  fonttools varLib.instancer -q "$f.ttf" wght=400:700 -o "$f-range.ttf"
  pyftsubset "$f-range.ttf" \
    --flavor=woff2 \
    --layout-features='kern,liga,calt,tnum,ccmp,locl,mark,mkmk' \
    --unicodes="$LATIN" \
    --output-file="$f.woff2"
done
```

`tnum` is kept deliberately: readings are numbers set to line up, and the
identity is the language of measurement (`docs/branding/identity.md`).
