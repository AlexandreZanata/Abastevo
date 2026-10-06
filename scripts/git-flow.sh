#!/usr/bin/env bash
# P01-T14 phase branch and PR lifecycle controller.
# Commands: start, checkpoint, sync, status, finish. Trusted repo only:
# origin must point to AlexandreZanata/brazil-fuel-prices unless
# ANPFUEL_TRUST_OVERRIDE=1 (synthetic tests only, never production).
# Guards: wrong repo, main branch, dirty tree, missing/failed/skipped/
# cancelled checks, changed head/base, unmerged deletion all refuse.
# Dry-run performs zero git mutations (no fetch/push/merge/branch/delete).
# finish runs `make quick-verify` after remote success unless --skip-quick (synthetic tests) or
# --dry-run (guards only). Remote smoke belongs to P01-T17; here the
# lifecycle is proven with synthetic git remotes and a fake `gh` stub.
set -euo pipefail

ROOT="${GIT_FLOW_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
GIT_DIR="$(git -C "$ROOT" rev-parse --absolute-git-dir)"
STATE_FILE="$GIT_DIR/anpfuel-phase.json"
CHECKPOINT_DIR="$GIT_DIR/anpfuel-checkpoints"
EXPECTED_SSH="git@github.com:AlexandreZanata/brazil-fuel-prices.git"
EXPECTED_HTTPS="https://github.com/AlexandreZanata/brazil-fuel-prices.git"

die() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "git-flow: $*"; }

origin_url() { git -C "$ROOT" remote get-url origin 2>/dev/null || echo ""; }

check_trusted_repo() {
    if [[ "${ANPFUEL_TRUST_OVERRIDE:-0}" == "1" ]]; then
        return 0
    fi
    local url
    url="$(origin_url)"
    if [[ "$url" != "$EXPECTED_SSH" && "$url" != "$EXPECTED_HTTPS" ]]; then
        die "untrusted origin '$url' (expected fuel repository); refusing"
    fi
}

require_clean_tree() {
    if [[ -n "$(git -C "$ROOT" status --porcelain)" ]]; then
        die "dirty working tree; commit or discard owned changes first"
    fi
}

require_phase_branch() {
    local b
    b="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    if [[ "$b" == "main" ]]; then
        die "refusing on main branch; use dev or an existing codex/phase-NN-slug branch"
    fi
    case "$b" in
        codex/phase-*|dev) ;;
        *) die "refusing on non-phase branch '$b'" ;;
    esac
}

write_state() {
    local phase="$1" slug="$2" branch="$3" base_ref="$4" base_sha="$5"
    printf '{"phase":"%s","slug":"%s","branch":"%s","base_ref":"%s","base_sha":"%s"}\n' \
        "$phase" "$slug" "$branch" "$base_ref" "$base_sha" > "$STATE_FILE"
}

read_state_field() {
    local field="$1"
    grep -o "\"$field\":\"[^\"]*\"" "$STATE_FILE" | cut -d'"' -f4
}

cmd_start() {
    local phase="" slug="" base_ref="origin/main" dry_run=0 from_checkpoint=0 base_given=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase) phase="$2"; shift 2 ;;
            --slug) slug="$2"; shift 2 ;;
            --base) base_ref="$2"; base_given=1; shift 2 ;;
            --from-checkpoint) from_checkpoint=1; shift ;;
            --dry-run) dry_run=1; shift ;;
            *) die "start: unknown flag $1" ;;
        esac
    done
    [[ -n "$phase" && -n "$slug" ]] || die "start requires --phase NN --slug name"
    [[ "$phase" =~ ^[0-9]{2}$ && "$slug" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]] || die "invalid phase/slug"
    check_trusted_repo
    require_clean_tree
    local cur
    cur="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    if [[ "$from_checkpoint" == "1" ]]; then
        require_phase_branch
        [[ "$base_given" == "0" ]] || die "checkpoint start cannot override its parent base"
        local checkpoint expected
        checkpoint="$CHECKPOINT_DIR/${cur//\//_}.json"
        [[ -f "$checkpoint" ]] || die "no local checkpoint for $cur; run checkpoint first"
        expected="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["head"])' "$checkpoint")"
        [[ "$(git -C "$ROOT" rev-parse HEAD)" == "$expected" ]] || die "checkpoint head changed; revalidate affected tests and checkpoint again"
        base_ref="$cur"
    elif [[ "$cur" != "main" ]]; then
        # A new isolated worktree may start detached at the verified base.
        [[ "$cur" == "HEAD" && "$(git -C "$ROOT" rev-parse HEAD)" == "$(git -C "$ROOT" rev-parse "$base_ref")" ]] || die "start from main or detached base; use --from-checkpoint for phase continuation"
    fi
    [[ "$base_ref" =~ ^[a-zA-Z0-9_./-]+$ && "$base_ref" != -* ]] || die "invalid base ref"
    git -C "$ROOT" rev-parse --verify "$base_ref" >/dev/null || die "unknown base $base_ref"
    local base_sha branch
    base_sha="$(git -C "$ROOT" rev-parse "$base_ref")"
    branch="codex/phase-$phase-$slug"
    if git -C "$ROOT" rev-parse --verify "$branch" >/dev/null 2>&1; then
        die "branch $branch already exists; reuse it instead of recreating"
    fi
    if [[ "$dry_run" == "1" ]]; then
        info "dry-run: would create $branch from $base_ref ($base_sha); zero mutations"
        return 0
    fi
    git -C "$ROOT" checkout -b "$branch" "$base_ref"
    write_state "$phase" "$slug" "$branch" "$base_ref" "$base_sha"
    info "started $branch from $base_ref ($base_sha)"
}

cmd_checkpoint() {
    local evidence="" dry_run=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --evidence) evidence="$2"; shift 2 ;;
            --dry-run) dry_run=1; shift ;;
            *) die "checkpoint: unknown flag $1" ;;
        esac
    done
    check_trusted_repo
    require_phase_branch
    require_clean_tree
    [[ -f "$STATE_FILE" ]] || die "missing phase state; start first"
    local branch head blob
    branch="$(git -C "$ROOT" branch --show-current)"
    [[ "$(read_state_field branch)" == "$branch" ]] || die "state branch mismatch"
    [[ "$evidence" == docs/*.md && "$evidence" != *..* ]] || die "evidence must be a tracked docs/*.md path"
    git -C "$ROOT" ls-files --error-unmatch -- "$evidence" >/dev/null 2>&1 || die "untracked evidence"
    [[ -f "$ROOT/$evidence" && ! -L "$ROOT/$evidence" ]] || die "evidence is not a regular file"
    grep -qx 'Status: LOCAL_DONE' "$ROOT/$evidence" || die "evidence must declare Status: LOCAL_DONE"
    grep -qx 'Validation: PASS' "$ROOT/$evidence" || die "local acceptance is not PASS"
    head="$(git -C "$ROOT" rev-parse HEAD)"
    blob="$(git -C "$ROOT" rev-parse "HEAD:$evidence")"
    if [[ "$dry_run" == "1" ]]; then
        info "dry-run: would checkpoint $branch at $head; zero mutations"
        return 0
    fi
    mkdir -p "$CHECKPOINT_DIR"
    python3 - "$CHECKPOINT_DIR/${branch//\//_}.json" "$head" "$evidence" "$blob" <<'JSON'
import json, sys
from pathlib import Path
Path(sys.argv[1]).write_text(json.dumps({"head": sys.argv[2], "evidence": sys.argv[3], "blob": sys.argv[4], "status": "LOCAL_DONE", "integration": "PENDING"}) + "\n")
JSON
    info "LOCAL_DONE $branch ($head); INTEGRATION_PENDING. No CI, PR, merge or release claim."
}

cmd_sync() {
    local dry_run=0
    [[ "${1:-}" == "--dry-run" ]] && dry_run=1
    check_trusted_repo
    require_phase_branch
    require_clean_tree
    [[ -f "$STATE_FILE" ]] || die "missing phase state; start first"
    local base_ref
    base_ref="$(read_state_field base_ref)"
    git -C "$ROOT" rev-parse --verify "$base_ref" >/dev/null || die "unknown base $base_ref"
    if [[ "$dry_run" == "1" ]]; then
        info "dry-run: would merge $base_ref into $(git -C "$ROOT" rev-parse --abbrev-ref HEAD); zero mutations"
        return 0
    fi
    git -C "$ROOT" merge --no-edit "$base_ref"
    info "synced with $base_ref"
}

cmd_status() {
    check_trusted_repo
    local b head base_ref base_sha
    b="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    head="$(git -C "$ROOT" rev-parse HEAD)"
    echo "branch: $b"
    echo "head: $head"
    echo "origin: $(origin_url)"
    echo "dirty: $(git -C "$ROOT" status --porcelain | wc -l | tr -d ' ')"
    if [[ -f "$STATE_FILE" ]]; then
        echo "state: $(cat "$STATE_FILE")"
        base_ref="$(read_state_field base_ref)"
        if git -C "$ROOT" rev-parse --verify "$base_ref" >/dev/null 2>&1; then
            base_sha="$(git -C "$ROOT" rev-parse "$base_ref")"
            echo "base: $base_ref ($base_sha)"
        fi
    else
        echo "state: none"
    fi
}

gh_json() {
    # Thin wrapper so tests can stub `gh` on PATH.
    gh "$@"
}

cmd_finish() {
    local required="Quick verification" dry_run=0 skip_quick=0 pr_num="" explicit_base=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --base) explicit_base="$2"; shift 2 ;;
            --required) required="$2"; shift 2 ;;
            --dry-run) dry_run=1; shift ;;
            --skip-quick) skip_quick=1; shift ;;
            --pr) pr_num="$2"; shift 2 ;;
            *) die "finish: unknown flag $1" ;;
        esac
    done
    check_trusted_repo
    require_phase_branch
    require_clean_tree
    local branch head base_ref base_sha
    branch="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    if [[ -n "$explicit_base" ]]; then
        [[ "$explicit_base" == "origin/main" ]] || die "explicit final target must be origin/main"
        base_ref="$explicit_base"
    else
        [[ -f "$STATE_FILE" ]] || die "missing phase state; use --base origin/main for existing dev/phase delivery"
        [[ "$(read_state_field branch)" == "$branch" ]] || die "state branch mismatch; use --base origin/main for existing delivery"
        base_ref="$(read_state_field base_ref)"
    fi
    git -C "$ROOT" rev-parse --verify "$base_ref" >/dev/null || die "unknown base $base_ref"
    base_sha="$(git -C "$ROOT" rev-parse "$base_ref")"
    head="$(git -C "$ROOT" rev-parse HEAD)"
    # Base must be merged (no divergence); otherwise sync first.
    if ! git -C "$ROOT" merge-base --is-ancestor "$base_sha" "$head"; then
        die "base $base_ref ($base_sha) not merged; run sync first"
    fi
    if [[ "$dry_run" == "1" ]]; then
        info "dry-run: guards pass for $branch ($head) on $base_ref ($base_sha); zero mutations (quick + remote deferred)"
        return 0
    fi
    [[ "$skip_quick" == "0" || "${ANPFUEL_TRUST_OVERRIDE:-0}" == "1" ]] || die "--skip-quick is synthetic-test-only"
    [[ "$required" =~ [^[:space:],] ]] || die "required check list must not be empty"
    gh_json auth status >/dev/null || die "missing gh authorization; refusing remote mutation (retryable)"
    local pr_json pr_head pr_base pr_state
    if [[ -n "$pr_num" ]]; then
        pr_json="$(gh_json pr view "$pr_num" --json number,headRefOid,baseRefName,headRefName,state,isDraft)"
    else
        pr_json="$(gh_json pr view "$branch" --json number,headRefOid,baseRefName,headRefName,state,isDraft)"
    fi
    pr_json="$(printf '%s' "$pr_json" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin), separators=(",", ":")))')"
    pr_head="$(echo "$pr_json" | grep -o '"headRefOid":"[^"]*"' | cut -d'"' -f4)"
    pr_base="$(echo "$pr_json" | grep -o '"baseRefName":"[^"]*"' | cut -d'"' -f4)"
    pr_state="$(echo "$pr_json" | grep -o '"state":"[^"]*"' | cut -d'"' -f4)"
    local pr_number
    pr_number="$(echo "$pr_json" | grep -o '"number":[0-9]*' | cut -d: -f2)"
    [[ "$pr_json" == *'"isDraft":false'* ]] || die "PR is draft or draft state unavailable; ready only at final batch closure"
    [[ "$pr_state" == "OPEN" ]] || die "PR $pr_number not open ($pr_state)"
    [[ "$pr_base" == "main" ]] || die "PR base is $pr_base, expected main"
    [[ "$(echo "$pr_json" | grep -o '"headRefName":"[^"]*"' | cut -d'"' -f4)" == "$branch" ]] || die "PR head branch mismatch"
    [[ "$pr_head" == "$head" ]] || die "PR head $pr_head != local $head (changed head; revalidate)"
    # A stacked phase parent is not the PR target. Fetch actual main once;
    # never treat an unchanged local parent ref as current base evidence.
    git -C "$ROOT" fetch origin main
    local current_base
    current_base="$(git -C "$ROOT" rev-parse origin/main)"
    git -C "$ROOT" merge-base --is-ancestor "$current_base" "$head" || die "origin/main moved; merge main and revalidate affected checks before final integration"
    # Required checks on the exact head: missing/failed/skipped/cancelled all refuse.
    local IFS=,
    for ctx in $required; do
        # Trim list separators only: check names may contain interior
        # spaces (e.g. "Quick verification"), which tr -d would destroy.
        ctx="$(echo "$ctx" | sed -e 's/^ *//' -e 's/ *$//')"
        [[ -n "$ctx" ]] || continue
        local runs status conclusion
        runs="$(gh_json api "repos/AlexandreZanata/brazil-fuel-prices/commits/$head/check-runs" --paginate 2>/dev/null || true)"
        # Trailing || true keeps a genuine miss loud: without it, pipefail
        # turns an empty grep into a silent exit instead of the die below.
        status="$(echo "$runs" | grep -o "\"name\":\"$ctx\"[^}]*\"status\":\"[^\"]*\"" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 | head -1 || true)"
        conclusion="$(echo "$runs" | grep -o "\"name\":\"$ctx\"[^}]*\"conclusion\":\"[^\"]*\"" | grep -o '"conclusion":"[^"]*"' | cut -d'"' -f4 | head -1 || true)"
        if [[ -z "$status" ]]; then
            die "missing required check '$ctx' on $head"
        fi
        if [[ "$status" != "completed" || "$conclusion" != "success" ]]; then
            die "required check '$ctx' is $status/$conclusion (need completed/success)"
        fi
    done
    # Remote pending/failure exits above: no waiting loop or repeated local
    # aggregate while a PR is queued. Invoke finish only at batch closure.
    if [[ "$skip_quick" == "0" ]]; then
        info "running local quick gate once at final batch integration"
        make -C "$ROOT" quick-verify
        require_clean_tree
    fi
    info "pushing $branch and merging PR $pr_number with head guard $head"
    git -C "$ROOT" push origin "$branch"
    gh_json pr merge "$pr_number" --merge --match-head-commit "$head"
    git -C "$ROOT" fetch origin
    if ! git -C "$ROOT" merge-base --is-ancestor "$head" "origin/main"; then
        die "merged head not in origin/main; retry before cleanup"
    fi
    if [[ "$(git -C "$ROOT" rev-parse --git-common-dir)" != "$(git -C "$ROOT" rev-parse --git-dir)" ]]; then
        # Do not take over main when it belongs to another worktree.
        git -C "$ROOT" checkout --detach origin/main
    else
        git -C "$ROOT" checkout main
        git -C "$ROOT" merge --ff-only origin/main
    fi
    if [[ "$branch" == "dev" ]]; then
        # dev is maintained. Fast-forward it only after proving the guarded
        # merge contains its head; no resets or forced updates.
        git -C "$ROOT" checkout dev
        git -C "$ROOT" merge --ff-only origin/main
        git -C "$ROOT" push origin dev
        info "integrated dev as $head (PR $pr_number); dev synchronized and retained"
        return 0
    fi
    # Always delete the merged phase branch locally and remotely, verified.
    # Safe deletion only: -d refuses unmerged work; never -D.
    git -C "$ROOT" branch -d "$branch"
    git -C "$ROOT" push origin --delete "$branch"
    git -C "$ROOT" fetch origin --prune
    if git -C "$ROOT" rev-parse --verify "origin/$branch" >/dev/null 2>&1; then
        die "remote branch origin/$branch still exists after deletion"
    fi
    info "integrated $branch as $head (PR $pr_number); branch deleted locally and remotely"
}

case "${1:-}" in
    start) shift; cmd_start "$@" ;;
    checkpoint) shift; cmd_checkpoint "$@" ;;
    sync) shift; cmd_sync "$@" ;;
    status) shift; cmd_status "$@" ;;
    finish) shift; cmd_finish "$@" ;;
    *) echo "usage: git-flow.sh {start|checkpoint|sync|status|finish} [flags]" >&2; exit 1 ;;
esac
