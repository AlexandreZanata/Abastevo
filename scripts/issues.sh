#!/usr/bin/env bash
# P01-T15 issue and milestone reconciliation adapter.
# Reads ONLY the requested phase tasks from ROADMAP.md, matches existing
# issues by the stable marker <!-- fuel-task: PNN-TMM --> (exact, never a
# loose title prefix) across open AND closed records with pagination, and
# previews (read-only) or synchronizes owned fields/labels/milestone links.
# Never closes issues: local completion leaves them AWAITING_PHASE_MERGE;
# only the verified phase PR merge closes them. Never touches other phases'
# records (e.g. historical P01). Dry-run performs zero writes (GETs only).
# On API failure it stops immediately, keeps the ledger, and reports the
# retryable operation; a retry only attempts missing mutations.
# This adapter never closes, reopens, deletes or force-updates issues.
set -euo pipefail

ROOT="${ISSUES_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
ROADMAP="$ROOT/ROADMAP.md"
LEDGER_DEFAULT="$ROOT/docs/planning/phase-ledger.json"
OWNER_REPO="AlexandreZanata/brazil-fuel-prices"
EXPECTED_SSH="git@github.com:AlexandreZanata/brazil-fuel-prices.git"
EXPECTED_HTTPS="https://github.com/AlexandreZanata/brazil-fuel-prices.git"
MARKER_PREFIX="fuel-task:"

die() { echo "ERROR: $*" >&2; exit 1; }
info() { echo "issues: $*"; }

check_trusted_repo() {
    if [[ "${ANPFUEL_TRUST_OVERRIDE:-0}" == "1" ]]; then
        return 0
    fi
    local url
    url="$(git -C "$ROOT" remote get-url origin 2>/dev/null || echo "")"
    if [[ "$url" != "$EXPECTED_SSH" && "$url" != "$EXPECTED_HTTPS" ]]; then
        die "untrusted origin '$url'; refusing"
    fi
}

phase_tasks() {
    # Prints "ID|TITLE|PRIORITY" for tasks P<NN>-T<MM> of the requested phase.
    local nn="$1"
    grep -E "^### P$nn-T[0-9]+ — " "$ROADMAP" | while IFS= read -r line; do
        local id title prio
        id="$(echo "$line" | grep -o "P$nn-T[0-9]*")"
        title="$(echo "$line" | sed -E "s/^### P$nn-T[0-9]+ — //")"
        prio="$(grep -A3 "^### $id — " "$ROADMAP" | grep -o "/ [A-Z]* /" | head -1 | tr -d '/ ')"
        printf "%s|%s|%s\n" "$id" "$title" "${prio:-MUST}"
    done
}

phase_milestone_title() {
    grep -E "^## P$1 — " "$ROADMAP" | head -1 | sed -E 's/^## //'
}

task_field() {
    # Extracts a "- **Field:** value" line from a task section.
    local id="$1" field="$2"
    awk -v id="$id" -v f="$field" '
        $0 ~ "^### "id" — " {insec=1; next}
        /^### P[0-9]+-T[0-9]+/ {insec=0}
        /^## / {insec=0}
        insec && $0 ~ "^- \\*\\*"f":\\*\\* " {sub("^- \\*\\*"f":\\*\\* ", ""); print; exit}
    ' "$ROADMAP"
}

managed_body() {
    # Generates ONLY the owned managed block. The stable task marker lives
    # on its own line outside this block: create callers prepend it, merge
    # callers substitute just the block span, so repeated syncs are stable
    # and never stack duplicate markers.
    local id="$1" title="$2" prio="$3" milestone="$4"
    local anchor goal accept valid deps
    anchor="ROADMAP.md#$(echo "$id" | tr '[:upper:]' '[:lower:]')"
    goal="$(task_field "$id" "Goal")"
    accept="$(task_field "$id" "Acceptance criteria")"
    valid="$(task_field "$id" "Validation commands")"
    deps="$(task_field "$id" "Dependencies")"
    cat <<EOF
<!-- anpfuel-managed:begin -->
- Phase/milestone: $milestone
- Source: $anchor
- Priority: $prio / risk: standard
- Dependencies: $deps
- Goal: $goal
- Acceptance: $accept
- Validation: $valid
- Status: AWAITING_PHASE_MERGE (this adapter never closes issues; the verified phase merge does)
<!-- anpfuel-managed:end -->
EOF
}

marker_line() { printf '<!-- %s %s -->\n' "$MARKER_PREFIX" "$1"; }

merge_managed_body() {
    # Replaces (or appends) the managed block in an existing body file,
    # preserving every human byte outside the block.
    local existing_file="$1" managed_file="$2"
    if grep -q "anpfuel-managed:begin" "$existing_file"; then
        python3 - "$existing_file" "$managed_file" <<'PY'
import re, sys
old = open(sys.argv[1]).read()
new = open(sys.argv[2]).read().strip()
pat = re.compile(r'<!-- anpfuel-managed:begin -->.*?<!-- anpfuel-managed:end -->', re.S)
assert pat.search(old), "managed block vanished"
open(sys.argv[1] + ".new", "w").write(pat.sub(lambda _: new, old, count=1))
PY
        mv "$existing_file.new" "$existing_file"
    else
        { cat "$existing_file"; echo; cat "$managed_file"; } > "$existing_file.new"
        mv "$existing_file.new" "$existing_file"
    fi
}

list_all_issues() {
    # Prints one flat JSON object per line (number/state/title/labels/marker),
    # paginating state=all. Bodies are reduced to their stable task marker so
    # nested API structures never break matching. Full bodies are fetched per
    # issue only when an update is actually needed.
    local page=1 per_page="${ISSUES_PER_PAGE:-100}"
    while true; do
        local out
        out="$(gh api "repos/$OWNER_REPO/issues?state=all&per_page=$per_page&page=$page")" \
            || die "issue list page $page failed (retryable)"
        echo "$out" | python3 -c "
import json, re, sys
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit('bad issue JSON page')
for it in data:
    body = it.get('body') or ''
    m = re.search(r'<!--\s*fuel-task:\s*(P\d{2}-T\d{2}[A-Z]?)\s*-->', body)
    labels = ','.join(sorted(str(l.get('name', '')) for l in it.get('labels', [])))
    print(json.dumps({'number': it['number'], 'state': it.get('state', ''),
                       'title': it.get('title', ''), 'labels': labels,
                       'marker': m.group(1) if m else ''}))
"
        local n
        n="$(echo "$out" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")"
        [[ "$n" -lt "$per_page" ]] && break
        page=$((page + 1))
        [[ "$page" -gt 50 ]] && die "too many issue pages; refusing runaway pagination"
    done
}

find_milestone() {
    local title="$1"
    local out
    out="$(gh api "repos/$OWNER_REPO/milestones?state=all&per_page=100")"
    echo "$out" | python3 -c "
import json, sys
title = '''$title'''
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(0)
for m in data:
    if m.get('title') == title:
        print(str(m.get('number')))
        break
"
}

prio_label() {
    case "$1" in
        MUST) echo "priority:must" ;;
        SHOULD) echo "priority:should" ;;
        LATER) echo "priority:later" ;;
        *) echo "priority:must" ;;
    esac
}

cmd_preview() {
    local nn="" ledger="$LEDGER_DEFAULT"
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase) nn="$2"; shift 2 ;;
            --ledger) ledger="$2"; shift 2 ;;
            *) die "preview: unknown flag $1" ;;
        esac
    done
    [[ -n "$nn" ]] || die "preview requires --phase NN"
    check_trusted_repo
    local milestone tasks
    milestone="$(phase_milestone_title "$nn")"
    [[ -n "$milestone" ]] || die "no roadmap phase header for P$nn"
    tasks="$(phase_tasks "$nn")"
    [[ -n "$tasks" ]] || die "no tasks found for phase P$nn"
    info "phase P$nn milestone: $milestone (ledger: $ledger)"
    local issues
    issues="$(list_all_issues)"
    while IFS='|' read -r id title prio; do
        [[ -n "$id" ]] || continue
        local hits count state numbers
        hits="$(echo "$issues" | grep -F "\"marker\": \"$id\"" || true)"
        count="$(echo "$hits" | grep -c '"number"' || true)"
        if [[ -z "$hits" ]]; then
            echo "TASK=$id ACTION=create MILESTONE='$milestone'"
        elif [[ "$count" -gt 1 ]]; then
            numbers="$(echo "$hits" | grep -o '"number": [0-9]*' | tr '\n' ' ')"
            echo "TASK=$id ACTION=duplicate-refuse NUMBERS='$numbers'"
        else
            state="$(echo "$hits" | grep -o '"state": "[a-z]*"' | cut -d'"' -f4)"
            numbers="$(echo "$hits" | grep -o '"number": [0-9]*' | grep -o '[0-9]*')"
            echo "TASK=$id ACTION=reuse NUMBER=$numbers STATE=$state"
        fi
    done <<< "$tasks"
}

cmd_sync() {
    local nn="" dry_run=0 ledger="$LEDGER_DEFAULT"
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --phase) nn="$2"; shift 2 ;;
            --dry-run) dry_run=1; shift ;;
            --ledger) ledger="$2"; shift 2 ;;
            *) die "sync: unknown flag $1" ;;
        esac
    done
    [[ -n "$nn" ]] || die "sync requires --phase NN"
    check_trusted_repo
    local milestone tasks
    milestone="$(phase_milestone_title "$nn")"
    tasks="$(phase_tasks "$nn")"
    [[ -n "$tasks" && -n "$milestone" ]] || die "unknown phase P$nn"
    # Resolve milestone number (create only in real mode when missing).
    local ms_num
    ms_num="$(find_milestone "$milestone")"
    if [[ -z "$ms_num" ]]; then
        if [[ "$dry_run" == "1" ]]; then
            info "dry-run: would create milestone '$milestone'; zero writes"
            ms_num="DRYRUN"
        else
            ms_num="$(gh api "repos/$OWNER_REPO/milestones" -f title="$milestone" --jq '.number')"
            [[ -n "$ms_num" ]] || die "milestone creation failed (retryable)"
            info "created milestone '$milestone' number $ms_num"
        fi
    fi
    local issues
    issues="$(list_all_issues)"
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    local ledger_new="$tmp/ledger.json"
    if [[ -f "$ledger" ]]; then cp "$ledger" "$ledger_new"; else echo '{"issues":{}}' > "$ledger_new"; fi
    local failed=0
    while IFS='|' read -r id title prio; do
        [[ -n "$id" ]] || continue
        local hits count
        hits="$(echo "$issues" | grep -F "\"marker\": \"$id\"" || true)"
        count="$(echo "$hits" | grep -c '"number"' || true)"
        if [[ -z "$hits" ]]; then
            local want
            want="$(prio_label "$prio")"
            if [[ "$dry_run" == "1" ]]; then
                echo "TASK=$id ACTION=would-create MILESTONE='$milestone' LABELS='phase:P$nn,type:task,$want,risk:standard'"
                continue
            fi
            managed_body "$id" "$title" "$prio" "$milestone" > "$tmp/managed.md"
            { marker_line "$id"; cat "$tmp/managed.md"; } > "$tmp/body.md"
            local num
            num="$(gh issue create --title "$id — $title" --body-file "$tmp/body.md" --milestone "$milestone" --label "phase:P$nn" --label "type:task" --label "$want" --label "risk:standard" --json number --jq '.number')" \
                || { echo "TASK=$id ACTION=create-failed-retryable" >&2; failed=1; break; }
            echo "TASK=$id ACTION=created NUMBER=$num"
            python3 - "$ledger_new" "$id" "$num" <<'PY'
import json, sys
p, tid, num = sys.argv[1], sys.argv[2], int(sys.argv[3])
d = json.load(open(p))
d.setdefault("issues", {})[tid] = num
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
            # Refresh local issue cache so later tasks see the new record.
            issues="$(list_all_issues)" || { echo "TASK=$id ACTION=relist-failed-retryable" >&2; failed=1; break; }
        elif [[ "$count" -gt 1 ]]; then
            echo "TASK=$id ACTION=duplicate-refuse (not creating; resolve manually)" >&2
            failed=1
        else
            local num state body labels
            num="$(echo "$hits" | grep -o '"number": [0-9]*' | grep -o '[0-9]*')"
            state="$(echo "$hits" | grep -o '"state": "[a-z]*"' | cut -d'"' -f4)"
            echo "TASK=$id ACTION=reuse NUMBER=$num STATE=$state (never closed by adapter)"
            if [[ "$dry_run" == "1" ]]; then
                continue
            fi
            body="$(gh issue view "$num" --json body --jq '.body')" \
                || { echo "TASK=$id ACTION=view-failed-retryable" >&2; failed=1; break; }
            printf "%s" "$body" > "$tmp/existing.md"
            managed_body "$id" "$title" "$prio" "$milestone" > "$tmp/managed.md"
            merge_managed_body "$tmp/existing.md" "$tmp/managed.md"
            if ! diff -q "$tmp/existing.md" <(printf "%s" "$body") >/dev/null 2>&1; then
                gh issue edit "$num" --body-file "$tmp/existing.md" >/dev/null \
                    || { echo "TASK=$id ACTION=update-failed-retryable" >&2; failed=1; break; }
                echo "TASK=$id ACTION=managed-updated NUMBER=$num (human notes preserved)"
            fi
            labels="$(gh issue view "$num" --json labels --jq '.labels[].name')" \
                || { echo "TASK=$id ACTION=labels-failed-retryable" >&2; failed=1; break; }
            local want
            want="$(prio_label "$prio")"
            if ! echo "$labels" | grep -qx "$want"; then
                gh issue edit "$num" --add-label "$want" >/dev/null \
                    || { echo "TASK=$id ACTION=label-failed-retryable" >&2; failed=1; break; }
                echo "TASK=$id ACTION=label-added NUMBER=$num LABEL=$want"
            fi
            python3 - "$ledger_new" "$id" "$num" <<'PY'
import json, sys
p, tid, num = sys.argv[1], sys.argv[2], int(sys.argv[3])
d = json.load(open(p))
d.setdefault("issues", {})[tid] = num
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
        fi
    done <<< "$tasks"
    if [[ "$dry_run" == "1" ]]; then
        trap - EXIT
        rm -rf "$tmp"
        info "dry-run complete; ledger untouched at $ledger"
        return 0
    fi
    # Publish ledger only after per-task successes; failures keep prior ledger.
    if [[ "$failed" == "0" ]]; then
        python3 - "$ledger_new" "$nn" "$milestone" "$ms_num" <<'PY'
import json, sys
p, nn, ms, msn = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
d = json.load(open(p))
d["phase"] = nn
d["milestone_title"] = ms
try:
    d["milestone"] = int(msn)
except ValueError:
    d["milestone"] = msn
json.dump(d, open(p, "w"), indent=2, sort_keys=True)
PY
        mkdir -p "$(dirname "$ledger")"
        cp "$ledger_new" "$ledger"
        info "ledger written to $ledger"
        trap - EXIT
        rm -rf "$tmp"
    else
        mkdir -p "$(dirname "$ledger")"
        cp "$ledger_new" "$ledger"
        trap - EXIT
        rm -rf "$tmp"
        die "reconciliation incomplete; ledger kept with completed IDs — retry only missing mutations"
    fi
}

case "${1:-}" in
    preview) shift; cmd_preview "$@" ;;
    sync) shift; cmd_sync "$@" ;;
    *) echo "usage: issues.sh {preview|sync} --phase NN [--dry-run] [--ledger FILE]" >&2; exit 1 ;;
esac
