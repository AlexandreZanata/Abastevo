# Community final safety and scoped staging delivery

Status: LOCAL_DONE / INTEGRATION_PENDING

Validation: PASS

Tested source: `96cc0bcc93c890fc9247166fdae440553cc3aeaa`.

Authority: the 2026-10-14 user request authorizes correcting discovered defects, pushing the cumulative batch, merging protected main after valid CI, and publishing only Abastevo resources on the VPS. Other applications, namespaces, images, containers and shared edge configuration are excluded.

## Corrections and contracts

- B-BR-008/016: current-price lookup, station batch, city feed and full-city statistics immediately suppress a hidden, deleted or rejected representative. Recompute drops invisible and future anchors. Authoritative facts and audit history stay intact.
- Community-v2 discards confirmation-only groups after contributor reduction. An author's newer observation cannot leave confirmations of an obsolete price as the winner or a false competing group. Explicit historical v1 replay remains unchanged.
- B-BR-012/BUC-006: a structured report commits its moderation OBSERVATION case and downstream job with the report, or all roll back. Duplicate reports converge on the open case. Reports alone never erase a price. Public tallies annotate both accuracy and social-abuse review without reporter identity.
- Confirmation, report and helpfulness writes lock the observation and re-read decisions after waiting for the lock. Hidden/deleted targets and stale revisions refuse new writes; replay receipts preserve their natural-key conflict semantics. Decisions take the same lock. Removal applies the existing write quota before mutations.
- Event-scoped confirmation/report/erasure/boundary/trust job deduplication preserves exact retries while permitting later recomputation after earlier DONE jobs. Internal projection version lookup stays unfiltered so invisibility never resets monotonic versions.
- Operator INVALIDATE commits audit, guarded case transition, community rejection and recompute intent in one transaction. Downstream target locks precede case locks, matching report order; no closed-case retry trap or report/invalidation deadlock remains.
- Operator BLOCK uses the same atomic audit/status/trust/job path. Its registered worker rebuilds both authored and confirmation-supported keys with bounded keyset continuations; reactivation creates a distinct event and restores eligible support.
- Android rechecks the captured owner after registration before SIGN/GET/POST dispatch. Periodic Home ticks preserve a slow city lookup plus fetch; explicit refresh/source/city changes retain cancellation. Expiry clears displayed community statistics independently of a slow refresh.

## Validation record

RED reproduced orphan consensus groups, hidden feed exposure, hidden anchor recomputation, owner replacement dispatch, periodic fetch starvation, missing report queue/queue failure acceptance, oversized report detail, and quota-free removal. No check was weakened.

Actual disposable PostGIS (`cfr-review-20261014`, tmpfs, synthetic credentials) with race detector:

```sh
source /tmp/cfr-review-env.sh
GOCACHE=/tmp/prm-community-go-cache go test -race -tags=integration \
  ./internal/modules/community/adapters ./internal/modules/community/adapters/read \
  ./internal/modules/moderation/... ./cmd/ops -count=1
GOCACHE=/tmp/prm-community-go-cache go test -race -tags=integration \
  ./cmd/api -run '^TestPublicProcessCommunityActions$' -count=1
```

The first union passed community adapters 29.754s, public reads 18.156s, moderation adapters 10.055s, application/domain/ops; subsequent lock-order and rollback campaign passed 3.335s/4.018s. Actual signed API→private storage→worker→two-fuel contribution→city summary→like/dislike/remove→report/operator queue→anonymous review passed 14.589s. New visibility races and atomic invalidation rollback passed 4.637s; six-reporter moderation convergence is covered. Full-city65-station pagination/four sorts and 1,500-station single-query plans remained green in the read suite. Backend fast/sqlc/secret review passed; final task-specific updates are recorded in delivery metadata.

Android transport 10 and Home 14 initial focused cases passed; data/app JVM, lintDebug, debug APK and native-test APK passed 3m34s. Final expiry regression (15 Home cases), lint/debug/native-test/release build PASS in 4m29s. Final debug APK SHA256 `6994aae2b6248b49ea27c316cb86ffcea3f60d17cf568983ac1f4fd86f0cb5e4`; release unsigned `fe07d396aad9d07a26be1a9d79bcb082216d45eb59ee35b9796fa1c016b60fb3`. Earlier cumulative Android/domain coverage evidence remains in COMMUNITY_HOME_CONSOLIDATION.md; no physical-device execution is inferred.

Final frozen-source integration union PASS: community adapters 34.059s, public reads 19.631s, jobs 1.016s, moderation adapters 12.856s, trust adapters 7.255s, application/domain/ops, staging repair 1.988s. Actual signed API/worker flow PASS 9.851s. `go vet` affected modules, sqlc vet/generate consistency and diff-check PASS. A stale fixture expected one job across six distinct reporters; updated to one job per immutable event while preserving one moderation case. An audit static guard caught a misleading word in a SQL comment; comment corrected and guard retained.

## VPS preparation and guarded rollout

Read-only inventory found API/worker/private-storage in deployment `abastevo-temp-api`, database StatefulSet `abastevo-temp-db`, namespace `abastevo-temp`. API/worker intake budget was0 and live schema74. Existing containers/images and private-media mount are preserved; only Abastevo API/worker binary commands and the explicit bounded intake setting are candidates for change.

An Abastevo-only private backup (21,315,055 bytes) was restored successfully into `abastevo_community_rehearsal_20261014` on the same Abastevo database instance. Backup and original deployment/service JSON stay outside Git, under `/opt/abastevo-temp/community-20261014/private` with restrictive permissions. No private database content or secrets are copied into this record. One existing open report lacks an operator case; repair must preserve it and be recorded before acceptance.

Next: immutable source/CI head, protected dev→main merge, checksum-verified binaries, migration rehearsal/idempotent rerun, fresh pre-cutover backup, Abastevo-only rollout, actual fixed-origin HTTPS synthetic verification and operator invalidation proof. Close intake on any failed health/cleanup/provider check; keep v2-compatible binaries and append-only schema. Do not downgrade outstanding v2 receipts. Device, elapsed retention and full production G09 certification remain separate from this staging delivery. PRM22 stays deferred.
