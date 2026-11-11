#!/usr/bin/env python3
"""Check public Markdown links/anchors, media, fences and Bash syntax (offline).

Uses only the standard library and Bash. Remote URL availability and GitHub's
browser layout need separate review. This checks inline Markdown links and
quoted HTML href/src attributes, not a complete CommonMark grammar.
"""
from collections import Counter
from html import unescape
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
LINK = re.compile(r'\]\(([^\s)]+)(?:\s+"[^"]*")?\)|(?:href|src)="([^"]+)"')


def prose_and_shells(text):
    prose, shells, block = [], [], []
    fence, language = None, None
    for line in text.splitlines():
        marker = re.match(r"^\s*(`{3,}|~{3,})(.*)$", line)
        if fence is None:
            if marker:
                fence, language = marker[1], marker[2].strip()
                block = []
            else:
                prose.append(line)
        elif marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip():
            if language in ("bash", "sh", "shell"):
                shells.append("\n".join(block) + "\n")
            fence = None
        else:
            block.append(line)
    if fence:
        raise ValueError("unclosed code fence")
    return "\n".join(prose), shells


def anchors(prose):
    counts, result = Counter(), set()
    for line in prose.splitlines():
        match = re.match(r"^#{1,6}\s+(.+?)(?:\s+#+)?$", line)
        if not match:
            continue
        heading = re.sub(r"\[([^]]+)\]\([^)]+\)", r"\1", match[1])
        heading = re.sub(r"<[^>]+>", "", heading)
        slug = re.sub(r"[^\w\- ]", "", unescape(heading).lower()).replace(" ", "-")
        result.add(slug + (f"-{counts[slug]}" if counts[slug] else ""))
        counts[slug] += 1
    result.update(re.findall(r'(?:id|name)="([^"]+)"', prose))
    return result


def main():
    paths = [ROOT / p for p in sys.argv[1:]] if len(sys.argv) > 1 else [
        ROOT / "README.md", *sorted((ROOT / "docs").rglob("*.md")),
        *sorted((ROOT / "examples/server").rglob("*.md")),
        *sorted((ROOT / "skills/graybox").rglob("*.md")),
    ]
    errors, links, blocks = [], 0, 0
    for path in paths:
        label = path.relative_to(ROOT)
        try:
            prose, shells = prose_and_shells(path.read_text())
            for shell in shells:
                blocks += 1
                checked = subprocess.run(["bash", "-n"], input=shell, text=True, capture_output=True)
                if checked.returncode:
                    errors.append(f"{label}: Bash syntax: {checked.stderr.strip()}")
            for match in LINK.finditer(prose):
                link = unescape(match[1] or match[2])
                url = urlsplit(link)
                if url.scheme or url.netloc:
                    continue
                links += 1
                target = (path.parent / unquote(url.path)).resolve() if url.path else path
                if not target.exists():
                    errors.append(f"{label}: missing target {link}")
                elif url.fragment and target.suffix == ".md":
                    target_prose, _ = prose_and_shells(target.read_text())
                    if unquote(url.fragment) not in anchors(target_prose):
                        errors.append(f"{label}: missing anchor {link}")
                elif target.suffix == ".svg":
                    tree = ET.parse(target)
                    if not tree.getroot().findall("{http://www.w3.org/2000/svg}title"):
                        errors.append(f"{label}: SVG needs accessible title: {link}")
        except (ValueError, OSError, ET.ParseError) as error:
            errors.append(f"{label}: {error}")
    for error in errors:
        print(error, file=sys.stderr)
    print(f"{len(paths)} documents, {links} local links, {blocks} shell blocks; {len(errors)} errors")
    return bool(errors)


if __name__ == "__main__":
    sys.exit(main())
