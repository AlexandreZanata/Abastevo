# Expansion and maintained-main reconciliation — 2026-10-22

Status: LOCAL_DONE
Validation: PASS

Parents: maintained dev/main `2480d2cc` and audited expansion `6efcbfc9`.
Normal merge preserves both histories and resumes construction on maintained dev;
the occupied primary checkout and source branch are untouched. No expansion PR,
publication or release claim follows this local checkpoint.

Before behavior (B-BR-MEX-07/19, existing snapshot/cache contracts): preserve
canonical Room v17 and its 16→17 community cache migration byte-for-byte.
The unmerged expansion's prefix index moves forward to v18 with a single
17→18 additive index migration, preserving catalog/ANP/cache/outbox data.
Keep immutable schema17 from main, generate schema18 from merged entities,
register the forward path and retain separate unit/native acceptance for both.
Historical MEX-05 schema17 evidence describes its original isolated checkpoint;
this reconciled allocation supersedes it without rewriting that history.

The Go1.27.3 security/CI/Docker pins now reach expansion via maintained main.
EE-11's repaired 53-vector pure contract and domain coverage audit are retained;
no source approval, provider activation or new dependency is inferred.

Commands/evidence: full combined JVM/build/lint/APKs in
`/tmp/bfp-expansion-reconciliation-jvm.log`; physical dedicated SQLite tests
`CommunityCacheMigration16To17Test` and `CatalogMigration17To18Test` in
`/tmp/bfp-expansion-reconciliation-native.log`; independent prefix query oracle
`python3 scripts/tests/test-mex05-prefix-sqlite.py`; Go race charging/metrics/
ledger-domain/API contracts and root compilation. Test databases only; no app
data clearing. Initial merge-resolution compilation failed due to a duplicated
migration declaration during editing; corrected before this checkpoint.

Combined JVM/build/lint PASS; physical migrations/reopen/index plans 2/2 PASS.
Full JVM domain638/application390/data465(two existing opt-in skips)/app400: zero failures/errors.
