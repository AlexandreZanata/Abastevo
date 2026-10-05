# P29-T02 — Source-to-app lifecycle and affected device reacceptance

Status: LOCAL_DONE on `codex/phase-29-catalog-acceptance`. Task: P29-T02.
Changed catalog/intake/social-target flows carry backend
integration proof on the new candidate; G24 evidence carries forward
only for identical inputs; device rows join the end manual batch (no
emulator per directive — no cached-home timing substituted anywhere).

## Lifecycle proof (new, real PostGIS)

- `lifecycle_integration_test.go` (package adapters, `-race`):
  stage CSV → reconcile 2 → suggest same CNPJ → exact-match sweep
  approves and links the SAME station → anonymous search finds it,
  detail reads with a canonical 36-char UUID target, zero price rows
  anywhere, unknown-location station searchable with honest nulls.
- Closed source + offline replay: CANCELADA snapshot reconciles to a
  still-listed station; identical replay converges (same run, 1
  station total, no duplicates).
- Backends: full `directory/account/feedback/moderation/evidence/
  privacy` unit+`-race` suites PASS (only "no test files" notes, zero
  failures); Android `:domain :application :data :app` + assemble
  PASS (89 tasks) on the candidate tree.

## Affected-row mapping (code-ready vs owed)

- Reaccepted in code: UUID discovery/cache/refresh/nearby,
  discussion targets, capture→outbox, suggest/status/cancel,
  Room v8 upgrade guards, intake contract/compat, reconcile/status/
  revocation, review/freshness jobs.
- Owed to the end manual/device batch (P38-T02 matrix, extended by
  catalog rows): denied GPS, photo expiry, low-end encode/memory,
  cold start, TalkBack/large-font, novice intake/update-price
  journeys, provider callbacks, signed-release behavior, v7→v8
  on-device migration. iOS stays deferred.

## Validation

- `git diff --check` PASS; `scan-secrets.sh` PASS.
- No production certification inferred (P09/G09 owns it).

## Limits and next

- Next: P29-T03 pinned catalog candidate and production handoff.
