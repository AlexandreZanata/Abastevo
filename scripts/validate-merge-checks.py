#!/usr/bin/env python3
"""Validate every enabled CI signal before the guarded merge can mutate Git."""

import argparse
import json
import sys


class CheckError(ValueError):
    pass


def validate(raw, head, required):
    names = {name.strip() for name in required.split(",") if name.strip()}
    if not names:
        raise CheckError("required check list must not be empty")
    decoder = json.JSONDecoder()
    remaining = raw.strip()
    checks = []
    total = None
    if not remaining:
        raise CheckError("check response unavailable")
    while remaining:
        try:
            page, end = decoder.raw_decode(remaining)
        except ValueError as error:
            raise CheckError("invalid paginated check response") from error
        if not isinstance(page, dict) or not isinstance(page.get("check_runs"), list):
            raise CheckError("invalid check response shape")
        count = page.get("total_count")
        if count is not None:
            if type(count) is not int or count < 0 or (total is not None and total != count):
                raise CheckError("inconsistent check pagination")
            total = count
        checks.extend(page["check_runs"])
        remaining = remaining[end:].lstrip()
    if total is not None and len(checks) != total:
        raise CheckError("incomplete check pagination")
    seen = set()
    for check in checks:
        if not isinstance(check, dict) or not isinstance(check.get("name"), str) or not check["name"]:
            raise CheckError("invalid check entry")
        name = check["name"]
        if check.get("head_sha") != head:
            raise CheckError(f"check {name!r} is not on the validated head")
        status, conclusion = check.get("status"), check.get("conclusion")
        if status != "completed":
            raise CheckError(f"check {name!r} is {status}/{conclusion}; CI is incomplete")
        # Optional neutral/path-selected skipped signals are not success
        # evidence; any enabled failure still blocks the entire delivery.
        if not isinstance(conclusion, str) or conclusion not in {"success", "neutral", "skipped"}:
            raise CheckError(f"check {name!r} is {status}/{conclusion}; merge refused")
        if name in names:
            if conclusion != "success":
                raise CheckError(f"required check {name!r} needs completed/success")
            seen.add(name)
    missing = names - seen
    if missing:
        raise CheckError(f"missing required checks: {', '.join(sorted(missing))}")
    return len(checks)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--head", required=True)
    parser.add_argument("--required", required=True)
    args = parser.parse_args()
    try:
        count = validate(sys.stdin.read(), args.head, args.required)
    except CheckError as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(f"git-flow: verified {count} CI signals on {args.head}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
