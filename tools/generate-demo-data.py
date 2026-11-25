#!/usr/bin/env python3
"""Create a tiny invented PhotoLine-shaped catalog for screenshot/demo use."""

import pathlib
import struct
import sys


def main() -> None:
    root = pathlib.Path(sys.argv[1])
    pictures = root / "Pictures"
    pictures.mkdir(parents=True, exist_ok=True)

    records = [
        (1, "Example County"),
        (3, "Sample School"),
        (5, "Class 5A"),
        (7, "Alex Example"),
        (8, "Student"),
        (7, "Jordan Sample"),
        (8, "Student"),
        (10, r"Pictures\demo-class-5a.jpg"),
        (11, "demo-"),
    ]
    data = bytearray(0x147)
    for field, text in records:
        encoded = text.encode("utf-8") + b"\0"
        data.extend(struct.pack(">III", field, 0x103, len(encoded)))
        data.extend(encoded)
    (root / "TextData.LST").write_bytes(data)


if __name__ == "__main__":
    main()
