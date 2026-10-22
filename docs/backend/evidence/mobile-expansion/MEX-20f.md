# MEX-20f — Whole-source enforcement acceptance

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Parent:91d86a05. B-BR-MEX-10/11/12/16, BUC-MEX-07.

Use only the private pinned workbook (16,188,087bytes, SHA256
952743f3fa2b83094ad2011c5a4a1bffc14222eabde1bb4ee3bf4816e2f7db40).
Reconcile its complete independent census through actual Rust→Go→worker→PostGIS,
replay and a private synthetic full-file correction. Traverse every frozen fact,
check every public field against its verified source digest and retain distinct
results. No sampling may stand for full traversal; no raw identifiers/names/
addresses or records in logs, fixtures or Git. This accepts the pipeline/profile
locally, not live source admission, station attribution or production readiness.

## Whole-source result

- Fresh independent Python census and native Rust full-source oracle agree:
 244247 rows,141290 documents,180661 source subject/day/document/procedure
 tuples,495 result labels,116 maximum characters,32822 results beyond64,
 37 segments with identical populations, company242948/personal1294/other5,
 EXCEL_1900 document days2019-01-02..2026-10-04. Native accepted244247,
 duplicates0/quarantined0/metadata0. The source's extraction clock is UNAVAILABLE.
- Actual image preparer→Go→worker/source lease→PostGIS: first publication,
 full native replay (one revision), then a private whole-file synthetic result
 correction (two revisions). Every original public fact/field is SHA-256
 compared on two complete traversals:244247 facts/2443 pages each, including
 frozen superseded history after correction. No source/station association.
- Critical acceptance ran with race:203.7s; initial worker65.9s. The test's
 XML correction writer was then repaired to avoid duplicate xmlns declarations;
 its explicit XML regression and final complete run PASS in54.4s without race
 (initial20.1s). Runtime code/image unchanged; source remains immutable.
 This is local test timing, not production latency/capacity.
- All three worker preparations executed the trusted static image binary;
 after each completed import the owned raw and prepared workspaces are gone.
 Separate privacy audit scans every native artifact record and removes its
 temporary artifacts:401 distinct normalized personal identifiers,40530 full
 source names and45007 trimmed addresses longer than3 characters checked;
 zero personal-ID matches in emitted string fields, zero exact name/address
 copies in procedure/result and zero excluded structured columns. This does
 not claim exhaustive substring de-identification of arbitrary future sources.
- Four synthetic privacy failures, five independent census boundaries, two
 CI-cadence checks, targeted final worker race/PostGIS, vet/staticcheck and
 diff/secret review PASS. Source oracle/privacy regressions are now in backend
 fast CI and their paths trigger it. No new dependency. The broader affected
 matrix, Rust170, sqlc/OpenAPI and image proofs are retained from c/d/e.

## Frozen policy and reproducible commands

Source profile anp-enforcement-xlsx-2019-v1; dataset edition
 enforcement-source-v1 (not a calendar coverage inference). Preserve full
 verbatim minimized facts, separate document day, null legacy event/finality
 fields, unavailable extraction clock,128-row atomic chunks,500000 rows,
100MiB input/200MiB output (writer and bridge),120s runner,4096B job payload,
three attempts, five-minute queue lease and one native source lease.
Both enforcement flags and the existing imports flag stay OFF. No scheduler,
VPS, deploy, tag, live source admission or production/Android certification.

Fresh oracle:
`python3 scripts/audit-anp-source-census.py --enforcement <private-xlsx>
--pmqc <private-pmqc> --out /tmp/abastevo-mex20f-census.json`.
Actual worker proof: set ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH,
ANPFUEL_TEST_ENFORCEMENT_CENSUS_PATH and ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH,
then run from backend `go test -race -tags=integration ./cmd/worker
-run '^TestEnforcementPinnedWholeWorker' -count=1 -timeout=10m`.
Privacy: `python3 scripts/audit-enforcement-privacy.py --workbook <private-xlsx>
--preparer <trusted-image-preparer> --out <aggregate-json>`.
Logs remain outside Git: /tmp/abastevo-mex20f-{full,final,native-oracle}.log;
aggregate census/privacy JSON likewise in /tmp. Test databases are unique and
removed by their owning test, never a shared or production database.

The four authorized units c/d/e/f are locally complete. MEX-20's local pipeline
orchestration gate is satisfied; MEX-21 association is next, not part of this
batch. PR144 is the cumulative b–f integration surface. Remote current-head
checks, protected integration and one wiki mirror remain separate delivery
states. EE-04/05 rights/admission, broad manual, signed staging contribution,
Android/commercial and production obligations remain OWED.
