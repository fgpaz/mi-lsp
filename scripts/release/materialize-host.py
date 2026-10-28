#!/usr/bin/env python3
"""Copy the two named GoReleaser uploadable binaries to dist for local upload."""
import json
import pathlib
import shutil
import sys

version = sys.argv[1]
dist = pathlib.Path('dist')
artifacts = json.loads((dist / 'artifacts.json').read_text(encoding='utf-8'))
for arch in ('x64', 'arm64'):
    name = f'mi-lsp-host_{version}_win-{arch}.exe'
    matches = [item for item in artifacts if item.get('name') == name and item.get('internal_type') == 2]
    if len(matches) != 1:
        raise SystemExit(f'expected exactly one uploadable binary named {name}; got {len(matches)}')
    shutil.copyfile(matches[0]['path'], dist / name)
