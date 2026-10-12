# RVA07.C / RVA08 staging reconciliation — 2026-10-12

Status: IN_PROGRESS. Origin: https://teste.abastevo.com.br.
Scope: only Abastevo resources in namespace `abastevo-temp`, with owned
artifacts under `/opt/abastevo-temp/owner-consent-20261012`. No shared edge,
other workload/image/database, global cleanup or capacity claim is authorized.
Candidate source: `ffdae9f19c7d5afd43d190f73ec011007fb35192`.

## Behavior and acceptance before deployment

- B-BR-RVA08-07: the staging API and worker use the same immutable tested
  revision, schema through 000073 and actual bundled terms digest. Full terms
  are anonymous/no-store; owner reads and decisions require valid signatures.
  Synthetic grant/revoke/replay/stale/foreign-owner/forged checks never change
  the real phone owner's decision. Training collection remains OFF.
- B-BR-RVA07-13: preserve the Abastevo database before additive migration;
  restore and rehearse the exact migration in a uniquely owned database.
  Never overlap old unfenced workers with the upgraded schema/new workers.
  Replace ephemeral media only after confirming absence of stored photos,
  preserving existing storage metadata and blocking public traffic for cutover.
  Keep all deployment resource limits, with one API/worker replica and bounded
  migration/acceptance jobs. New intake stays zero until measured admission.
- BUC: old public terms 404 -> rehearse restore/upgrade/replay -> controlled
  maintenance -> schema upgrade -> API/worker health -> exact public terms
  hash -> real HTTPS signed synthetic owner decisions and negative checks.
  Verify autonomous cleanup health, private storage, resource use and no OOM.
  A real elapsed retention canary cannot pass before its actual time window;
  provider/backup/device limits remain explicit, never inferred from clocks.

## Rollback and preservation

Save the original Deployment/Service manifests and a mode-0600 database dump
outside Git. Verify the dump by restoring into the named owned rehearsal DB.
If rehearsal fails, leave the live deployment/schema unchanged. During cutover,
remove only the Abastevo Service selector temporarily, stop the old deployment,
apply the rehearsed append-only migrations, then start the new API/worker with
preserved private media. A binary failure can restore the original deployment
manifest after stopping new workers; do not run down migrations or overwrite
live database data. Any new-schema incompatibility keeps traffic closed until
recovery. Backup is private operational data, never a Git artifact or evidence
that every historical operational photo has disappeared from provider backups.

## Initial read-only observations

Live schema is 000053. API/worker host-mounted binaries identify revision
13c7c25. New terms route returned 404. API/worker limits are 1 CPU/1 GiB and
0.5 CPU/512 MiB; private MinIO 0.5 CPU/1 GiB; PostgreSQL 1 CPU/1 GiB.
Observed memory: API 7 MiB, worker 3 MiB, storage 234 MiB, PostgreSQL 180 MiB.
Private bucket lists zero objects and is unversioned with one-day lifecycle;
that lifecycle is not a strict 24h deletion certificate. Existing database
retains 10 evidence sessions / seven object receipts and 28 observations.
Media resides in a pod emptyDir; preserve its metadata before pod replacement.
Host reserve observation: about 24 GiB available memory and 235 GiB free disk.
No actual migration/deployment or signed live acceptance yet at this opening.

## Bounded operational retention fixture opening

B-BR-RVA07-14: create exactly five uniquely owned synthetic sessions, ten
private transport objects of at most 3 MiB each (30 MiB aggregate), sequential
PUTs, one DB connection, five-second HTTP calls and a 90-second seeding bound.
No account, GPS, community observation, real photo or training inventory is
created. Persist reservation before source PUT and nonce inventory before final
PUT. Four rejected/abandoned final writers exercise actual expiry/recovery; one
READY final retains the real first-receipt anchor until autonomous cleanup at
22h. No clock backdating, manual sweep or manual byte deletion can qualify.
The manifest stays private outside Git; inspection is read-only and checks
actual signed HEAD against per-copy DB absence receipts. A passed transport
fixture does not certify minimization, public admission or device retention.
Operational deadline proof remains pending until the actual wall-time window.

## Maximum transfer failure and correction opening

The HTTPS 3 MiB source PUT returned 413 before reaching API access logging.
Read-only inspection of the owned Abastevo nginx site found `client_max_body_size
2m`. A second, independent source issue limits the private preview proxy to the
generic 1 MiB HTTP cap. No shared nginx reload/change is authorized by the
container-only rule. Prepare a reviewable Abastevo-site-only 3 MiB patch; applying
it requires explicit scope extension. No other nginx site may be edited.

B-BR-RVA07-15: the already authorized 3 MiB photo contract must reach the
private staging proxy when that bounded preview is configured. Permit exactly
that cap only for PUT under the owned q/f bucket paths; all JSON/identity/consent
routes, other methods/buckets and production keep their existing smaller caps.
Known-length and chunked over-limit streams must still refuse. Prove the defect
RED before correction. Seed retention via the pod-local private S3 endpoint
explicitly if the external edge remains blocked; that is not a passed public
maximum-transfer result. Never raise global request limits as a shortcut.

## Accepted staging checkpoint (before the HTTP-cap follow-up)

The private 19,790,982-byte database dump was restored into the uniquely owned
rehearsal database. Migrations 000054–000073 applied, then replayed as a no-op;
all 10 sessions, seven object receipts and 28 observations were preserved. The
rehearsal database was subsequently dropped; live data and private backup remain.
The old deployment was stopped before live migration. Only the Abastevo Service
selector was put into maintenance and restored. API/worker source ffdae9f and
private media metadata were deployed; media uses a persistent owned host path.
No shared edge, other app/image/database or global cleanup was changed.

Live terms now return 200/no-store, exact pt-BR text and SHA-256
`4a9763192620d267311f49fd43c9f4502144fa3bf78edc04b4112982e87bf8ce`,
with collection unavailable. All 14 real HTTPS consent checks passed in 29
requests: two synthetic owners, default OFF, signed ON/OFF, other-owner OFF,
unsigned read/write 401, forged signature 401 without mutation, idempotent OFF,
stale ON 409, wrong digest 400 and final durable OFF. The original harness
finally advanced its synthetic OFF to revision three; no real account decision
changed and training derivative count stayed zero. Subsequent harness code
preserves failed cleanup and avoids inventing collection availability on failure.

Autonomous worker observation was healthy and refreshed each minute. After the
retention seed at 2026-10-12T22:09:34Z, all five originals were autonomously
deleted by 22:10:09Z. All four abandoned final writers were PURGED and confirmed
absent by the 22:12:24Z inspection. One READY synthetic final remains; its real
cleanup boundary is 2026-10-13T20:09:34Z and deadline 22:09:34Z (18:09 Cuiaba).
No manual sweep/backdate/deletion was used for these ten canary copies. Two
earlier public PUT attempts refused at the external 2 MiB edge; their empty
ISSUED fixtures are not part of the elapsed canary or a successful transfer.
Maximum transfers were seeded explicitly through the pod-local S3 listener.

Read-only recovery inspection agrees with actual provider HEAD and DB absence
receipts. Provider is the existing staging MinIO, private and unversioned; no
R2/commercial provider certificate is claimed. One-day lifecycle is not a strict
24h deadline. Host snapshots/backups/exports, physical Android private files,
elapsed final retention and prolonged stability remain unaccepted. ADB currently
lists no connected device; prior native evidence remains the recorded eight
cases, not a new real staging phone pass.

Observed after recovery: API 7 MiB, worker 7 MiB, storage 228 MiB, PostgreSQL
210 MiB, zero new pod/container restarts and no OOM. Fixed resource limits and
training OFF are preserved; photo admission remains explicitly zero.

## Validation and limits of the follow-up

HTTP cap RED: exact 3 MiB requests hit the generic 1 MiB cap in both the policy
regression and real HTTP stream case (413). GREEN: q/f preview PUT permits
exactly 3 MiB, known-length/chunked excess refuses, unrelated/production routes
retain their original limits. Unit/race/vet and signed-proxy integration pass.
Actual API+worker over disposable local PostGIS/private S3 passes both signed
process flows (20.551s). No dependencies or migrations added by this correction.
The shared-edge proposal passed isolated `nginx -t`; it is not installed.

Two external 5 req/s campaigns stopped during warm-up, with resource guards
healthy. The two-client run exhausted slots; the eight-client run had three
timeouts and one drop. Neither is a capacity/latency pass or a steady-state
benchmark. Low-concurrency direct HTTP checks from the VPS all returned 200
(city median 9.67 ms, national median 40.01 ms; five samples each, not percentiles).
Default Python User-Agent was refused at Cloudflare (403); the explicit existing
Abastevo pilot User-Agent succeeded. Do not weaken or disable WAF for tests.
A bounded VPS-origin HTTPS comparison is independent evidence, not a substitute
for the failed external path. No supported-user count is inferred.
