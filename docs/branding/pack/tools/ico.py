#!/usr/bin/env python3
"""Pack rendered PNGs into one .ico.

A browser that cannot use an SVG favicon asks for `/favicon.ico`, and a
static host answers a missing one with a 404 and the reader gets whatever
default the browser draws. That is the whole reason this file exists: the
mark was correct and simply never offered in a format every browser takes.

An .ico is a trivial container: a six-byte header, one sixteen-byte
directory entry per image, then the images themselves. The entries here
carry PNG payloads rather than the older BMP encoding, which every browser
in support has read for years and which keeps the file small.

Standard library only, like `build.py` beside it. The PNGs are `rsvg-convert`
renders of the canonical icon, so this adds no second source for the artwork.

Usage: python3 ico.py OUT.ico IN-16.png IN-32.png IN-48.png ...
"""

import struct
import sys
from pathlib import Path


def png_size(data: bytes) -> tuple[int, int]:
    """Width and height from the IHDR chunk, which is always first."""
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    width, height = struct.unpack(">II", data[16:24])
    return width, height


def pack(pngs: list[bytes]) -> bytes:
    # ICONDIR: reserved, type 1 (icon), image count.
    header = struct.pack("<HHH", 0, 1, len(pngs))
    # Each entry is fixed width, so the first image starts after all of them.
    offset = len(header) + 16 * len(pngs)
    entries, images = [], []
    for data in pngs:
        width, height = png_size(data)
        if not (0 < width <= 256 and 0 < height <= 256):
            raise ValueError(f"{width}x{height} is outside what an .ico can hold")
        entries.append(
            struct.pack(
                "<BBBBHHII",
                # 256 is written as 0: the field is one byte, and 256 does
                # not fit in it. Every other size is itself.
                width % 256,
                height % 256,
                0,  # palette size, 0 for a true-colour image
                0,  # reserved
                1,  # colour planes
                32,  # bits per pixel
                len(data),
                offset,
            )
        )
        images.append(data)
        offset += len(data)
    return header + b"".join(entries) + b"".join(images)


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__.strip().splitlines()[-1], file=sys.stderr)
        return 2
    out = Path(sys.argv[1])
    pngs = [Path(p).read_bytes() for p in sys.argv[2:]]
    out.write_bytes(pack(pngs))
    sizes = ", ".join(f"{png_size(p)[0]}" for p in pngs)
    print(f"  {out} ({sizes})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
