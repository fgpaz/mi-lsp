#!/usr/bin/env python
"""Materialize GoReleaser host assets only after manifest and checksum checks."""
import hashlib
import json
import pathlib
import re
import shutil
import sys

version = sys.argv[1]
if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:[.-][0-9A-Za-z]+)?', version):
    raise SystemExit('invalid release version')
dist = pathlib.Path('dist').resolve(strict=True)
artifacts = json.loads((dist / 'artifacts.json').read_text(encoding='utf-8'))
checksums = (dist / f'mi-lsp_{version}_checksums.txt').read_text(encoding='utf-8').splitlines()

def digest(path):
    with path.open('rb') as stream:
        sha = hashlib.sha256()
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            sha.update(chunk)
        return sha.hexdigest()

for arch, goarch in (('x64', 'amd64'), ('arm64', 'arm64')):
    name = f'mi-lsp-host_{version}_win-{arch}.exe'
    matches = [item for item in artifacts if item.get('name') == name and item.get('internal_type') == 2]
    if len(matches) != 1:
        raise SystemExit(f'expected one uploadable host named {name}; got {len(matches)}')
    artifact = matches[0]
    if artifact.get('goos') != 'windows' or artifact.get('goarch') != goarch:
        raise SystemExit(f'invalid platform in manifest for {name}')
    source = pathlib.Path(artifact['path']).resolve(strict=True)
    if not source.is_relative_to(dist) or not source.is_file() or source.name not in ('mi-lsp-host.exe', name):
        raise SystemExit(f'invalid host source for {name}: {source}')
    records = [line.split() for line in checksums if len(line.split()) == 2 and line.split()[1].lstrip('*') == name]
    if len(records) != 1 or not re.fullmatch(r'[0-9a-fA-F]{64}', records[0][0]):
        raise SystemExit(f'missing or ambiguous checksum for {name}')
    expected = records[0][0].lower()
    if digest(source) != expected:
        raise SystemExit(f'host source checksum mismatch for {name}')
    target = dist / name
    if target.is_symlink():
        raise SystemExit(f'refusing symlink host target for {name}')
    if target.exists():
        if not target.is_file() or digest(target) != expected:
            raise SystemExit(f'existing host target differs for {name}; inspect dist')
        continue  # Includes same-file targets; never copy a file onto itself.
    shutil.copyfile(source, target)
    if digest(target) != expected:
        raise SystemExit(f'host target checksum mismatch for {name}')
