#!/usr/bin/env python3
"""Run one leg of the backend integration gate (real PostGIS, race detector).

The two largest packages dominate the wall time of a single
`go test -race -tags=integration ./...` (organization/adapters ~33 min and
platform/apicontract ~21 min on the hosted runner, 2026-11-16), because Go runs
the top-level tests of one package one after another. The gate therefore runs
in matrix legs on separate runners, each with its own PostGIS and MinIO:

* `--package P --shard I --shards N` runs exactly the top-level tests, examples
  and fuzz seeds of P whose stable CRC-32 bucket is I (of N);
* `--rest` runs every other package, unsharded.

Every test is selected by exactly one leg (the partition is a pure function of
the test name) and nothing is skipped or shortened: the union of the legs is the
former single command. SHARDED below must match the matrix in
.github/workflows/backend.yml (scripts/tests/test-backend-integration-shard.py).
"""
import argparse
import os
import re
import subprocess
import sys
import zlib

SHARDED = {
    "./internal/modules/organization/adapters": 4,
    "./internal/platform/apicontract": 2,
}
GO_TEST = ["go", "test", "-race", "-count=1", "-timeout=45m", "-tags=integration"]
NAME = re.compile(r"^(Test|Example|Fuzz)[A-Za-z0-9_]*$")


def bucket(name: str, shards: int) -> int:
    return zlib.crc32(name.encode()) % shards


def select(names, shard: int, shards: int):
    if shards < 1 or not 0 <= shard < shards:
        raise ValueError(f"shard {shard} outside 0..{shards - 1}")
    return [n for n in names if bucket(n, shards) == shard]


def run_pattern(names) -> str:
    return "^(" + "|".join(names) + ")$"


def list_tests(package: str):
    # -race so the listing compiles the same test binary the run reuses.
    out = subprocess.run(["go", "test", "-race", "-tags=integration", "-list", ".*", package],
                         check=True, capture_output=True, text=True).stdout
    names = [line.strip() for line in out.splitlines() if NAME.match(line.strip())]
    if not names:
        raise SystemExit(f"no tests listed for {package}")
    return names


def rest_packages(all_packages, sharded_import_paths):
    return [p for p in all_packages if p not in sharded_import_paths]


def main(argv=None) -> int:
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--package")
    group.add_argument("--rest", action="store_true")
    parser.add_argument("--shard", type=int, default=0)
    parser.add_argument("--shards", type=int, default=1)
    args = parser.parse_args(argv)
    os.chdir(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "backend"))
    if args.rest:
        listed = subprocess.run(["go", "list", "-tags=integration", "./..."], check=True,
                                capture_output=True, text=True).stdout.split()
        sharded = subprocess.run(["go", "list", "-tags=integration", *SHARDED], check=True,
                                 capture_output=True, text=True).stdout.split()
        packages = rest_packages(listed, set(sharded))
        if len(packages) != len(listed) - len(SHARDED):
            raise SystemExit("sharded packages not found in ./...: " + " ".join(SHARDED))
        print(f"integration rest: {len(packages)} packages (excluded: {' '.join(sorted(SHARDED))})", flush=True)
        return subprocess.run([*GO_TEST, *packages]).returncode
    if SHARDED.get(args.package) != args.shards:
        raise SystemExit(f"{args.package} is configured for {SHARDED.get(args.package)} shards, not {args.shards}")
    names = list_tests(args.package)
    chosen = select(names, args.shard, args.shards)
    print(f"integration {args.package} shard {args.shard + 1}/{args.shards}: {len(chosen)} of {len(names)} tests", flush=True)
    if not chosen:
        return 0
    return subprocess.run([*GO_TEST, "-run", run_pattern(chosen), args.package]).returncode


if __name__ == "__main__":
    sys.exit(main())
