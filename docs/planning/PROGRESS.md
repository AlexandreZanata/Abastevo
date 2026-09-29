# Current execution state

- Updated: 2026-09-28, P03-T01 LOCAL_DONE; P02 INTEGRATED.
- Branch: `codex/phase-03-anonymous-identity`, based on `e604e29`; clean.
- Phase P02 INTEGRATED: PR #3 merged `f266799` → `e604e29` (match-head-commit, all checks green after fixing brittle P01-T08 migration-count assertions to chain/once-per-version); branch `codex/phase-02-official-catalog` deleted locally + remotely, verified; merge recorded in PR #3 metadata; wiki WIKI_PENDING. Post-merge main CI runs automatically.
- Main protection ACTIVE: strict, required `[Quick verification]`, PR required, enforce-admins, no force/deletion.
- P03 entry G02 satisfied (canonical identity, exact units/CNPJ, idempotent revisioned import, quarantine/reporting, fixture-based geolocation path pending D05, public official reads; no Android modifications).
- P02-T01…T08: LOCAL_DONE (fixtures, kernel values, directory repository, stdlib parser, allowlisted fetch, revisioned publication, geocoder port, read API). G02 INTEGRATED.
- P03-T01: LOCAL_DONE — `docs/security/identity-profile.md` frozen (P-256/SHA-512, exact covered set/order, raw R||S only, 5-min window, challenge binding, proxy rules; RFC 9421 inspiration explicitly not a compliance claim) + `contracts/testdata/identity/` 12 golden vectors (valid, body, 7 tampers, expired, wrong-key, DER, reorder, all attacker-mutated post-signing) + `modules/identity/profile` dual verifiers (shared-helper path and hand-rebuilt path incl. explicit curve check); ambiguous/reordered/duplicated lines rejected; RED proven by accepting all signatures, GREEN on restore; manifest extended. Cross-library RFC replay stays a later gate per ADR-007.
- P03-T02: LOCAL_DONE — challenge-bound registration, RED→GREEN.
- P03-T03: LOCAL_DONE — `identity/adapters/auth` Verifier rebuilding the base from the live request (configured authority, exact body digest), verifying against server-stored keys, consuming SIGN nonces atomically with fingerprint/purpose/hash binding; duplicate/missing headers, tamper, expiry, revoked/blocked/unknown denied; failed proofs never consume; integration on real PostGIS: happy read/write, tamper matrix, replay once, 8-worker same-nonce once, expired/revoked/blocked/unknown; RED proven by skipping consume, GREEN on restore; manifest extended.
- P03-T04: LOCAL_DONE — idempotency runner, RED→GREEN.
- P03-T05: LOCAL_DONE — quotas, RED→GREEN.
- P03-T06: LOCAL_DONE — `adapters` Rotate with old/new SIGN proofs: server-stored old coords, fingerprint/purpose/nonce binding, single-tx consume-both + revoke + bind, takeover refused, revoked-old only replays same-contributor bindings, no lost-key recovery; integration on real PostGIS: happy path with old-denied/new-works via auth Verifier, old/new proof failures preserving challenges, concurrent exactly-one-wins, idempotent replay, takeover refused with old intact, stranger denied; RED proven by skipping revoke, GREEN on restore; rotate path + schemas + 2 vectors, spec 100/100; no new manifest packages.
- G03: P03-T07…T08 NOT STARTED (P03 scope runs T01…T08; phase PR body to update).
- Next: **P03-T07 — PostgreSQL job queue**.
- Issues/milestone/PR/wiki: P03 phase PR pending; wiki once per merged phase; no invented IDs.
- G09 NOT STARTED; P10 BLOCKED BY G09; roadmap 78 tasks.

## Evidence pointers

- [Prior P01 command outcomes preserved verbatim](history/P01_FOUNDATION_EVIDENCE.md).
- [Android baseline re-validation](BASELINE_VALIDATION.md#p01-t01-re-validation--2026-09-28).
- [Workflow policy](DELIVERY_WORKFLOW.md), [CI plan](CI_PLAN.md), [fast execution card](FAST_EXECUTION.md), [ADR-013](../adr/013-fast-phase-delivery.md).
- [Phase record template](templates/PHASE_RECORD.md): use one compact state and task evidence entries, not a growing full transcript in this file.
