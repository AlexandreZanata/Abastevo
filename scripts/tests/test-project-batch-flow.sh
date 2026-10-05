#!/usr/bin/env bash
# Real Git checkpoints, linked worktrees, no remote CI or merge between phases.
set -euo pipefail
FLOW="$(cd "$(dirname "$0")/../.." && pwd)/scripts/git-flow.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export ANPFUEL_TRUST_OVERRIDE=1 GIT_FLOW_ROOT="$TMP/work"
git init -q -b main "$GIT_FLOW_ROOT"
git -C "$GIT_FLOW_ROOT" config user.email test@example.invalid
git -C "$GIT_FLOW_ROOT" config user.name Test
mkdir -p "$GIT_FLOW_ROOT/docs"
printf 'Status: LOCAL_DONE\nValidation: PASS\nCommands: synthetic fixture only\n' > "$GIT_FLOW_ROOT/docs/exit.md"
git -C "$GIT_FLOW_ROOT" add docs
git -C "$GIT_FLOW_ROOT" commit -qm foundation
git -C "$GIT_FLOW_ROOT" update-ref refs/remotes/origin/main HEAD
mkdir "$TMP/bin"
cat > "$TMP/bin/gh" <<'SH'
#!/usr/bin/env bash
echo 'Unexpected remote operation during production' >&2
exit 97
SH
chmod +x "$TMP/bin/gh"
export PATH="$TMP/bin:$PATH"
refuse() { if "$@" >/dev/null 2>&1; then echo "FAIL: accepted $*" >&2; exit 1; fi; }
state_path() { git -C "$GIT_FLOW_ROOT" rev-parse --path-format=absolute --git-path anpfuel-phase.json; }
bash "$FLOW" start --phase 25 --slug catalog >/dev/null
refuse bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint
refuse bash "$FLOW" checkpoint --evidence missing.md
printf 'Status: LOCAL_DONE\nValidation: PENDING\n' > "$GIT_FLOW_ROOT/docs/exit.md"
git -C "$GIT_FLOW_ROOT" commit -qam pending
refuse bash "$FLOW" checkpoint --evidence docs/exit.md
printf 'Status: LOCAL_DONE\nValidation: PASS\nCommands: synthetic fixture only\n' > "$GIT_FLOW_ROOT/docs/exit.md"
refuse bash "$FLOW" checkpoint --evidence docs/exit.md
git -C "$GIT_FLOW_ROOT" commit -qam acceptance
refuse bash "$FLOW" checkpoint --evidence ../exit.md
BEFORE="$(git -C "$GIT_FLOW_ROOT" for-each-ref)"
bash "$FLOW" checkpoint --evidence docs/exit.md --dry-run >/dev/null
refuse bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint
bash "$FLOW" checkpoint --evidence docs/exit.md >/dev/null
[[ "$(git -C "$GIT_FLOW_ROOT" for-each-ref)" == "$BEFORE" ]]
refuse bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint --base origin/main
git -C "$GIT_FLOW_ROOT" commit -q --allow-empty -m invalidate
refuse bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint
bash "$FLOW" checkpoint --evidence docs/exit.md >/dev/null
PARENT="$(git -C "$GIT_FLOW_ROOT" rev-parse HEAD)"
bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint --dry-run >/dev/null
[[ "$(git -C "$GIT_FLOW_ROOT" branch --show-current)" == codex/phase-25-catalog ]]
bash "$FLOW" start --phase 26 --slug discovery --from-checkpoint >/dev/null
[[ "$(git -C "$GIT_FLOW_ROOT" rev-parse HEAD)" == "$PARENT" ]]
grep -q 'codex/phase-25-catalog' "$(state_path)"
git -C "$GIT_FLOW_ROOT" merge-base --is-ancestor "$PARENT" HEAD
bash "$FLOW" checkpoint --evidence docs/exit.md >/dev/null
bash "$FLOW" start --phase 27 --slug suggestions --from-checkpoint >/dev/null
[[ "$(git -C "$GIT_FLOW_ROOT" rev-parse HEAD)" == "$PARENT" ]]
# The linked worktree must have private state and cannot reuse another
# worktree's checkpoint merely because its branch has the same commit.
git -C "$GIT_FLOW_ROOT" worktree add -q -b fixture-main "$TMP/linked" main
export GIT_FLOW_ROOT="$TMP/linked"
git -C "$GIT_FLOW_ROOT" branch -m main-fixture
# Standard starts require main; use a detached main checkpoint in a worktree.
git -C "$GIT_FLOW_ROOT" checkout -q --detach main
bash "$FLOW" start --phase 28 --slug linked >/dev/null
[[ -f "$(state_path)" && -f "$GIT_FLOW_ROOT/.git" ]]
refuse bash "$FLOW" start --phase 29 --slug linked-next --from-checkpoint
bash "$FLOW" checkpoint --evidence docs/exit.md >/dev/null
bash "$FLOW" start --phase 29 --slug linked-next --from-checkpoint >/dev/null
refuse bash "$FLOW" start --phase '30"bad' --slug invalid --from-checkpoint
refuse bash "$FLOW" start --phase 30 --slug invalid/slash --from-checkpoint
echo 'PASS: checkpoint refusal, stale head, dry-run, branch ancestry, worktree isolation; zero gh/CI/merge calls'
