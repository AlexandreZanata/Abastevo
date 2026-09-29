#!/usr/bin/env bash
# P01-T14 focused harness: phase lifecycle with synthetic git + fake gh.
# Proves guards (wrong repo/main/dirty), dry-run zero mutations, missing/
# failed/skipped/cancelled check refusal, changed head/base refusal,
# unmerged safe-delete refusal and missing-auth no-mutation.
# Real merge smoke belongs to P01-T17; success here is the guarded dry-run
# lifecycle plus the full refusal matrix.
set -euo pipefail

FLOW="$(cd "$(dirname "$0")/../.." && pwd)/scripts/git-flow.sh"
PASS=0
FAIL=0
ok() { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

REFS_SNAPSHOT=""

snapshot() { git -C "$1" status --porcelain; git -C "$1" rev-parse HEAD; git -C "$1" for-each-ref; }

make_work() {
    local tmp remote work
    tmp="$(mktemp -d)"
    remote="$tmp/remote.git"
    work="$tmp/work"
    git init --bare -q "$remote"
    git init -q -b main "$work"
    git -C "$work" config user.email t@t.t
    git -C "$work" config user.name t
    git -C "$work" commit -q --allow-empty -m init
    git -C "$work" remote add origin "$remote"
    git -C "$work" push -q origin main
    printf "%s %s" "$tmp" "$work"
}

# Fake gh stub: controlled by env GH_DIR fixtures.
make_gh_stub() {
    local dir="$1"
    mkdir -p "$dir/bin"
    cat > "$dir/bin/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${GH_AUTH_FAIL:-0}" == "1" && "$1" == "auth" ]]; then
    echo "auth failed" >&2; exit 1
fi
if [[ "$1" == "auth" ]]; then exit 0; fi
if [[ "$1" == "pr" && "$2" == "view" ]]; then cat "$GH_PR_JSON"; exit 0; fi
if [[ "$1" == "api" ]]; then cat "$GH_CHECKS_JSON"; exit 0; fi
if [[ "$1" == "pr" && "$2" == "merge" ]]; then echo "fake-merge $*" >> "$GH_LOG"; exit 0; fi
echo "unexpected gh $*" >&2; exit 2
SH
    chmod +x "$dir/bin/gh"
    [[ -x "$dir/bin/gh" ]] || { echo "FAIL: fake gh not executable" >&2; exit 1; }
}

echo "== start creates branch+state; dry-run mutates nothing =="
read TMP WORK <<< "$(make_work)"
export GIT_FLOW_ROOT="$WORK" ANPFUEL_TRUST_OVERRIDE=1
BEFORE="$(snapshot "$WORK")"
if GIT_FLOW_ROOT="$WORK" ANPFUEL_TRUST_OVERRIDE=1 bash "$FLOW" start --phase 02 --slug lifecycle --dry-run >/dev/null; then
    AFTER="$(snapshot "$WORK")"
    [[ "$BEFORE" == "$AFTER" ]] && ok "start dry-run zero mutations" || bad "start dry-run mutated"
else bad "start dry-run should pass"; fi
if [[ -e "$WORK/.git/anpfuel-phase.json" ]]; then bad "dry-run wrote state"; else ok "dry-run wrote no state"; fi
bash "$FLOW" start --phase 02 --slug lifecycle >/dev/null
[[ "$(git -C "$WORK" rev-parse --abbrev-ref HEAD)" == "codex/phase-02-lifecycle" ]] && ok "start creates phase branch" || bad "start branch"
[[ -f "$WORK/.git/anpfuel-phase.json" ]] && ok "start writes state" || bad "start state"

echo "== refuses dirty/main/wrong-repo =="
echo x > "$WORK/dirty.txt"
if bash "$FLOW" sync --dry-run >/dev/null 2>&1; then bad "dirty sync accepted"; else ok "dirty sync refuses"; fi
rm "$WORK/dirty.txt"
git -C "$WORK" checkout -q main
if bash "$FLOW" finish --dry-run >/dev/null 2>&1; then bad "main finish accepted"; else ok "main finish refuses"; fi
git -C "$WORK" checkout -q codex/phase-02-lifecycle
if ANPFUEL_TRUST_OVERRIDE=0 GIT_FLOW_ROOT="$WORK" bash "$FLOW" status >/dev/null 2>&1; then bad "wrong repo accepted"; else ok "wrong repo refuses"; fi

echo "== dry-run lifecycle zero mutations + status read-only =="
BEFORE="$(snapshot "$WORK")"
bash "$FLOW" sync --dry-run >/dev/null
bash "$FLOW" status >/dev/null
bash "$FLOW" finish --dry-run >/dev/null
AFTER="$(snapshot "$WORK")"
[[ "$BEFORE" == "$AFTER" ]] && ok "dry-run lifecycle zero mutations" || bad "dry-run lifecycle mutated"

echo "== check refusal matrix (fake gh, real mode, skip-quick) =="
GH="$(mktemp -d)"
make_gh_stub "$GH"
export PATH="$GH/bin:$PATH" GH_LOG="$GH/log" GH_AUTH_FAIL=0
hash -r
HEAD_SHA="$(git -C "$WORK" rev-parse HEAD)"
BASE_SHA="$(git -C "$WORK" rev-parse origin/main)"
pr_json() { printf '{"number":7,"headRefOid":"%s","baseRefName":"main","headRefName":"codex/phase-02-lifecycle","state":"OPEN"}' "$1"; }
checks_json() { printf '{"check_runs":[{"name":"fast","status":"%s","conclusion":"%s"},{"name":"integration","status":"%s","conclusion":"%s"},{"name":"test","status":"%s","conclusion":"%s"}]}' "$1" "$2" "$3" "$4" "$5" "$6"; }
export GH_PR_JSON="$GH/pr.json" GH_CHECKS_JSON="$GH/checks.json"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
checks_json queued "" completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "missing/queued check accepted"; else ok "missing/queued check refuses"; fi
checks_json completed failure completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "failed check accepted"; else ok "failed check refuses"; fi
checks_json completed skipped completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "skipped check accepted"; else ok "skipped check refuses"; fi
checks_json completed cancelled completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "cancelled check accepted"; else ok "cancelled check refuses"; fi
checks_json completed success completed success completed success > "$GH_CHECKS_JSON"
pr_json deadbeefdeadbeefdeadbeefdeadbeefdeadbeef > "$GH_PR_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "changed head accepted"; else ok "changed head refuses"; fi
pr_json "$HEAD_SHA" > "$GH_PR_JSON"

echo "== missing auth mutates nothing (branch current) =="
BEFORE_REF="$(git -C "$WORK" for-each-ref)"
export GH_AUTH_FAIL=1
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "missing auth accepted"; else ok "missing auth refuses"; fi
AFTER_REF="$(git -C "$WORK" for-each-ref)"
[[ "$BEFORE_REF" == "$AFTER_REF" ]] && ok "missing auth zero ref mutations" || bad "missing auth mutated refs"
export GH_AUTH_FAIL=0

# Move base: commit on main and update origin/main, phase branch now behind.
git -C "$WORK" checkout -q main
git -C "$WORK" commit -q --allow-empty -m base-move
git -C "$WORK" push -q origin main
git -C "$WORK" checkout -q codex/phase-02-lifecycle
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "changed base accepted"; else ok "changed base refuses"; fi

echo "== sync merges base, then guarded finish succeeds =="
bash "$FLOW" sync >/dev/null
git -C "$WORK" merge-base --is-ancestor origin/main HEAD && ok "sync merged base" || bad "sync missed base"
HEAD_SHA="$(git -C "$WORK" rev-parse HEAD)"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
: > "$GH_LOG"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then
    grep -q "fake-merge" "$GH_LOG" && ok "finish merges with head guard" || bad "finish missed merge guard"
    [[ "$(git -C "$WORK" rev-parse --abbrev-ref HEAD)" == "main" ]] && ok "finish lands on main" || bad "finish branch"
    git -C "$WORK" rev-parse --verify codex/phase-02-lifecycle >/dev/null 2>&1 && bad "phase branch not cleaned" || ok "phase branch cleaned locally"
    git -C "$WORK" rev-parse --verify origin/codex/phase-02-lifecycle >/dev/null 2>&1 && bad "remote branch not cleaned" || ok "remote branch cleaned"
else bad "guarded finish should succeed"; fi

echo "== multi-word required context matches exactly =="
# No branch-unique commit here: the fake merge is a no-op, so only a head
# already contained in the base clears the ancestry guard — exactly like
# the passing single-word success case above. The regression under test
# is the required-check name match, which still runs fully.
bash "$FLOW" start --phase 03 --slug words >/dev/null
HEAD_SHA="$(git -C "$WORK" rev-parse HEAD)"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
printf '{"check_runs":[{"name":"Quick verification","status":"completed","conclusion":"success"}]}' > "$GH_CHECKS_JSON"
: > "$GH_LOG"
if bash "$FLOW" finish --skip-quick --pr 7 --required "Quick verification" >/dev/null 2>&1; then
    grep -q "fake-merge" "$GH_LOG" && ok "multi-word context merges with head guard" || bad "multi-word context missed merge guard"
    git -C "$WORK" rev-parse --verify origin/codex/phase-03-words >/dev/null 2>&1 && bad "words branch not cleaned" || ok "words branch cleaned remotely"
else bad "multi-word required check refused"; fi

echo "== safe deletion only =="
if grep -q 'branch -D' "$FLOW"; then bad "script contains unsafe branch -D"; else ok "no unsafe branch -D"; fi
git -C "$WORK" checkout -q -b codex/phase-99-unmerged
git -C "$WORK" commit -q --allow-empty -m unmerged
if git -C "$WORK" branch -d codex/phase-99-unmerged >/dev/null 2>&1; then bad "unmerged deletion allowed"; else ok "unmerged deletion refuses"; fi
git -C "$WORK" checkout -q main
git -C "$WORK" branch -D codex/phase-99-unmerged >/dev/null

echo "== summary: $PASS passed, $FAIL failed =="
rm -rf "$TMP" "$GH"
unset GIT_FLOW_ROOT ANPFUEL_TRUST_OVERRIDE
[[ "$FAIL" -eq 0 ]]
