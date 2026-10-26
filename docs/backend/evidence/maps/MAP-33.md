# MAP-33 — Protected cumulative delivery

Status: IN_PROGRESS / CI_PENDING (this record is committed before the final check/merge)
Validation: local union PASS; remote checks/reviews and merge pending
Integration: INTEGRATION_PENDING
Gate: DELIVERY_GATE not yet closed

## Source union

- Maintained `dev` (`507d915e`) and `main` (`4eeccfbf`, including PR #153) were merged
  normally into the MAP branch. The MAP engine AAR, patches, source-offer/SBOM and
  `tools/station-prep/src/location_review.rs` are present after the merge.
- This delivery updates the existing open `dev → main` PR **#151**. The already-merged
  PR #150 is not reused; PR #152 (driver suite) is a different front and is untouched.
- The checked head carrying this record is the new `origin/dev` head; no force push,
  admin bypass, `--no-verify` or direct-main push was used.

## Local gate state carried into the PR

- MAP-23 current-head guidance/off-route: phone PASS.
- MAP-24/25/26: prior local checkpoints retained; changed inputs reviewed at the merge.
- MAP-27/28: local real-engine/UI/accessibility PASS, with synthetic charging fixtures
  labelled and no live/national coverage claim.
- MAP-29: local proxy budgets PASS; **PERFORMANCE_GATE PARTIAL** with the owed frame pacing,
  current-head network, PostGIS load/EXPLAIN, API 26, clean ARMv7, physical battery and
  national items.
- MAP-30: EXTERNAL_PENDING; no car adapter/dependency.
- MAP-31: local union PASS (Kotlin 2,466 tests/4 opt-in skips, Rust 38, Go unit/race and real
  disposable PostGIS integration after 132 migrations, lint/release/size/16 KB).
- MAP-32: runbook + local 132-migration/163-table restore rehearsal PASS;
  **STAGING_PENDING** (no operator/SSH/DB access and no staging health contract at the
  probed read-only paths).
- Release candidate: 78,396,538 bytes / 74.76 MiB, SHA-256
  `feadd9f34d60c26b57a2025e96a1b799c04b52c90da9d47c9784a430363536e0`, both ARM ABIs,
  16 KB LOAD alignment and size guard PASS.

## Pending final controls

At record creation the remote PR #151 was open/ready and mergeable, with `Quick verification`,
`fast`, `integration` and `test` in progress on the pre-record head. The final exact-head/base
checks/reviews, the guarded merge and the post-merge ancestry/wiki status must be read and
recorded after they complete. A missing/failed/skipped/cancelled required check blocks the
merge; pending CI is not a merge. If the head changes, all affected checks must be rerun.

Post-merge record to append (not yet true): PR merge method/merge SHA, ancestry proof for the
included head in `main`, retained/synchronized `dev`, and WIKI_PENDING/INTEGRATED separately.
Staging/production/release certification is not part of this delivery.
