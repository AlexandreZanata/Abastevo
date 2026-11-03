#!/usr/bin/env bash
# Sourced by test-agent.sh: safety regressions share signed fixture builders.
# shellcheck disable=SC2154
for mode in behind diverged notfound ratelimit fail; do
    CASE="$(new_case "floor-$mode")"
    make_current_old "$CASE"; prepare_landing "$CASE" 1
    CURL_FLOOR_MODE="$mode" expect_failure "deployed floor refuses $mode" run_agent_apply "$CASE"
    assert_absent "floor $mode no cluster call" "$CASE/kubectl.log"
done
CASE="$(new_case rejected-latch)"
make_current_old "$CASE"; prepare_landing "$CASE" 1
KUBECTL_ROLLOUT_FAIL=1 expect_failure "candidate fails once" run_agent_apply "$CASE"
assert_file "rejected latch written" "$CASE/state/checked.json"
BEFORE="$(wc -l < "$CASE/kubectl.log")"
expect_success "rejected candidate skipped" run_agent_apply "$CASE"
assert_eq "skip makes no cluster call" "$(wc -l < "$CASE/kubectl.log")" "$BEFORE"
expect_success "exact rejected SHA recheck" run_agent_apply "$CASE" plan --recheck "$FIXTURE_SHA"
assert_eq "recheck records current" "$(jq -r .sha "$CASE/state/current.json")" "$FIXTURE_SHA"
CASE="$(new_case rollback-hold)"
make_current_old "$CASE"; prepare_landing "$CASE" 1
KUBECTL_ROLLOUT_FAIL=1 KUBECTL_UNDO_FAIL=1 expect_failure "failed undo holds" run_agent_apply "$CASE"
assert_file "hold exists" "$CASE/state/operator-hold.json"
BEFORE="$(wc -l < "$CASE/kubectl.log")"
expect_failure "held agent refuses" run_agent_apply "$CASE"
assert_eq "hold makes no cluster call" "$(wc -l < "$CASE/kubectl.log")" "$BEFORE"
expect_success "explicit release hold" run_agent_apply "$CASE" release-hold
assert_absent "hold released" "$CASE/state/operator-hold.json"

for point in migrate rollout; do
    CASE="$(new_case "crash-$point")"
    make_current_old "$CASE"; prepare_landing "$CASE" 1
    KUBECTL_KILL_POINT="$point" expect_failure "interrupted $point" run_agent_apply "$CASE"
    assert_file "interrupted $point journal persists" "$CASE/state/inflight.json"
    expect_success "resume after $point" run_agent_apply "$CASE"
    assert_eq "resume $point records candidate" "$(jq -r .sha "$CASE/state/current.json")" "$FIXTURE_SHA"
    assert_absent "resume $point closes journal" "$CASE/state/inflight.json"
done
CASE="$(new_case malformed-latch)"
make_current_old "$CASE"; prepare_landing "$CASE" 1
printf '{}\n' > "$CASE/state/checked.json"
expect_failure "malformed latch fails closed" run_agent_apply "$CASE"
assert_absent "malformed latch no cluster action" "$CASE/kubectl.log"

for point in before after; do
    CASE="$(new_case "crash-current-$point")"
    make_current_old "$CASE"; prepare_landing "$CASE" 1
    KILL_CURRENT_COMMIT="$point" expect_failure "killed $point current commit" run_agent_apply "$CASE"
    assert_file "current $point journal persists" "$CASE/state/inflight.json"
    expect_success "resume $point current commit" run_agent_apply "$CASE"
    assert_eq "current $point candidate recorded" "$(jq -r .sha "$CASE/state/current.json")" "$FIXTURE_SHA"
    assert_file "current $point old landing retained" "$CASE/landing/releases/$FIXTURE_OLD_SHA/index.html"
    assert_absent "current $point journal closed" "$CASE/state/inflight.json"
done
CASE="$(new_case post-switch-fail)"
make_current_old "$CASE"; prepare_landing "$CASE" 1
SMOKE_MODE=after-landing expect_failure "post-switch smoke failure" run_agent_apply "$CASE"
assert_eq "post-switch restores previous landing" "$(readlink "$CASE/landing/current")" "releases/$FIXTURE_OLD_SHA"
assert_eq "post-switch current unchanged" "$(jq -r .sha "$CASE/state/current.json")" "$FIXTURE_OLD_SHA"
CASE="$(new_case landing-restore-hold)"
make_current_old "$CASE"; prepare_landing "$CASE" 1
SMOKE_MODE=after-landing LANDING_ROLLBACK_FAIL=1 expect_failure "landing restore failure" run_agent_apply "$CASE"
assert_file "landing restore failure holds" "$CASE/state/operator-hold.json"
