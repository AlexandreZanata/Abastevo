#!/usr/bin/env python3
"""Render staging manifests without substituting pod shell variables.

The render environment is an operator-owned file outside Git (mode 600). Only
declared ${NAME} placeholders are replaced; shell variables such as
$POSTGRES_USER inside probe commands are left intact.
"""
import os
import re
import sys

PLACEHOLDER = re.compile(r"\$\{([A-Z0-9_]+)\}")


def load_env(path):
    env = {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if not line or "=" not in line:
                continue
            key, value = line.split("=", 1)
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "'\"":
                value = value[1:-1]
            env[key] = value
    return env


def main():
    env_path = os.environ.get("STAGING_RENDER_ENV", "")
    if not env_path:
        sys.exit("STAGING_RENDER_ENV must point to the operator render file")
    env = load_env(env_path)
    documents = []
    for path in sys.argv[1:]:
        with open(path, encoding="utf-8") as handle:
            text = handle.read()
        missing = sorted(set(PLACEHOLDER.findall(text)) - set(env))
        if missing:
            sys.exit(f"{path}: missing render values: {', '.join(missing)}")
        documents.append(PLACEHOLDER.sub(lambda m: env[m.group(1)], text))
    sys.stdout.write("\n---\n".join(documents))


if __name__ == "__main__":
    main()
