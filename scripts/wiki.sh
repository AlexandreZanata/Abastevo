#!/usr/bin/env bash
# P01-T16 wiki exporter/publisher with owned-page manifest.
# Canonical content is committed Markdown; snapshots come from an explicit
# committed SHA (never the working tree). Only allowlisted docs plus selected
# root files are exported; everything else (including .local/, secrets and
# non-docs code) can only appear as immutable source permalinks, never as
# copied content. Nested paths are flattened with their full-path prefix so
# two different READMEs can never collide into one wiki name: a collision
# fails the export instead of silently overwriting.
# The wiki repository is a second remote resolved and verified independently.
# Publication requires --allow-publish and never auto-creates a disabled wiki:
# an unreachable remote, missing authorization scope, edited managed pages or
# any other failure leaves WIKI_PENDING with the exact reason and performs
# zero wiki commits/pushes. Dry-run performs zero wiki mutations.
# Manual/unmanaged wiki pages are never touched; only previously owned pages
# proven obsolete AND unchanged since their last managed version are deleted.
# There is deliberately no close/reopen/delete-all path in this script.
set -euo pipefail

ROOT="${WIKI_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
OWNER_REPO="AlexandreZanata/brazil-fuel-prices"
EXPECTED_WIKI_SSH="git@github.com:AlexandreZanata/brazil-fuel-prices.wiki.git"
EXPECTED_WIKI_HTTPS="https://github.com/AlexandreZanata/brazil-fuel-prices.wiki.git"
MANIFEST_NAME="wiki-manifest.json"
SIDEBAR_NAME="_Sidebar.md"

die() { echo "ERROR: $*" >&2; exit 1; }
pending() { echo "WIKI_PENDING: $*" >&2; exit 1; }
info() { echo "wiki: $*"; }

resolve_sha() {
    local sha="${1:-HEAD}"
    sha="$(git -C "$ROOT" rev-parse --verify "$sha^{commit}" 2>/dev/null)" \
        || die "not a committed SHA: $1 (snapshots never come from the working tree)"
    echo "$sha"
}

cmd_export() {
    # Offline: renders the snapshot into --out (default temp dir, removed)
    # and prints the manifest JSON plus a stats line to stdout.
    local sha="HEAD" out=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --sha) sha="$2"; shift 2 ;;
            --out) out="$2"; shift 2 ;;
            *) die "export: unknown flag $1" ;;
        esac
    done
    sha="$(resolve_sha "$sha")"
    local stage tmp_rm=0
    if [[ -z "$out" ]]; then
        stage="$(mktemp -d)"
        tmp_rm=1
    else
        mkdir -p "$out"
        stage="$out"
    fi
    set +e
    python3 - "$ROOT" "$sha" "$stage" "$OWNER_REPO" <<'PY'
import os, re, subprocess, sys, json, hashlib
root, sha, stage, owner_repo = sys.argv[1:5]

def git(*args):
    return subprocess.check_output(['git', '-C', root] + list(args)).decode()

sources = [l for l in git('ls-tree', '-r', '--name-only', sha, '--', 'docs/')
           .splitlines() if l.endswith('.md')]
for f in ('README.md', 'ROADMAP.md', 'TRADEMARKS.md'):
    if subprocess.run(['git', '-C', root, 'cat-file', '-e', f'{sha}:{f}'],
                       capture_output=True).returncode == 0:
        sources.append(f)
sources = sorted(set(sources))
if not sources:
    sys.exit('EMPTY_EXPORT: allowlist matched nothing; refusing instead of blanking the wiki')

# Configuration belongs to the same immutable snapshot as the docs, never
# the working tree or mutable environment. Preserve default for other sources.
overview_page = 'Home.md'
config_path = 'docs/planning/wiki-config.json'
if subprocess.run(['git', '-C', root, 'cat-file', '-e', f'{sha}:{config_path}'],
                  capture_output=True).returncode == 0:
    config = json.loads(git('show', f'{sha}:{config_path}'))
    if not isinstance(config, dict) or set(config) != {'overview_page'}:
        sys.exit('BAD_WIKI_CONFIG: expected only overview_page')
    overview_page = config['overview_page']
    if not isinstance(overview_page, str) or not re.fullmatch(
            r'[A-Za-z0-9][A-Za-z0-9-]*\.md', overview_page):
        sys.exit('BAD_WIKI_CONFIG: overview must be a bounded page filename')
    if len(overview_page) > 80:
        sys.exit('BAD_WIKI_CONFIG: overview name too long')

def wiki_name(src):
    if src == 'README.md':
        return overview_page
    if src == 'ROADMAP.md':
        return 'Roadmap.md'
    if src == 'TRADEMARKS.md':
        return 'Trademarks.md'
    return src.replace('/', '-')[:-3] + '.md'

mapping, seen = {}, {}
for s in sources:
    w = wiki_name(s)
    if w in seen:
        sys.exit(f'COLLISION: {s} and {seen[w]} both map to {w}')
    seen[w] = s
    mapping[s] = w

SECRET = re.compile(r'ghp_[A-Za-z0-9]{20,}|github_pat_|BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY'
                    r'|sk_live_|AKIA[0-9A-Z]{16}|xox[bap]-')
LINK = re.compile(r'(!?)\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
ABS = re.compile(r'^(?:[a-zA-Z][a-zA-Z0-9+.-]*:|#|mailto:)')

def rewrite(src, text):
    srcdir = os.path.dirname(src)
    def rep(m):
        bang, label, target = m.group(1), m.group(2), m.group(3)
        if ABS.match(target):
            return m.group(0)
        path, hashsep, anchor = target.partition('#')
        if not path:
            return m.group(0)
        norm = os.path.normpath(os.path.join(srcdir, path))
        if norm.startswith('..'):
            sys.exit(f'ESCAPE: {src} links above the repo root: {target}')
        frag = (hashsep + anchor) if hashsep else ''
        if norm in mapping:
            return f'{bang}[{label}]({mapping[norm]}{frag})'
        if bang == '!':
            if not norm.startswith('docs/assets/'):
                sys.exit(f'IMAGE_OUTSIDE_ASSETS: {src} -> {target}')
            if subprocess.run(['git', '-C', root, 'cat-file', '-e', f'{sha}:{norm}'],
                               capture_output=True).returncode != 0:
                sys.exit(f'MISSING_IMAGE: {src} -> {target}')
            return (f'![{label}](https://raw.githubusercontent.com/'
                    f'{owner_repo}/{sha}/{norm})')
        return (f'[{label}](https://github.com/{owner_repo}/blob/'
                f'{sha}/{norm}{frag})')
    return LINK.sub(rep, text)

pages = {}
for src in sources:
    text = git('show', f'{sha}:{src}')
    if SECRET.search(text):
        sys.exit(f'SECRET_IN_SOURCE: {src} matches a high-confidence secret pattern')
    pages[mapping[src]] = {'source': src, 'text': rewrite(src, text)}

nav = [overview_page, 'Roadmap.md'] + sorted(
    w for w in pages if w not in (overview_page, 'Roadmap.md', 'Trademarks.md'))
lines = ['### Fuel prices wiki', '']
for w in nav:
    if w in pages:
        lines.append(f'- [{w[:-3]}]({w})')
lines += ['', f'_Source: {owner_repo} @ {sha}_', '']
pages['_Sidebar.md'] = {'source': 'generated:_Sidebar', 'text': '\n'.join(lines)}

manifest = {'source_sha': sha, 'pages': {}}
for w in sorted(pages):
    body = pages[w]['text']
    if w != '_Sidebar.md' and SECRET.search(body):
        sys.exit(f'SECRET_IN_RENDERED: {w}')
    if not w.endswith('.md'):
        sys.exit(f'BAD_WIKI_NAME: {w}')
    with open(os.path.join(stage, w), 'w') as f:
        f.write(body)
    manifest['pages'][w] = {
        'source': pages[w]['source'],
        'sha256': hashlib.sha256(body.encode()).hexdigest(),
    }
with open(os.path.join(stage, 'MANIFEST.json'), 'w') as f:
    json.dump(manifest, f, indent=2, sort_keys=True)
print(json.dumps(manifest))
print(f'WIKI_EXPORT_OK sha={sha} pages={len(manifest["pages"])}', file=sys.stderr)
PY
    rc=$?
    set -e
    if [[ "$tmp_rm" == "1" ]]; then
        rm -rf "$stage"
    fi
    return $rc
}

wiki_origin_ok() {
    local wiki="$1"
    local url
    url="$(git -C "$wiki" remote get-url origin 2>/dev/null || echo "")"
    if [[ "${ANPFUEL_WIKI_OVERRIDE:-0}" == "1" ]]; then
        return 0
    fi
    [[ "$url" == "$EXPECTED_WIKI_SSH" || "$url" == "$EXPECTED_WIKI_HTTPS" ]] || return 1
}

cmd_publish() {
    local sha="HEAD" wiki="" dry_run=0 allow_publish=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --sha) sha="$2"; shift 2 ;;
            --wiki) wiki="$2"; shift 2 ;;
            --dry-run) dry_run=1; shift ;;
            --allow-publish) allow_publish=1; shift ;;
            *) die "publish: unknown flag $1" ;;
        esac
    done
    [[ -n "$wiki" ]] || die "publish requires --wiki PATH-or-URL"
    sha="$(resolve_sha "$sha")"
    local work clone_tmp=""
    if [[ "$wiki" == *"://"* || "$wiki" == *"@"* || "$wiki" =~ ^[^/]+:[^/] ]]; then
        clone_tmp="$(mktemp -d)"
        git ls-remote "$wiki" HEAD >/dev/null 2>&1 \
            || pending "wiki remote unreachable or disabled: $wiki"
        git clone -q "$wiki" "$clone_tmp/wiki" 2>/dev/null \
            || pending "cannot clone wiki remote: $wiki"
        work="$clone_tmp/wiki"
    else
        [[ -d "$wiki/.git" ]] || pending "not a wiki checkout (no .git): $wiki"
        work="$wiki"
    fi
    cleanup() { if [[ -n "$clone_tmp" ]]; then rm -rf "$clone_tmp"; fi; }
    if ! wiki_origin_ok "$work"; then
        cleanup
        pending "wiki origin is not the fuel wiki remote; refusing second-remote write"
    fi
    if [[ "$dry_run" == "0" && "$allow_publish" == "0" ]]; then
        cleanup
        pending "real publish needs --allow-publish within an authorized wiki scope"
    fi
    if [[ -n "$(git -C "$work" status --porcelain)" ]]; then
        cleanup
        die "wiki worktree dirty; refusing"
    fi
    local stage
    stage="$(mktemp -d)"
    local manifest export_err
    export_err="$(mktemp)"
    manifest=""
    if ! manifest="$(bash "$0" export --sha "$sha" --out "$stage" 2>"$export_err")"; then
        cat "$export_err" >&2
        rm -f "$export_err"
        cleanup
        rm -rf "$stage"
        die "export failed; wiki untouched"
    fi
    rm -f "$export_err"
    local actions
    if ! actions="$(python3 - "$work" "$stage" <<'PY'
import json, os, sys, hashlib
work, stage = sys.argv[1], sys.argv[2]
new = json.load(open(os.path.join(stage, 'MANIFEST.json')))
prev = {}
mpath = os.path.join(work, 'wiki-manifest.json')
if os.path.exists(mpath):
    prev = json.load(open(mpath)).get('pages', {})
def wread(name):
    p = os.path.join(work, name)
    return open(p).read() if os.path.exists(p) else None
def whash(name):
    c = wread(name)
    return hashlib.sha256(c.encode()).hexdigest() if c is not None else None
out = []
for w, meta in sorted(new['pages'].items()):
    with open(os.path.join(stage, w)) as f:
        body = f.read()
    cur = wread(w)
    if cur == body:
        out.append(f'noop {w}')
    elif cur is None:
        out.append(f'add {w}')
    elif w in prev and whash(w) == prev[w]['sha256']:
        out.append(f'update {w}')
    else:
        sys.exit(f'CONFLICT: {w} edited since last managed sync or unmanaged; refusing')
for w, meta in sorted(prev.items()):
    if w not in new['pages']:
        cur = wread(w)
        if cur is None:
            out.append(f'noop-gone {w}')
        elif whash(w) == meta['sha256']:
            out.append(f'delete {w}')
        else:
            sys.exit(f'CONFLICT: obsolete owned page {w} edited by hand; refusing')
print('\n'.join(out))
PY
)"; then
        cleanup
        rm -rf "$stage"
        pending "conflict detected; prior wiki commit kept, zero wiki mutations"
    fi
    if [[ "$dry_run" == "1" ]]; then
        echo "$actions"
        local before
        before="$(git -C "$work" rev-parse HEAD 2>/dev/null || echo NONE)"
        info "dry-run complete; $before unchanged, zero wiki commits/pushes"
        cleanup
        rm -rf "$stage"
        return 0
    fi
    while IFS= read -r line; do
        local act name
        act="${line%% *}"
        name="${line#* }"
        case "$act" in
            add|update) cp "$stage/$name" "$work/$name" ;;
            delete) rm "$work/$name" ;;
        esac
    done <<< "$actions"
    python3 - "$work" "$stage" <<'PY'
import json
new = json.load(open(f'{__import__("sys").argv[2]}/MANIFEST.json'))
json.dump(new, open(f'{__import__("sys").argv[1]}/wiki-manifest.json', 'w'), indent=2, sort_keys=True)
PY
    if [[ -z "$(git -C "$work" status --porcelain)" ]]; then
        info "unchanged snapshot; no wiki commit"
        cleanup
        rm -rf "$stage"
        return 0
    fi
    git -C "$work" add -A
    git -C "$work" commit -q -m "wiki: sync docs from $sha"
    git -C "$work" push origin HEAD
    info "published wiki from $sha"
    cleanup
    rm -rf "$stage"
}

case "${1:-}" in
    preview) shift; cmd_export "$@" ;;
    export) shift; cmd_export "$@" ;;
    publish) shift; cmd_publish "$@" ;;
    *) echo "usage: wiki.sh {preview|export|publish} [--sha SHA] [--out DIR] [--wiki PATH-or-URL] [--dry-run] [--allow-publish]" >&2; exit 1 ;;
esac
