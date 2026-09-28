#!/usr/bin/env bash
# P01-T14 phase branch and PR lifecycle controller.
# Commands: start, sync, status, finish. Trusted repo only:
# origin must point to AlexandreZanata/brazil-fuel-prices unless
# ANPFUEL_TRUST_OVERRIDE=1 (synthetic tests only, never production).
# Guards: wrong repo, main branch, dirty tree, missing/failed/skipped/
# cancelled checks, changed head/base, unmerged deletion all refuse.
# Dry-run performs zero git mutations (no fetch/push/merge/branch/delete).
# finish runs `make quick-verify` once unless --skip-quick (tests) or
# --dry-run (guards only). Remote smoke belongs to P01-T17; here the
# lifecycle is proven with synthetic git remotes and a fake `gh` stub.
set -euo pipefail

ROOT="${GIT_FLOW_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
STATE_FILE="$ROOT/.git/anpfuel-phase.json"
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
        die "refusing on main branch; use a codex/phase-NN-slug branch"
    fi
    case "$b" in
        codex/phase-*) ;;
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
    local phase="" slug="" base_ref="origin/main" dry_run=0
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase) phase="$2"; shift 2 ;;
            --slug) slug="$2"; shift 2 ;;
            --base) base_ref="$2"; shift 2 ;;
            --dry-run) dry_run=1; shift ;;
            *) die "start: unknown flag $1" ;;
        esac
    done
    [[ -n "$phase" && -n "$slug" ]] || die "start requires --phase NN --slug name"
    check_trusted_repo
    require_clean_tree
    local cur
    cur="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    if [[ "$cur" != "main" ]]; then
        die "start from main (currently on $cur)"
    fi
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
    local required="fast,integration,test" dry_run=0 skip_quick=0 pr_num=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
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
    [[ -f "$STATE_FILE" ]] || die "missing phase state; start first"
    local branch head base_ref base_sha
    branch="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
    [[ "$(read_state_field branch)" == "$branch" ]] || die "state branch mismatch"
    base_ref="$(read_state_field base_ref)"
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
    if [[ "$skip_quick" == "0" ]]; then
        info "running local quick gate once"
        make -C "$ROOT" quick-verify
    fi
    gh_json auth status >/dev/null || die "missing gh authorization; refusing remote mutation (retryable)"
    local pr_json pr_head pr_base pr_state
    if [[ -n "$pr_num" ]]; then
        pr_json="$(gh_json pr view "$pr_num" --json number,headRefOid,baseRefName,headRefName,state)"
    else
        pr_json="$(gh_json pr view "$branch" --json number,headRefOid,baseRefName,headRefName,state)"
    fi
    pr_head="$(echo "$pr_json" | grep -o '"headRefOid":"[^"]*"' | cut -d'"' -f4)"
    pr_base="$(echo "$pr_json" | grep -o '"baseRefName":"[^"]*"' | cut -d'"' -f4)"
    pr_state="$(echo "$pr_json" | grep -o '"state":"[^"]*"' | cut -d'"' -f4)"
    local pr_number
    pr_number="$(echo "$pr_json" | grep -o '"number":[0-9]*' | cut -d: -f2)"
    [[ "$pr_state" == "OPEN" ]] || die "PR $pr_number not open ($pr_state)"
    [[ "$pr_base" == "main" ]] || die "PR base is $pr_base, expected main"
    [[ "$pr_head" == "$head" ]] || die "PR head $pr_head != local $head (changed head; revalidate)"
    # Current base must still match the recorded base; a moved base invalidates evidence.
    local current_base
    current_base="$(git -C "$ROOT" rev-parse "$base_ref")"
    [[ "$current_base" == "$base_sha" ]] || die "base moved ($base_sha -> $current_base); sync + revalidate"
    # Required checks on the exact head: missing/failed/skipped/cancelled all refuse.
    local IFS=,
    for ctx in $required; do
        ctx="$(echo "$ctx" | tr -d ' ')"
        [[ -n "$ctx" ]] || continue
        local runs status conclusion
        runs="$(gh_json api "repos/AlexandreZanata/brazil-fuel-prices/commits/$head/check-runs" --paginate 2>/dev/null || true)"
        status="$(echo "$runs" | grep -o "\"name\":\"$ctx\"[^}]*\"status\":\"[^\"]*\"" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 | head -1)"
        conclusion="$(echo "$runs" | grep -o "\"name\":\"$ctx\"[^}]*\"conclusion\":\"[^\"]*\"" | grep -o '"conclusion":"[^"]*"' | cut -d'"' -f4 | head -1)"
        if [[ -z "$status" ]]; then
            die "missing required check '$ctx' on $head"
        fi
        if [[ "$status" != "completed" || "$conclusion" != "success" ]]; then
            die "required check '$ctx' is $status/$conclusion (need completed/success)"
        fi
    done
    info "pushing $branch and merging PR $pr_number with head guard $head"
    git -C "$ROOT" push origin "$branch"
    gh_json pr merge "$pr_number" --merge --match-head-commit "$head"
    git -C "$ROOT" fetch origin
    if ! git -C "$ROOT" merge-base --is-ancestor "$head" "origin/main"; then
        die "merged head not in origin/main; retry before cleanup"
    fi
    git -C "$ROOT" checkout main
    git -C "$ROOT" merge --ff-only origin/main
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
    sync) shift; cmd_sync "$@" ;;
    status) shift; cmd_status "$@" ;;
    finish) shift; cmd_finish "$@" ;;
    *) echo "usage: git-flow.sh {start|sync|status|finish} [flags]" >&2; exit 1 ;;
esac
