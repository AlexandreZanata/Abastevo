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
    cat > "$dir/bin/make" <<'SH'
#!/usr/bin/env bash
echo "local-quick $*" >> "$GH_LOG"
SH
    chmod +x "$dir/bin/make"
    chmod +x "$dir/bin/gh"
    [[ -x "$dir/bin/gh" ]] || { echo "FAIL: fake gh not executable" >&2; exit 1; }
}

echo "== start creates branch+state; dry-run mutates nothing =="
read -r TMP WORK <<< "$(make_work)"
export GIT_FLOW_ROOT="$WORK" ANPFUEL_TRUST_OVERRIDE=1
BEFORE="$(snapshot "$WORK")"
if GIT_FLOW_ROOT="$WORK" ANPFUEL_TRUST_OVERRIDE=1 bash "$FLOW" start --phase 02 --slug lifecycle --dry-run >/dev/null; then
    AFTER="$(snapshot "$WORK")"
    if [[ "$BEFORE" == "$AFTER" ]]; then ok "start dry-run zero mutations"; else bad "start dry-run mutated"; fi
else bad "start dry-run should pass"; fi
if [[ -e "$WORK/.git/anpfuel-phase.json" ]]; then bad "dry-run wrote state"; else ok "dry-run wrote no state"; fi
bash "$FLOW" start --phase 02 --slug lifecycle >/dev/null
if [[ "$(git -C "$WORK" rev-parse --abbrev-ref HEAD)" == "codex/phase-02-lifecycle" ]]; then ok "start creates phase branch"; else bad "start branch"; fi
if [[ -f "$WORK/.git/anpfuel-phase.json" ]]; then ok "start writes state"; else bad "start state"; fi

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
if [[ "$BEFORE" == "$AFTER" ]]; then ok "dry-run lifecycle zero mutations"; else bad "dry-run lifecycle mutated"; fi

echo "== check refusal matrix (fake gh, real mode, skip-quick) =="
GH="$(mktemp -d)"
make_gh_stub "$GH"
export PATH="$GH/bin:$PATH" GH_LOG="$GH/log" GH_AUTH_FAIL=0
hash -r
HEAD_SHA="$(git -C "$WORK" rev-parse HEAD)"
pr_json() { printf '{"number":7,"headRefOid":"%s","baseRefName":"main","headRefName":"%s","state":"OPEN","isDraft":false}' "$1" "$(git -C "$GIT_FLOW_ROOT" branch --show-current)"; }
checks_json() { printf '{"check_runs":[{"name":"fast","status":"%s","conclusion":"%s"},{"name":"integration","status":"%s","conclusion":"%s"},{"name":"test","status":"%s","conclusion":"%s"}]}' "$1" "$2" "$3" "$4" "$5" "$6"; }
export GH_PR_JSON="$GH/pr.json" GH_CHECKS_JSON="$GH/checks.json"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
# Pending final CI must not run a local aggregate or attempt a merge.
: > "$GH_LOG"
printf '{"check_runs":[{"name":"Quick verification","status":"queued","conclusion":null}]}' > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --pr 7 >/dev/null 2>&1; then bad "pending final check accepted"; else ok "pending final check exits"; fi
if [[ ! -s "$GH_LOG" ]]; then ok "pending CI runs no local aggregate or merge"; else bad "pending CI ran aggregate/merge"; fi
if bash "$FLOW" finish --pr 7 --required ' , ' >/dev/null 2>&1; then bad "empty check list accepted"; else ok "empty check list refuses"; fi
checks_json queued "" completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "missing/queued check accepted"; else ok "missing/queued check refuses"; fi
checks_json completed failure completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "failed check accepted"; else ok "failed check refuses"; fi
checks_json completed skipped completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "skipped check accepted"; else ok "skipped check refuses"; fi
checks_json completed cancelled completed success completed success > "$GH_CHECKS_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "cancelled check accepted"; else ok "cancelled check refuses"; fi
checks_json completed success completed success completed success > "$GH_CHECKS_JSON"
pr_json deadbeefdeadbeefdeadbeefdeadbeefdeadbeef > "$GH_PR_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "changed head accepted"; else ok "changed head refuses"; fi
pr_json "$HEAD_SHA" > "$GH_PR_JSON"

pr_json "$HEAD_SHA" | sed 's/"isDraft":false/"isDraft":true/' > "$GH_PR_JSON"
if bash "$FLOW" finish --skip-quick --pr 7 >/dev/null 2>&1; then bad "draft accepted"; else ok "draft refuses"; fi
pr_json "$HEAD_SHA" > "$GH_PR_JSON"

echo "== missing auth mutates nothing (branch current) =="
BEFORE_REF="$(git -C "$WORK" for-each-ref)"
export GH_AUTH_FAIL=1
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "missing auth accepted"; else ok "missing auth refuses"; fi
AFTER_REF="$(git -C "$WORK" for-each-ref)"
if [[ "$BEFORE_REF" == "$AFTER_REF" ]]; then ok "missing auth zero ref mutations"; else bad "missing auth mutated refs"; fi
export GH_AUTH_FAIL=0

# Move base: commit on main and update origin/main, phase branch now behind.
git -C "$WORK" checkout -q main
git -C "$WORK" commit -q --allow-empty -m base-move
git -C "$WORK" push -q origin main
git -C "$WORK" checkout -q codex/phase-02-lifecycle
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then bad "changed base accepted"; else ok "changed base refuses"; fi

echo "== sync merges base, then guarded finish succeeds =="
bash "$FLOW" sync >/dev/null
if git -C "$WORK" merge-base --is-ancestor origin/main HEAD; then ok "sync merged base"; else bad "sync missed base"; fi
HEAD_SHA="$(git -C "$WORK" rev-parse HEAD)"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
: > "$GH_LOG"
if bash "$FLOW" finish --skip-quick --pr 7 --required "fast,integration,test" >/dev/null 2>&1; then
    if grep -q "fake-merge" "$GH_LOG"; then ok "finish merges with head guard"; else bad "finish missed merge guard"; fi
    if [[ "$(git -C "$WORK" rev-parse --abbrev-ref HEAD)" == "main" ]]; then ok "finish lands on main"; else bad "finish branch"; fi
    if git -C "$WORK" rev-parse --verify codex/phase-02-lifecycle >/dev/null 2>&1; then bad "phase branch not cleaned"; else ok "phase branch cleaned locally"; fi
    if git -C "$WORK" rev-parse --verify origin/codex/phase-02-lifecycle >/dev/null 2>&1; then bad "remote branch not cleaned"; else ok "remote branch cleaned"; fi
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
if bash "$FLOW" finish --pr 7 --required "Quick verification" >/dev/null 2>&1; then
    if grep -q "fake-merge" "$GH_LOG"; then ok "multi-word context merges with head guard"; else bad "multi-word context missed merge guard"; fi
    if [[ "$(grep -c '^local-quick ' "$GH_LOG")" == 1 ]]; then ok "final success runs local quick once"; else bad "local quick count"; fi
    if git -C "$WORK" rev-parse --verify origin/codex/phase-03-words >/dev/null 2>&1; then bad "words branch not cleaned"; else ok "words branch cleaned remotely"; fi
else bad "multi-word required check refused"; fi

echo "== linked worktree finalization preserves occupied main =="
git -C "$WORK" worktree add -q --detach "$TMP/linked" origin/main
export GIT_FLOW_ROOT="$TMP/linked"
bash "$FLOW" start --phase 04 --slug worktree >/dev/null
HEAD_SHA="$(git -C "$GIT_FLOW_ROOT" rev-parse HEAD)"
pr_json "$HEAD_SHA" > "$GH_PR_JSON"
: > "$GH_LOG"
if bash "$FLOW" finish --pr 7 --required "Quick verification" >/dev/null 2>&1; then
    if [[ "$(git -C "$WORK" branch --show-current)" == main && "$(git -C "$GIT_FLOW_ROOT" rev-parse --abbrev-ref HEAD)" == HEAD ]]; then ok "linked finish leaves occupied main intact"; else bad "linked finish moved another worktree"; fi
    if git -C "$WORK" show-ref --verify --quiet refs/heads/codex/phase-04-worktree; then bad "linked merged branch retained"; else ok "linked merged branch safely deleted"; fi
else bad "linked finalization failed"; fi
git -C "$WORK" worktree remove "$TMP/linked"
export GIT_FLOW_ROOT="$WORK"

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
