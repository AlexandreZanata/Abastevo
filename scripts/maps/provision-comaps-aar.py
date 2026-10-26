#!/usr/bin/env python3
"""Unpack only the admitted, vendored native build input into a private build dir."""
import argparse
import hashlib
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import tempfile
import zipfile

AAR_SHA256 = '7ad828883c4aca8afc46601ea98ed340db5056cbc61df2e1896d8916d98d6837'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def provision(aar, output, expected=AAR_SHA256):
    if sha(aar.read_bytes()) != expected:
        raise ValueError('native build input digest mismatch')
    files = {}
    with zipfile.ZipFile(aar) as archive:
        if len(archive.infolist()) > 1000 or sum(i.file_size for i in archive.infolist()) > 128 * 1024 * 1024:
            raise ValueError('native archive budget exceeded')
        seen = set()
        for entry in archive.infolist():
            name = entry.filename
            path = PurePosixPath(name)
            if (not name or path.is_absolute() or '..' in path.parts or '\\' in name
                    or ':' in name or name in seen or stat.S_ISLNK(entry.external_attr >> 16)):
                raise ValueError('unsafe native archive entry')
            seen.add(name)
            if not entry.is_dir():
                if entry.file_size > 32 * 1024 * 1024:
                    raise ValueError('native archive member budget exceeded')
                files[name] = archive.read(entry)
    required = {'classes.jar', 'AndroidManifest.xml', 'jni/arm64-v8a/liborganicmaps.so',
                'jni/armeabi-v7a/liborganicmaps.so'}
    if not required.issubset(files):
        raise ValueError('incomplete native build input')
    if output.is_symlink():
        raise ValueError('native output must not be a symlink')
    if output.exists():
        existing = {str(p.relative_to(output)) for p in output.rglob('*') if p.is_file()}
        if existing != set(files) or any(p.is_symlink() for p in output.rglob('*')):
            raise ValueError('modified native cache')
        if any(sha((output / name).read_bytes()) != sha(data) for name, data in files.items()):
            raise ValueError('modified native cache')
        return
    output.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix='.native-input-', dir=output.parent))
    try:
        for name, data in files.items():
            dest = stage / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(data)
        os.rename(stage, output)
    finally:
        if stage.exists():
            shutil.rmtree(stage)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--aar', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    provision(args.aar, args.output)
