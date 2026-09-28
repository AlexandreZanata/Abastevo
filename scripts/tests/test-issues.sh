#!/usr/bin/env bash
# P01-T15 focused harness: issue/milestone reconciliation with fake gh API.
# Proves paginated matching, duplicate refusal without new creates, human
# notes preserved across managed updates, closed records reused (never
# closed/reopened by the adapter), API-failure stop with ledger kept for
# retry, dry-run zero writes, idempotent second sync and other phases
# (historical P01) untouched. Real publication belongs to an authorized
# session; this harness performs zero remote mutations.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
ISSUES="$REPO/scripts/issues.sh"
PASS=0
FAIL=0
ok() { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1" >&2; FAIL=$((FAIL + 1)); }

write_stub() {
    local dir="$1"
    mkdir -p "$dir/bin"
    cat > "$dir/bin/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${FAIL_ON:-}" && "$*" == *"$FAIL_ON"* ]]; then
    echo "injected API failure for: $*" >&2
    exit 1
fi
op="$1"
if [[ "$op" == "api" ]]; then
    echo "API $*" >> "$FIX/calls.log"
    url="$2"
    if [[ "$url" == *"/issues?"* ]]; then
        python3 - "$FIX/issues.json" "$url" <<'PY'
import json, re, sys
issues = json.load(open(sys.argv[1]))
url = sys.argv[2]
pp = int(re.search(r'[?&]per_page=(\d+)', url).group(1))
pg = int(re.search(r'[?&]page=(\d+)', url).group(1))
print(json.dumps(issues[(pg - 1) * pp:pg * pp]))
PY
        exit 0
    fi
    if [[ "$url" == *"/milestones"* && "$*" != *"-f "* ]]; then
        cat "$FIX/milestones.json"
        exit 0
    fi
    if [[ "$url" == *"/milestones"* ]]; then
        echo "MUT milestones-create $*" >> "$FIX/calls.log"
        python3 - "$FIX/issues.json" "$FIX/milestones.json" "$FIX/next" "$*" <<'PY'
import json, re, sys
_, _, msf, nextf, args = sys.argv
m = re.search(r'title=(.*?) --jq', args)
title = m.group(1) if m else 'untitled'
n = int(open(nextf).read())
ms = json.load(open(msf))
ms.append({"number": n, "title": title})
json.dump(ms, open(msf, "w"))
open(nextf, "w").write(str(n + 1))
if "--jq" in args:
    print(n)
else:
    print(json.dumps({"number": n}))
PY
        exit 0
    fi
    echo "unexpected api $*" >&2
    exit 2
fi
if [[ "$op" == "issue" && "$2" == "create" ]]; then
    echo "MUT issue-create $*" >> "$FIX/calls.log"
    python3 - "$FIX/issues.json" "$FIX/next" "$*" <<'PY'
import json, re, sys
_, isf, nextf, args = sys.argv
bodyf = re.search(r'--body-file (\S+)', args).group(1)
title = re.search(r'--title (.*?)( --|$)', args).group(1)
labels = re.findall(r'--label (\S+)', args)
n = int(open(nextf).read())
issues = json.load(open(isf))
issues.append({"number": n, "state": "open", "title": title,
               "body": open(bodyf).read(), "milestone": None,
               "labels": [{"name": l} for l in labels],
               "user": {"login": "bot"}})
json.dump(issues, open(isf, "w"))
open(nextf, "w").write(str(n + 1))
if "--jq" in args:
    print(n)
else:
    print(json.dumps({"number": n}))
PY
    exit 0
fi
if [[ "$op" == "issue" && "$2" == "view" ]]; then
    python3 - "$FIX/issues.json" "$3" "$*" <<'PY'
import json, sys
issues = json.load(open(sys.argv[1]))
num = int(sys.argv[2])
args = sys.argv[3]
it = next(i for i in issues if i["number"] == num)
if "--json body" in args:
    if "--jq" in args:
        print(it.get("body", ""), end="")
    else:
        print(json.dumps({"body": it.get("body", "")}))
elif "--json labels" in args:
    if "--jq" in args:
        print("\n".join(l.get("name", "") for l in it.get("labels", [])))
    else:
        print(json.dumps({"labels": it.get("labels", [])}))
else:
    print(json.dumps(it))
PY
    exit 0
fi
if [[ "$op" == "issue" && "$2" == "edit" ]]; then
    echo "MUT issue-edit $*" >> "$FIX/calls.log"
    python3 - "$FIX/issues.json" "$3" "$*" "$FIX" <<'PY'
import json, re, sys
isf, num, args, fix = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
issues = json.load(open(isf))
it = next(i for i in issues if i["number"] == num)
m = re.search(r'--body-file (\S+)', args)
if m:
    body = open(m.group(1)).read()
    it["body"] = body
    open(fix + "/last-body.md", "w").write(body)
for lab in re.findall(r'--add-label (\S+)', args):
    if lab not in [l["name"] for l in it.get("labels", [])]:
        it.setdefault("labels", []).append({"name": lab})
json.dump(issues, open(isf, "w"))
print("{}")
PY
    exit 0
fi
echo "unexpected gh $*" >&2
exit 2
SH
    chmod +x "$dir/bin/gh"
    [[ -x "$dir/bin/gh" ]] || { echo "FAIL: fake gh not executable" >&2; exit 1; }
}

seed_fixture() {
    # $1 = FIX dir. Base: P01 historical + P02-T01 existing (stale managed,
    # human note, missing priority label) + P02-T02 closed + P02-T03 x2 dup.
    local fix="$1"
    python3 - "$fix/issues.json" <<'PY'
import json, sys
issues = [
    {"number": 1, "state": "closed", "title": "P01-T01 — Prerequisites",
     "body": "<!-- fuel-task: P01-T01 -->\nold foundation record",
     "labels": [{"name": "phase:P01"}], "user": {"login": "human"},
     "milestone": {"number": 1, "title": "P01"}},
    {"number": 11, "state": "open", "title": "P02-T01 — Shared ANP fixtures",
     "body": ("<!-- fuel-task: P02-T01 -->\n<!-- anpfuel-managed:begin -->\n"
              "STALE managed content\n<!-- anpfuel-managed:end -->\n"
              "## Human notes\nReviewer note: keep me"),
     "labels": [{"name": "phase:P02"}, {"name": "type:task"}],
     "user": {"login": "human"}},
    {"number": 12, "state": "closed", "title": "P02-T02 — something",
     "body": "<!-- fuel-task: P02-T02 -->\nshipped long ago",
     "labels": [{"name": "phase:P02"}], "user": {"login": "human"}},
    {"number": 13, "state": "open", "title": "P02-T03 dup A",
     "body": "<!-- fuel-task: P02-T03 -->", "labels": [],
     "user": {"login": "human"}},
    {"number": 14, "state": "open", "title": "P02-T03 dup B",
     "body": "<!-- fuel-task: P02-T03 -->", "labels": [],
     "user": {"login": "human"}},
]
json.dump(issues, open(sys.argv[1], "w"))
PY
    echo '[{"number": 9, "title": "P02 — Official catalog and ANP ingestion"}]' > "$fix/milestones.json"
    echo 100 > "$fix/next"
    : > "$fix/calls.log"
}

run_adapter() {
    # Runs issues.sh with the fake gh first on PATH.
    export PATH="$FIXBIN:$PATH"
    hash -r 2>/dev/null || true
    bash "$ISSUES" "$@"
}

echo "== preview lists exact real/missing without writes =="
FIX="$(mktemp -d)"
seed_fixture "$FIX"
export FIX FIXBIN="$FIX/bin" ANPFUEL_TRUST_OVERRIDE=1 ISSUES_PER_PAGE=2
write_stub "$FIX"
OUT="$(run_adapter preview --phase 02 --ledger "$FIX/ledger.json")"
echo "$OUT" | grep -q "TASK=P02-T01 ACTION=reuse NUMBER=11 STATE=open" && ok "preview reuses P02-T01" || bad "preview P02-T01"
echo "$OUT" | grep -q "TASK=P02-T02 ACTION=reuse NUMBER=12 STATE=closed" && ok "preview reuses closed P02-T02" || bad "preview P02-T02"
echo "$OUT" | grep -q "TASK=P02-T03 ACTION=duplicate-refuse" && ok "preview flags duplicate" || bad "preview duplicate"
if echo "$OUT" | grep -q "ACTION=create"; then ok "preview lists missing later tasks"; else bad "preview missing tasks"; fi
if grep -q "^MUT" "$FIX/calls.log"; then bad "preview wrote mutations"; else ok "preview zero writes"; fi
# Pagination honored: >2 issues forces page 2 fetch.
if grep -c "API.*issues?.*page=2" "$FIX/calls.log" | grep -q "[1-9]"; then ok "pagination fetches page 2"; else bad "pagination single page"; fi

echo "== sync refuses duplicates without creating them =="
if run_adapter sync --phase 02 --ledger "$FIX/ledger.json" >/dev/null 2>&1; then
    bad "duplicate sync accepted"
else
    ok "duplicate sync refuses"
fi
if grep -q "MUT issue-create.*P02-T03" "$FIX/calls.log"; then bad "created duplicated task"; else ok "no create for duplicate"; fi

echo "== human notes preserved, closed reused, P01 untouched =="
FIX2="$(mktemp -d)"
seed_fixture "$FIX2"
# Remove duplicates so sync can complete: keep #13, drop #14.
python3 - "$FIX2/issues.json" <<'PY'
import json, sys
issues = [i for i in json.load(open(sys.argv[1])) if i["number"] != 14]
json.dump(issues, open(sys.argv[1], "w"))
PY
export FIX="$FIX2" FIXBIN="$FIX2/bin"
write_stub "$FIX2"
export PATH="$FIX2/bin:$PATH"
hash -r 2>/dev/null || true
if bash "$ISSUES" sync --phase 02 --ledger "$FIX2/ledger.json" >/tmp/sync2.log 2>&1; then
    ok "clean sync succeeds"
else
    bad "clean sync failed: $(tail -n 3 /tmp/sync2.log)"
fi
if python3 - "$FIX2/issues.json" <<'PY'; then
import json, sys
issues = json.load(open(sys.argv[1]))
it = next(i for i in issues if i["number"] == 11)
assert "Reviewer note: keep me" in it["body"], "note lost in stored body"
assert "anpfuel-managed:begin" in it["body"], "managed block missing"
print("stored body ok")
PY
    ok "human note preserved in stored body"
else
    bad "stored body lost note"
fi
grep -q "anpfuel-managed:begin" "$FIX2/last-body.md" && ok "managed block refreshed" || bad "managed block missing"
if grep -q "issue-close\|issue-reopen\|\"closed\"" "$FIX2/calls.log"; then bad "adapter closed/reopened"; else ok "no close/reopen calls"; fi
if grep -Eq "(view|edit) 1([^0-9]|$)" "$FIX2/calls.log"; then bad "touched P01"; else ok "P01 untouched"; fi
if python3 - "$FIX2/ledger.json" <<'PY'; then
import json, sys
d = json.load(open(sys.argv[1]))
assert d["issues"].get("P02-T01") == 11, d
print("ledger ok")
PY
    ok "ledger records reused IDs"
else
    bad "ledger IDs"
fi

echo "== idempotent second sync creates/updates nothing =="
: > "$FIX2/calls.log"
if bash "$ISSUES" sync --phase 02 --ledger "$FIX2/ledger.json" >/tmp/sync3.log 2>&1; then
    ok "second sync succeeds"
else
    bad "second sync failed: $(tail -n 3 /tmp/sync3.log)"
fi
if grep -q "^MUT" "$FIX2/calls.log"; then bad "second sync mutated"; else ok "second sync zero mutations"; fi

echo "== API failure stops, ledger kept, retryable =="
FIX3="$(mktemp -d)"
seed_fixture "$FIX3"
python3 - "$FIX3/issues.json" <<'PY'
import json, sys
issues = [i for i in json.load(open(sys.argv[1])) if i["number"] not in (13, 14)]
json.dump(issues, open(sys.argv[1], "w"))
PY
export FIX="$FIX3" FIXBIN="$FIX3/bin" FAIL_ON="issue edit"
write_stub "$FIX3"
export PATH="$FIX3/bin:$PATH"
hash -r 2>/dev/null || true
if bash "$ISSUES" sync --phase 02 --ledger "$FIX3/ledger.json" >/tmp/sync-fail.log 2>&1; then
    bad "failing sync accepted"
else
    ok "failing sync stops non-zero"
fi
if grep -q "retryable" /tmp/sync-fail.log; then ok "failure reports retryable"; else bad "no retryable report"; fi
if python3 - "$FIX3/ledger.json" <<'PY' 2>/dev/null; then
import json, sys
d = json.load(open(sys.argv[1]))
assert isinstance(d.get("issues"), dict), d
print("ledger present")
PY
    ok "ledger kept with completed IDs"
else
    bad "ledger lost"
fi
unset FAIL_ON

echo "== dry-run writes nothing, ledger untouched =="
FIX4="$(mktemp -d)"
seed_fixture "$FIX4"
export FIX="$FIX4" FIXBIN="$FIX4/bin"
write_stub "$FIX4"
export PATH="$FIX4/bin:$PATH"
hash -r 2>/dev/null || true
echo '{"issues":{}}' > "$FIX4/ledger.json"
BEFORE="$(sha256sum "$FIX4/ledger.json" | cut -d' ' -f1)"
if bash "$ISSUES" sync --phase 02 --dry-run --ledger "$FIX4/ledger.json" >/dev/null 2>&1; then
    ok "dry-run succeeds"
else
    bad "dry-run failed"
fi
if grep -q "^MUT" "$FIX4/calls.log"; then bad "dry-run mutated"; else ok "dry-run zero writes"; fi
AFTER="$(sha256sum "$FIX4/ledger.json" | cut -d' ' -f1)"
[[ "$BEFORE" == "$AFTER" ]] && ok "dry-run ledger untouched" || bad "dry-run touched ledger"

echo "== adapter never closes issues by construction =="
if grep -n "^[^#]*issue close\|^[^#]*issue reopen\|^[^#]*--close" "$REPO/scripts/issues.sh" | grep -q .; then
    bad "close path present in adapter"
else
    ok "no close/reopen path in adapter"
fi

echo "== summary: $PASS passed, $FAIL failed =="
rm -rf "$FIX" "$FIX2" "$FIX3" "$FIX4" /tmp/sync2.log /tmp/sync3.log /tmp/sync-fail.log
unset FIX FIXBIN ANPFUEL_TRUST_OVERRIDE ISSUES_PER_PAGE
[[ "$FAIL" -eq 0 ]]
