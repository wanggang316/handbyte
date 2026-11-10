#!/usr/bin/env python3
"""Extract the changelog body for a CLI release."""

import argparse
from datetime import date
from pathlib import Path
import re
import sys


def extract_notes(text, tag):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise ValueError("expected tag vX.Y.Z")
    version = tag[1:]
    sections = re.split(r"(?m)^## ", text)
    matches = [part for part in sections[1:] if part.startswith(f"[{version}]")]
    if len(matches) != 1:
        raise ValueError(f"expected one changelog section for {version}")
    header, _, body = matches[0].partition("\n")
    match = re.fullmatch(rf"\[{re.escape(version)}\] - (\d{{4}}-\d{{2}}-\d{{2}})", header)
    if not match:
        raise ValueError("expected heading ## [X.Y.Z] - YYYY-MM-DD")
    date.fromisoformat(match[1])
    if not re.search(r"(?m)^- \S", body):
        raise ValueError("release notes must contain at least one change entry")
    return body.strip() + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("changelog", nargs="?", default="CHANGELOG.md")
    args = parser.parse_args()
    try:
        result = extract_notes(Path(args.changelog).read_text(encoding="utf-8"), args.tag)
    except (OSError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    sys.stdout.write(result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
