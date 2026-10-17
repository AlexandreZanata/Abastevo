# FQ-05W4 — Verified file-backed quality job boundary

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-17 on maintained dev, parent d57710c8.

Before behavior: B-BR-FQ-01/02/03/04/05/07/12/13/16, BUC-FQ-01/02/03/04.
Use the existing versioned quality-import envelope, but compose PMQC native work
through a file artifact rather than an archive-sized byte slice. Strictly validate
source/stream/member/edition/URL/hash before fetch; verify a private regular file
against exact fetched and planned hash/bytes before and after preparation.
Artifacts are owned/cleaned on success, source/prepare/integrity/publication
failure and cancellation; raw paths/source/process errors never enter job errors.

Require an injected whole-attempt serial lease (acquired before fetch) and a live
per-attempt Directory publication guard. This prevents stale source retries or
cross-worker correction reordering when the production lease is composed. Shared
publication/watermark rules remain with the existing importer. Preserve the
existing injected byte-backed compatibility tests; they do not activate native
runtime intake. Native production fetch/lease/Directory/root/image wiring and
queued-job exclusion still require the remaining FQ-05W composition checkpoint.

Immediate acceptance: malformed/duplicate/unknown payload/source/member before
I/O, wrong/changed hash or byte counts, nonregular/symlink/permissions/oversize
artifacts, missing lease/guard, prepare/cleanup failure and cancellation. Real
PostGIS native-file publish/replay/history/watermark preservation, stale Directory
guard and no partial effects. No new source activation, D or release claim.


## Delivered and verified

FileImport accepts only the strict version1 PMQC assay source selection. Unknown,
duplicate/case-aliased JSON, unknown versions/source/stream, wrong monthly member,
JSON/non-source/traversal URL, query credentials and invalid hashes fail before
ports. FileArtifact owns its path, expected identity and cleanup capability; a
path never comes from the job. Verify SHA256/bytes/private regular-file identity
before/after preparation, bounded100 MiB through streaming hash. No archive-sized
buffer in this path; the byte-backed injected compatibility handler remains.

Whole-attempt Serial and per-batch live Guard ports are mandatory. The actual
cross-worker lease/Directory/native/network factory is still the next root
composition obligation; the file boundary does not pretend those ports are
already deployed. Publication uses one shared existing helper and the immutable
Publisher.WithGuard path. Superseded retries preserve watermarks as before.

RED was the missing file-backed boundary/guard capability. GREEN tests:

- malformed/unknown/ambiguous/oversize payload and source/member before I/O;
- hash, metadata bytes, oversize, relative path, symlink/directory and unsafe
  permissions refused; cancellation and fetch/preparation failures clean owned
  files with redacted errors;
- real PostGIS publication/replay converges to one revision; serial and live
  guard are invoked on every retry; stale guard and changed source cannot
  resurrect a payload or change the existing publication/watermark;
- cleanup failure after a complete committed publication reports an error;
  subsequent retry converges without a second revision. A failed cleanup is
  never reported as successful physical deletion.

Commands from backend/, GOCACHE=/tmp/fq-completion-go-cache:
`go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/jobs`
PASS against disposable local PostGIS (6.676s). All existing discovery/import/
report job regressions retained. `go test -count=1 -race` for jobs/native pure
cases PASS; affected vet/staticcheck and worker/API build PASS. Format,
whitespace, links and tracked/changed/untracked secret/PII surface review PASS.
No schema/dependency change or new aggregate release run.

Next: production file fetch/serial/Directory preparation factory, bounded live
discovery and non-starving enqueue, explicit disabled native claim isolation and
static Rust image. FQ-05W overall, FQ-12–17/final protected delivery remain pending.
D receipt/business portal scope remains NOT_AUTHORIZED.
