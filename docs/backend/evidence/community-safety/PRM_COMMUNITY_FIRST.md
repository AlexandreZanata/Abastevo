# Community-first source construction

Status: IN_PROGRESS

Opening: isolated codex/community-first from origin/dev=origin/main 68483db9. Premium predecessor commits already integrated. Primary detached e22824db and its unrelated dirty evidence adapter preserved. Scoped planning commit cherry-picked as 8e52aa2e, retaining current PROGRESS history. User authorized source implementation and GitHub PR to main; no merge or VPS activation requested.

## PRM15-T01

LOCAL_DONE / INTEGRATION_PENDING: ADR-021 and active contract supersession before behavior. Existing 150m evidence preserved; normal receipt admission, optional observation claims, nearby geometry and price confidence audited separately. Documentation links/whitespace checked. Runtime unchanged at this checkpoint.

## PRM15-T02

LOCAL_DONE / INTEGRATION_PENDING: COMMUNITY_FIRST_POLICY freezes v2/legacy/debug/unknown receipt behavior, server-first rollout, incompatible-client recovery and rollback. Original deadlines and owner/key/station/origin/evidence bindings retained. Documentation diff check PASS; implementation tests belong to PRM18/PRM19.

## PRM15-T03

LOCAL_DONE / INTEGRATION_PENDING: existing consensus thresholds retained under a new version without proximity prerequisite; existing helpfulness/moderator authority/budgets retained. Frozen reviewed-coverage denominator/numerator, strict >90% and owner-reviewed product evidence; PRM22 DEFERRED. No new numeric vote/adoption threshold invented. Scoped document/diff review PASS.

## PRM16-T01

LOCAL_DONE / INTEGRATION_PENDING: new registry wire regression proves eight valid nominal-address rows survive with null latitude/longitude and unknown quality; no coordinate columns required. `CARGO_TARGET_DIR=/tmp/prm-community-rust cargo test --locked --manifest-path tools/station-prep/Cargo.toml community_first_registry`: 1 passed. Existing parser already supports this contract; no speculative parser rewrite.

## PRM16-T02

LOCAL_DONE / INTEGRATION_PENDING: rejected optional PMQC coordinates cannot remove registry identity/address or manufacture points; explicit join regression preserves all eight serialized registry facts. Existing quarantine and deterministic join contract retained. Rust join suite 8/8 PASS; locked prepare_registry build PASS.

## PRM16-T03

LOCAL_DONE / INTEGRATION_PENDING: actual Rust prepare_registry→Go loader/reconciliation/HTTP publication, replay, refresh and overlap cases passed in directory/adapters real disposable-PostGIS suite (25.095s). Prepared refresh preserves UUIDs and reviewed points, absent points stay unknown; no consumer schema/parser change required. `CARGO_TARGET_DIR=/tmp/prm-community-rust bash scripts/check-rust-fast.sh`: fmt/clippy/locked tests PASS. Broader Directory subpackage campaign recorded below.

## PRM17-T01

LOCAL_DONE / INTEGRATION_PENDING: new real-PostGIS regression finds two homonymous address-only establishments, preserves their distinct UUIDs and addresses through stable pagination/detail, and keeps coordinates null/quality unknown. `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/read -run '^(TestCommunityFirst|TestSearch|TestHandoff)'` PASS (9.830s). Canonical schema/reads already support optional points; no migration required.

## PRM17-T02

LOCAL_DONE / INTEGRATION_PENDING: existing bounded parametrized/literal-escaped name search reused; documented case/accents, page bounds, stable cursor and address disambiguation. Full real-PostGIS Directory campaign `go test -race -count=1 -tags=integration ./internal/modules/directory/adapters/...` PASS (nine package summaries in /tmp/prm-directory-tests.log), including constant-round-trip/failure recovery/query plan/municipal/HTTP/cursor/source trust cases. Scoped read regression PASS above.

## PRM17-T03

LOCAL_DONE / INTEGRATION_PENDING: extended real prepared publication→anonymous HTTP name search→profile verifies nominal address and explicit null coordinates; actual canonical IDs preserved. Targeted TestPreparedPublicationThroughHTTPAndProfiles race/PostGIS PASS (2.042s). Broader Directory nine-package race/PostGIS campaign PASS. Ready for normal capture consumer; no live deployment claimed.

## PRM18-T01

LOCAL_DONE / INTEGRATION_PENDING: RED no-GPS regression failed with capture-ineligible before change. GREEN normal capture issues photo-capture-community-v2 from mandatory active canonical-station port, without geometry; optional claims still verified against forgery. HTTP accepts absent location, rejects malformed UUID/body; append-only 000091 admits v2 while retaining v1/debug. Camera/24h deadlines unchanged. Application/HTTP race suites PASS (1.031s/1.030s); capture real-PostGIS ownership/replay/concurrency/address-only/legacy/unknown-policy/inactive-station/migration-replay campaign PASS (4.233s). Fixture follow-up correctly restored hidden owner/key fields when seeding legacy receipts; no production rule weakened.
