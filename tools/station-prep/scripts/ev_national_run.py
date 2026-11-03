#!/usr/bin/env python3
"""Local-only Linux resource fence for the frozen national OSM preparation.

Raw payload/output remains private, outside Git. No network or database access.
Child logs are private and never echoed; only bounded aggregate receipts escape.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import resource
import signal
import subprocess
import time

from ev_national_sources import SourceError, new_directory, outside_checkout, write_json

PROFILE_FILE = Path(__file__).resolve().parents[3] / 'contracts/ev/osm-national-limits-v1.json'

class RunError(Exception):
    pass


def working_bytes(root, extras):
    total = 0
    paths = list(root.rglob('*')) + list(extras)
    seen = set()
    for path in paths:
        if path.is_symlink():
            raise RunError('symlink refused in owned working paths')
        if path.is_file():
            identity = path.resolve()
            if identity not in seen:
                total += path.stat().st_size
                seen.add(identity)
    return total


def run_process(command, root, extras, limits):
    """Fresh CLI process owns one parser child; RUSAGE_CHILDREN measures that child.

    Tests may pass smaller budgets; the public CLI always loads compiled-profile
    values from the tracked file and accepts no cap override.
    """
    def fence():
        for kind, value in [(resource.RLIMIT_AS, limits['max_memory_bytes']),
                            (resource.RLIMIT_CPU, limits['max_cpu_seconds']),
                            (resource.RLIMIT_FSIZE, limits['max_output_file_bytes']),
                            (resource.RLIMIT_CORE, 0)]:
            resource.setrlimit(kind, (value, value))
        os.umask(0o077)

    if working_bytes(root, extras) > limits['max_working_disk_bytes']:
        raise RunError('working disk budget exceeded before parser start')
    start = time.monotonic()
    child = None
    try:
        with (root/'parser.stdout').open('xb') as out, (root/'parser.stderr').open('xb') as err:
            child = subprocess.Popen(command, cwd=root, stdout=out, stderr=err,
                                     start_new_session=True, preexec_fn=fence)
            while child.poll() is None:
                if time.monotonic() - start > limits['max_wall_seconds']:
                    raise RunError('national parser wall budget exceeded')
                if working_bytes(root, extras) > limits['max_working_disk_bytes']:
                    raise RunError('national parser working disk budget exceeded')
                time.sleep(0.05)
            if child.returncode:
                raise RunError('national parser failed; private logs retained')
            if working_bytes(root, extras) > limits['max_working_disk_bytes']:
                raise RunError('national parser working disk budget exceeded')
        usage = resource.getrusage(resource.RUSAGE_CHILDREN)
        return {'wall_seconds': round(time.monotonic() - start, 3),
                'peak_rss_bytes': usage.ru_maxrss * 1024,
                'working_disk_bytes': working_bytes(root, extras)}
    finally:
        if child is not None and child.poll() is None:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait()


def require_local_environment():
    modes = [os.environ.get(key, '').lower() for key in
             ['APP_ENV', 'ANPFUEL_ENV', 'ANPFUEL_RUN_ENV', 'DEPLOY_ENV']]
    if os.environ.get('KUBERNETES_SERVICE_HOST') or any(
            mode in ('prod', 'production', 'stage', 'staging') for mode in modes):
        raise RunError('national preparation is local only')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True)
    parser.add_argument('--input', required=True)
    parser.add_argument('--boundaries', required=True)
    parser.add_argument('--output', required=True, help='new private working directory outside checkout')
    args = parser.parse_args()
    require_local_environment()
    os.umask(0o077)
    profile_raw = PROFILE_FILE.read_bytes()
    profile = json.loads(profile_raw)
    binary = Path(args.binary).resolve(strict=True)
    pbf = outside_checkout(Path(args.input)).resolve(strict=True)
    boundary = outside_checkout(Path(args.boundaries)).resolve(strict=True)
    for path in [pbf, boundary]:
        if not path.is_file():
            raise RunError('regular input file required')
    if pbf.stat().st_size > profile['limits']['max_input_bytes'] or boundary.stat().st_size > profile['census']['boundary_max_bytes']:
        raise RunError('input exceeds frozen profile')
    root = new_directory(args.output)
    generation = root/'generation'
    index = pbf.with_suffix('.pif')
    extras = [pbf, boundary, index, index.with_suffix('.pif.tmp')]
    measured = run_process([str(binary), '--profile', profile['profile_id'], '--input', str(pbf),
                            '--boundaries', str(boundary), '--output', str(generation)],
                           root, extras, profile['resources'])
    manifest = json.loads((generation/'manifest.json').read_bytes())
    profile_hash = hashlib.sha256(profile_raw).hexdigest()
    if manifest.get('preparation_profile', {}).get('sha256') != profile_hash:
        raise RunError('binary and tracked national profile differ')
    # Validate every emitted file before recording successful preparation.
    for name, expected in manifest['files'].items():
        if Path(name).name != name:
            raise RunError('unsafe output manifest name')
        raw = (generation/name).read_bytes()
        if len(raw) != expected['bytes'] or hashlib.sha256(raw).hexdigest() != expected['sha256']:
            raise RunError('output manifest hash mismatch')
    receipt = {'status': 'LOCAL_PREPARED_NOT_IMPORTED', 'profile_id': profile['profile_id'],
               'profile_sha256': profile_hash, 'source_sha256': manifest['source_sha256'],
               'manifest_sha256': hashlib.sha256((generation/'manifest.json').read_bytes()).hexdigest(),
               'limits': profile['resources'], 'measurements': measured, 'census': manifest['census']}
    write_json(root/'execution-receipt.json', receipt)
    print(json.dumps(receipt, separators=(',', ':')))


if __name__ == '__main__':
    try:
        main()
    except (RunError, SourceError, OSError, ValueError, KeyError):
        # Never expose child stderr, arbitrary source bytes or private paths.
        print('national preparation failed; inspect private working logs locally', file=__import__('sys').stderr)
        raise SystemExit(2)
