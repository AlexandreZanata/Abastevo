# MAP-33 — Protected cumulative delivery

Status: INTEGRATED / WIKI_SYNCED (source integration only; staging/production not)
Validation: local union PASS; all required remote checks PASS; protected merge PASS
Integration: INTEGRATED in `main` at `bbd96568`
Gate: DELIVERY_GATE source integration closed; PERFORMANCE_GATE remains PARTIAL

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

## Final controls executed

- PR **#151** (existing `dev → main`; #150 not reused, #152 untouched) was ready and merged
  with merge commit `bbd96568180563ced975b96f8f89f7a3c242522e` at
  `2026-10-26T19:28:55Z`.
- Exact checked head: `04e9a6b3f49c52616ecf04fc1b1e90e7a34c916e`. Required current-head
  checks all succeeded: `Quick verification` 2m51s, `fast` 1m47s, `integration` ~11m52s,
  `test` ~11m56s. No failed/skipped/cancelled required check; no review was requested and
  none was present.
- `git merge-base --is-ancestor 04e9a6b3 origin/main` is true; `main` and `dev` both pointed
  at `bbd96568` immediately after the guarded merge. `dev` was retained and pushed.
- Wiki publication from merged SHA `bbd96568`: commit
  `3c08ac4ccb29e26f695e2399d777528c19da5630`, 778 owned pages, independently verified by a
  fresh clone of `AlexandreZanata/Abastevo.wiki.git`; `wiki-manifest.json` records source
  SHA `bbd96568`.
- This post-merge evidence/progress note is a docs-only follow-up on retained `dev`; it does
  not change the merged application code and does not reopen the merged PR.
- Full MAP-00–33/gate inventory, including the still-PARTIAL MAP-18 native combined artwork:
  [MAP_BATCH_INVENTORY](MAP_BATCH_INVENTORY.md).

Staging/production/release certification is not part of this delivery. MAP-29 remains
PERFORMANCE_GATE PARTIAL, MAP-30 EXTERNAL_PENDING, MAP-32 STAGING_PENDING, and the four
opt-in Kotlin wire tests, two-account/two-device restore, manual TalkBack, API 26, clean ARMv7,
physical 16 KB, frame pacing, physical battery and national capacity remain OWED.
