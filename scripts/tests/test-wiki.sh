#!/usr/bin/env bash
# P01-T16 focused harness: wiki exporter/publisher against temporary local
# Git source/wiki repositories (zero network, zero real-remote mutations).
# Proves offline preview with source SHA, link/anchor/asset rewriting,
# collision refusal, manual-page preservation, edited-managed conflict,
# obsolete-owned-only deletion, identical-snapshot no-op, empty-export and
# secret refusal, dry-run zero mutations, missing-wiki WIKI_PENDING and no
# blanket-deletion path. Real wiki publication stays out of scope here.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
WIKI="$REPO/scripts/wiki.sh"
PASS=0
FAIL=0
ok() { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

git_cfg() { git -C "$1" config user.email t@t.t; git -C "$1" config user.name t; }

make_source() {
    # $1 = dir. Minimal allowlisted tree with links, anchor, code + asset refs.
    local d="$1"
    mkdir -p "$d/docs" "$d/docs/assets" "$d/backend"
    printf '# Root\n\nSee [guide](docs/guide.md) and [plan](ROADMAP.md).\n' > "$d/README.md"
    printf '# Roadmap\n' > "$d/ROADMAP.md"
    printf '# TM\n' > "$d/TRADEMARKS.md"
    printf 'package x\n' > "$d/backend/x.go"
    printf 'PNG' > "$d/docs/assets/pic.png"
    cat > "$d/docs/guide.md" <<'EOF'
# Guide

See [other](other.md#sec) and [home](../README.md) and [code](../backend/x.go).

![pic](assets/pic.png)
EOF
    printf '# Other\n\n## Sec\n\nBody.\n' > "$d/docs/other.md"
    git -C "$d" init -q
    git_cfg "$d"
    git -C "$d" add -A
    git -C "$d" commit -qm init
}

make_wiki_remote() {
    # $1 = dir. Prints "remote work": bare remote + work clone checked out.
    local d="$1"
    local remote="$d/remote.git" work="$d/work"
    git init --bare -q "$remote"
    git init -q "$work"
    git_cfg "$work"
    git -C "$work" commit -q --allow-empty -m init
    git -C "$work" remote add origin "$remote"
    git -C "$work" push -q origin HEAD:master 2>/dev/null || git -C "$work" push -q origin HEAD
    printf "%s %s" "$remote" "$work"
}

export ANPFUEL_WIKI_OVERRIDE=1

echo "== offline preview carries source SHA =="
T="$(mktemp -d)"
make_source "$T/src"
SHA="$(git -C "$T/src" rev-parse HEAD)"
OUT="$(WIKI_ROOT="$T/src" bash "$WIKI" preview --out "$T/stage" 2>/tmp/prev-err.log)"
if echo "$OUT" | grep -q "\"source_sha\": \"$SHA\""; then ok "preview records source SHA"; else bad "preview SHA"; fi
[[ -f "$T/stage/Home.md" ]] && ok "Home generated" || bad "Home missing"
[[ -f "$T/stage/_Sidebar.md" ]] && ok "sidebar generated" || bad "sidebar missing"
[[ -f "$T/stage/MANIFEST.json" ]] && ok "manifest staged" || bad "manifest missing"
rm -f /tmp/prev-err.log

echo "== links, anchors and assets rewritten =="
GUIDE="$(cat "$T/stage/docs-guide.md")"
echo "$GUIDE" | grep -q "(docs-other.md#sec)" && ok "doc link rewritten" || bad "doc link"
echo "$GUIDE" | grep -q "(Home.md)" && ok "root link rewritten" || bad "root link"
echo "$GUIDE" | grep -q "blob/$SHA/backend/x.go" && ok "code permalink pinned" || bad "code permalink"
echo "$GUIDE" | grep -q "raw.githubusercontent.com/AlexandreZanata/brazil-fuel-prices/$SHA/docs/assets/pic.png" \
    && ok "asset raw pinned" || bad "asset link"
if echo "$GUIDE" | grep -q "](docs/\|](\.\./"; then bad "relative leftovers"; else ok "no relative leftovers"; fi

echo "== nested README collision refuses =="
C="$(mktemp -d)"
mkdir -p "$C/docs/x" "$C/docs/x-y"
printf '# a\n' > "$C/docs/x/y-z.md"
printf '# b\n' > "$C/docs/x-y/z.md"
printf '# r\n' > "$C/README.md"
git -C "$C" init -q
git_cfg "$C"
git -C "$C" add -A
git -C "$C" commit -qm init
if WIKI_ROOT="$C" bash "$WIKI" preview >/dev/null 2>&1; then bad "collision accepted"; else ok "collision refuses"; fi

echo "== publish adds pages, preserves manual content =="
read WREMOTE WWORK <<< "$(make_wiki_remote "$T")"
printf '# Mine\n\nHand-written.\n' > "$WWORK/Manual.md"
git -C "$WWORK" add -A
git -C "$WWORK" commit -qm manual
if WIKI_ROOT="$T/src" bash "$WIKI" publish --wiki "$WWORK" --allow-publish >/dev/null 2>&1; then
    ok "publish succeeds"
else
    bad "publish failed"
fi
[[ "$(cat "$WWORK/Manual.md")" == *"Hand-written"* ]] && ok "manual page preserved" || bad "manual lost"
[[ -f "$WWORK/wiki-manifest.json" ]] && ok "manifest published" || bad "manifest missing"
git -C "$WWORK" log --oneline | grep -q "sync docs from $SHA" && ok "commit records source SHA" || bad "commit message"

echo "== dry-run on synced state succeeds with zero mutations =="
DHEAD="$(git -C "$WWORK" rev-parse HEAD)"
if WIKI_ROOT="$T/src" bash "$WIKI" publish --wiki "$WWORK" --allow-publish --dry-run >/dev/null 2>&1; then
    ok "dry-run succeeds"
else
    bad "dry-run failed"
fi
[[ "$(git -C "$WWORK" rev-parse HEAD)" == "$DHEAD" && -z "$(git -C "$WWORK" status --porcelain)" ]] \
    && ok "dry-run zero mutations" || bad "dry-run mutated"

echo "== edited managed page conflicts, prior commit kept =="
printf '# Guide\n\nHuman rewrite of managed content.\n' > "$WWORK/docs-guide.md"
git -C "$WWORK" add -A
git -C "$WWORK" commit -qm human-edit
BEFORE="$(git -C "$WWORK" rev-parse HEAD)"
printf '# Guide\n\nSee [other](other.md#sec) and NEW.\n' > "$T/src/docs/guide.md"
git -C "$T/src" add -A
git -C "$T/src" commit -qm source-change
if WIKI_ROOT="$T/src" bash "$WIKI" publish --wiki "$WWORK" --allow-publish >/tmp/conflict.log 2>&1; then
    bad "conflict accepted"
else
    ok "conflict refuses"
fi
grep -q "WIKI_PENDING" /tmp/conflict.log && ok "conflict reports WIKI_PENDING" || bad "no pending marker"
[[ "$(git -C "$WWORK" rev-parse HEAD)" != "$BEFORE" ]] && bad "conflict committed" || ok "prior commit kept"

echo "== obsolete owned-only page deleted; edited one kept =="
O="$(mktemp -d)"
make_source "$O/src"
read OREM OWORK <<< "$(make_wiki_remote "$O")"
WIKI_ROOT="$O/src" bash "$WIKI" publish --wiki "$OWORK" --allow-publish >/dev/null 2>&1
printf 'obsolete body\n' > "$OWORK/Old.md"
OBSHA="$(sha256sum "$OWORK/Old.md" | cut -d' ' -f1)"
python3 - "$OWORK/wiki-manifest.json" "$OBSHA" <<'PY'
import json, sys
p, h = sys.argv[1], sys.argv[2]
d = json.load(open(p))
d["pages"]["Old.md"] = {"source": "docs/old.md", "sha256": h}
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
git -C "$OWORK" add -A
git -C "$OWORK" commit -qm add-obsolete
if WIKI_ROOT="$O/src" bash "$WIKI" publish --wiki "$OWORK" --allow-publish >/dev/null 2>&1; then
    ok "obsolete sync succeeds"
else
    bad "obsolete sync failed"
fi
[[ -e "$OWORK/Old.md" ]] && bad "obsolete kept" || ok "obsolete owned-only deleted"
printf 'obsolete body\n' > "$OWORK/Old2.md"
OB2SHA="$(sha256sum "$OWORK/Old2.md" | cut -d' ' -f1)"
python3 - "$OWORK/wiki-manifest.json" "$OB2SHA" <<'PY'
import json, sys
p, h = sys.argv[1], sys.argv[2]
d = json.load(open(p))
d["pages"]["Old2.md"] = {"source": "docs/old2.md", "sha256": h}
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
git -C "$OWORK" add -A
git -C "$OWORK" commit -qm add-obsolete2
printf 'human touched obsolete\n' > "$OWORK/Old2.md"
if WIKI_ROOT="$O/src" bash "$WIKI" publish --wiki "$OWORK" --allow-publish >/dev/null 2>&1; then
    bad "edited obsolete deleted"
else
    ok "edited obsolete conflicts, kept"
fi
[[ -e "$OWORK/Old2.md" ]] && ok "edited obsolete file kept" || bad "edited obsolete lost"

echo "== identical snapshot produces no commit =="
H1="$(git -C "$OWORK" rev-parse HEAD 2>/dev/null || echo NONE)"
git -C "$OWORK" checkout -q -- Old2.md 2>/dev/null || true
python3 - "$OWORK/wiki-manifest.json" <<'PY'
import json, sys
p = sys.argv[1]
d = json.load(open(p))
d["pages"].pop("Old2.md", None)
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
rm -f "$OWORK/Old2.md"
git -C "$OWORK" add -A
git -C "$OWORK" commit -qm cleanup 2>/dev/null || true
if WIKI_ROOT="$O/src" bash "$WIKI" publish --wiki "$OWORK" --allow-publish 2>&1 | grep -q "unchanged snapshot"; then
    ok "identical snapshot no-op"
else
    bad "identical snapshot committed"
fi

echo "== empty export and secret source refuse =="
E="$(mktemp -d)"
mkdir -p "$E/backend"
printf 'package x\n' > "$E/backend/x.go"
git -C "$E" init -q
git_cfg "$E"
git -C "$E" add -A
git -C "$E" commit -qm init
if WIKI_ROOT="$E" bash "$WIKI" preview >/dev/null 2>&1; then bad "empty accepted"; else ok "empty export refuses"; fi
S="$(mktemp -d)"
mkdir -p "$S/docs"
printf '# r\n' > "$S/README.md"
printf 'token ghp_%s\n' 'ABCDEFGHIJKLMNOPQRSTUVWXYZ123456' > "$S/docs/leak.md"
git -C "$S" init -q
git_cfg "$S"
git -C "$S" add -A
git -C "$S" commit -qm init
if WIKI_ROOT="$S" bash "$WIKI" preview >/dev/null 2>&1; then bad "secret accepted"; else ok "secret source refuses"; fi

echo "== missing wiki and unauthorized scope stay pending =="
if WIKI_ROOT="$T/src" bash "$WIKI" publish --wiki /nonexistent-wiki --allow-publish >/tmp/missing.log 2>&1; then
    bad "missing wiki accepted"
else
    ok "missing wiki refuses"
fi
grep -q "WIKI_PENDING" /tmp/missing.log && ok "missing wiki pending marker" || bad "missing marker"
if WIKI_ROOT="$T/src" bash "$WIKI" publish --wiki "$WWORK" >/tmp/auth.log 2>&1; then
    bad "publish without scope accepted"
else
    ok "publish needs authorized scope"
fi
grep -q "WIKI_PENDING" /tmp/auth.log && ok "scope refusal is pending" || bad "scope marker"

echo "== no blanket-deletion path by construction =="
if grep -nE 'rm +-rf +"\$work"|rm +"\$work/"\*|git clean' "$REPO/scripts/wiki.sh" | grep -q .; then
    bad "blanket deletion present"
else
    ok "deletions are single owned files only"
fi

echo "== summary: $PASS passed, $FAIL failed =="
rm -rf "$T" "$C" "$O" "$E" "$S" /tmp/conflict.log /tmp/missing.log /tmp/auth.log
unset ANPFUEL_WIKI_OVERRIDE
[[ "$FAIL" -eq 0 ]]
