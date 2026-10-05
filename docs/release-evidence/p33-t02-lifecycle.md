# P33-T02 — End-to-end authority, privacy and device reacceptance (source slice)

Branch: `codex/phase-33-profile-acceptance`. Locks the source lifecycle on the updated candidate (with P33-T01 Busy fix in); provider/device rows stay OWED at conjunto closure per user directive.

## Test added

- `application/lifecycle_test.go` (`TestAuthorityLifecycleTerminalClosed`): open → reissue (version 2, supersede chain) → independent approve (grant) → terminal: cancel past approval fails `ErrClaimClosed`, reissue past approval fails `ErrClaimClosed`, rival status/cancel get not-found (no ownership oracle), rival `ListMine` empty.

## Validation

- New test GREEN first run (behavior already contained; now locked end-to-end).
- `go test -race -count=1 ./internal/modules/stationprofile/...` — 5/5 pkgs ok. `go vet`, `gofmt -l` clean. `git diff --check` PASS. `scan-secrets.sh` PASS.
- Privacy: terminal competition cases stay invisible across accounts (disjoint-IDs proof in P33-T01 + zero rival visibility here); owner-only status/cancel/reissue enforced.
- No production code change (test-only task). No migration/SQL/OpenAPI change.

## Owed rows (P33-T02 manifest)

Real provider trust acceptance, private-storage attach proof, full device/accessibility/performance matrix, operator staffing drill — consolidated in the conjunto-closure manual batch (P38-T02 union). No simulation substituted: recorded OWED, not waived.

## Next

P33-T03 pinned profile candidate + release operating handoff.
