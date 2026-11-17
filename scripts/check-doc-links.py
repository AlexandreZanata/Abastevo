#!/usr/bin/env python3
"""Fail when a tracked Markdown file links to a repository path that does not exist.

Relative links and images are resolved against the linking file. External URLs,
in-page anchors, site-absolute paths ("/...") and fenced or inline code are
ignored; landing/content holds site-relative page content and is skipped.
Usage: check-doc-links.py [--self-test]
"""
import os
import re
import subprocess
import sys
import tempfile
from urllib.parse import unquote

LINK = re.compile(r'!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)')
FENCE = re.compile(r'^(```|~~~).*?^\1', re.M | re.S)
INLINE_CODE = re.compile(r'`[^`\n]*`')
SKIP_PREFIXES = ('landing/content/',)


def broken_links(root, files):
    problems = []
    for name in files:
        if not name.endswith('.md') or name.startswith(SKIP_PREFIXES):
            continue
        with open(os.path.join(root, name), encoding='utf-8') as stream:
            text = INLINE_CODE.sub('', FENCE.sub('', stream.read()))
        base = os.path.dirname(name)
        for target in LINK.findall(text):
            if re.match(r'^[a-zA-Z][a-zA-Z0-9+.-]*:', target) or target.startswith(('#', '/')):
                continue
            path = unquote(target.split('#', 1)[0].split('?', 1)[0])
            if not path:
                continue
            resolved = os.path.normpath(os.path.join(base, path))
            if resolved.startswith('..') or not os.path.exists(os.path.join(root, resolved)):
                problems.append(f'{name}: {target}')
    return problems


def tracked_markdown(root):
    out = subprocess.run(['git', '-C', root, 'ls-files', '*.md'], capture_output=True, text=True, check=True)
    return [line for line in out.stdout.splitlines() if line]


def self_test():
    with tempfile.TemporaryDirectory() as root:
        os.makedirs(os.path.join(root, 'docs/a'))
        files = {
            'README.md': '[ok](docs/a/x.md) [dir](docs/a/) [web](https://e.invalid) [top](#t)\n'
                         '`[code](missing.md)`\n```\n[fenced](missing.md)\n```\n',
            'docs/a/x.md': '[up](../../README.md#intro) ![img](missing.png) [gone](y.md)\n',
            'landing/content/p.md': '[site](relative-page/)\n',
        }
        for name, text in files.items():
            os.makedirs(os.path.dirname(os.path.join(root, name)) or root, exist_ok=True)
            with open(os.path.join(root, name), 'w', encoding='utf-8') as stream:
                stream.write(text)
        got = broken_links(root, list(files))
        expected = ['docs/a/x.md: missing.png', 'docs/a/x.md: y.md']
        if got != expected:
            print(f'self-test failed: {got} != {expected}', file=sys.stderr)
            return 1
    print('check-doc-links self-test passed')
    return 0


def main():
    if sys.argv[1:] == ['--self-test']:
        return self_test()
    root = subprocess.run(['git', 'rev-parse', '--show-toplevel'], capture_output=True, text=True,
                          check=True).stdout.strip()
    problems = broken_links(root, tracked_markdown(root))
    for problem in problems:
        print(f'broken link: {problem}', file=sys.stderr)
    if problems:
        return 1
    print('Markdown links resolve.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
