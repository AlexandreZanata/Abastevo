#!/usr/bin/env python3
"""Publish verified assets without replacing any existing tag or asset."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tempfile
from release_flow import REPOSITORY, gh, assert_ready
from artifact import digest


def same_asset(path, asset):
    return asset.get('size') == path.stat().st_size and asset.get('digest') == 'sha256:' + digest(path)


def verify_local(directory):
    manifest = json.loads((directory / 'release.json').read_text())
    if manifest.get('schema') != 1 or manifest.get('channel') != 'staging' or manifest.get('application_id') != 'com.anpfuel.app':
        raise ValueError('unsupported release manifest')
    expected = set(manifest['files']) | {'release.json', 'SHA256SUMS'}
    if set(p.name for p in directory.iterdir()) != expected:
        raise ValueError('unexpected release bundle files')
    for name, info in manifest['files'].items():
        if Path(name).name != name or not (directory / name).is_file() or digest(directory / name) != info['sha256'] or (directory / name).stat().st_size != info['bytes']:
            raise ValueError('release bundle hash/size mismatch')
    lines = ''.join(f'{digest(p)}  {p.name}\n' for p in sorted(directory.iterdir()) if p.name != 'SHA256SUMS')
    if (directory / 'SHA256SUMS').read_text() != lines:
        raise ValueError('release bundle checksums mismatch')
    return manifest


def release_or_none(tag):
    result = subprocess.run(['gh','api',f'repos/{REPOSITORY}/releases/tags/{tag}'],capture_output=True,text=True)
    if result.returncode == 0:
        return json.loads(result.stdout)
    if 'HTTP 404' in result.stderr:
        return None
    raise ValueError('cannot inspect existing GitHub release')


def verify_tag(tag, sha):
    obj = gh(f'repos/{REPOSITORY}/git/ref/tags/{tag}')['object']
    for _ in range(5):
        if obj['type'] == 'commit':
            if obj['sha'] != sha:
                raise ValueError('existing release tag targets another commit')
            return
        if obj['type'] != 'tag':
            break
        obj = gh(f'repos/{REPOSITORY}/git/tags/{obj["sha"]}')['object']
    raise ValueError('invalid release tag')


def ensure_tag(tag, sha):
    probe = subprocess.run(['gh', 'api', f'repos/{REPOSITORY}/git/ref/tags/{tag}'], capture_output=True, text=True)
    if probe.returncode:
        if 'HTTP 404' not in probe.stderr:
            raise ValueError('cannot inspect release tag')
        # Create-only Git operation: a simultaneous tag creation is re-read and
        # must resolve to the same source; an existing ref is never patched.
        result = subprocess.run(['gh', 'api', '--method', 'POST', f'repos/{REPOSITORY}/git/refs', '--input', '-'],
                                input=json.dumps({'ref': 'refs/tags/' + tag, 'sha': sha}), text=True, capture_output=True)
        if result.returncode and 'HTTP 422' not in result.stderr:
            raise ValueError('cannot create immutable release tag')
    verify_tag(tag, sha)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    manifest = verify_local(args.directory)
    tag, sha = manifest['tag'], manifest['commit']
    if not re.fullmatch(r'android-v[0-9]+\.[0-9]+\.[0-9]+', tag) or not re.fullmatch(r'[a-f0-9]{40}', sha):
        raise ValueError('invalid release identity')
    if gh(f'repos/{REPOSITORY}/git/ref/heads/main')['object']['sha'] != sha:
        raise ValueError('main advanced before publication; release the current delivery')
    assert_ready(sha)
    release = release_or_none(tag)
    ensure_tag(tag, sha)
    if release is None:
        notes = (f"Signed Android release build for staging: https://teste.abastevo.com.br\n\n"
                 f"Version {manifest['version_name']} · code {manifest['version_code']}\n"
                 f"Source: {sha}\n\nAPK, checksums, public signing certificate and license notices are attached. "
                 "This prerelease does not certify production or iOS.\n")
        with tempfile.TemporaryDirectory() as temp:
            body=Path(temp)/'notes.txt';body.write_text(notes)
            subprocess.run(['gh','release','create',tag,'--repo',REPOSITORY,'--target',sha,'--draft','--prerelease',
                            '--title',f"Abastevo Android {manifest['version_name']} (staging)",'--notes-file',str(body)],check=True)
        release = release_or_none(tag)
    verify_tag(tag, sha)
    if not release['prerelease']:
        raise ValueError('existing tag is not an Android staging prerelease')
    local = {p.name:p for p in args.directory.iterdir()}
    remote = {a['name']:a for a in release['assets']}
    if len(remote) != len(release['assets']) or set(remote) - set(local):
        raise ValueError('unexpected existing release assets')
    for name, asset in remote.items():
        if not same_asset(local[name], asset):
            raise ValueError('existing immutable asset differs; refuse overwrite')
    missing = sorted(set(local) - set(remote))
    if missing and not release['draft']:
        raise ValueError('published release is incomplete; refuse mutation')
    for name in missing:
        subprocess.run(['gh','release','upload',tag,str(local[name]),'--repo',REPOSITORY],check=True)
    current = release_or_none(tag)
    assets = {a['name']:a for a in current['assets']}
    if set(assets) != set(local) or any(not same_asset(path,assets[name]) for name,path in local.items()):
        raise ValueError('GitHub asset verification failed')
    if current['draft']:
        subprocess.run(['gh','release','edit',tag,'--repo',REPOSITORY,'--draft=false','--prerelease'],check=True)
    print(release_or_none(tag)['html_url'])


if __name__ == '__main__':
    main()
