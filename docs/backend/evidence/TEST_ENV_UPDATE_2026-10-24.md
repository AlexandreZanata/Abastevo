# Test environment update and physical APK installation

Status: DEPLOYED_TO_TEST_ENVIRONMENT
Validation: PARTIAL — available checks passed; electric runtime gaps remain
Source test-harness corrections: LOCAL_DONE / INTEGRATION_PENDING

The user explicitly selected `https://teste.abastevo.com.br` for this update.
The existing `abastevo-temp` Kubernetes namespace was updated; no other workload,
shared edge configuration, paid provider or source-import flag was changed.
Related handoffs: [EE-30](ev-support/EE-30.md), [MEX-38](mobile-expansion/MEX-38.md)
and [EE-22](ev-support/EE-22.md). This is a scoped runtime follow-up, not G09
certification, full electric acceptance or a public release.

## Artifacts and preservation

Deployed API/worker source: `537193367af294167f75555a5f0b7ecb776076b1`, built
from the clean maintained dev worktree using `infra/docker/Dockerfile` and its
pinned builders/runtime. The subsequent local changes affect test setup and
documentation only; they do not change the deployed application source.

- Previous live source annotation: `91ff1f93650839a3a55a7239d14b737b25919d04`.
- Database: 95 migrations before, 120 afterward (`000120`).
- New image tags: `abastevo-api:deploy-53719336`,
  `abastevo-worker:deploy-53719336`, `abastevo-migrate:deploy-53719336`.
- Running API image ID: `sha256:84f856246c5dae2cb3253a910f40900e01566b282c3adf1bc9b8f0cf4a85c9a1`.
- Running worker image ID: `sha256:df6472ee3b30304b2b3aada8552260cbc523d01fe3e1966696025a736bdb76ae`.
- Existing private-storage image, host storage, service, secret references,
  container resources and runtime safety settings were preserved.

Private backups and configuration snapshots remain under
`/opt/abastevo-temp/update-20261023-53719336/private` on the VPS, outside Git.
The initial custom-format PostgreSQL dump was restored into the operation's
isolated rehearsal database; the real candidate migrator applied 000096–000120
successfully. All constraints were validated.

For live cutover, only the Abastevo deployment and its existing registry schedule
were paused. A fresh quiesced database dump and media archive were taken before
the migration. Exact row fingerprints, including duplicate multiplicity and the
original column projection, matched across all 105 existing non-migration tables
before and after migration. The migration ledger is the separately verified
106th original table. No original data differences were found.

- Quiesced dump: 28,932,585 bytes;
  SHA-256 `f4de86f4685a27d774491c0a010f607edb5f0b4ec27f0d1e77fe416a49fc39da`.
- Quiesced media archive: 81,920 bytes;
  SHA-256 `73453347205ee40acf84a3be93f34e0c51b3bf1ee12986a4e646ac6a7a843f1e`.
- API, worker and private-storage became ready with zero restarts; both public
  health endpoints returned HTTP 200. The original unsuspended registry schedule
  was restored. Only the operation-owned rehearsal database and the two completed
  migration jobs were removed afterward; backup files were retained.

After traffic resumes, prefer forward repair or restoration of a fresh schema-120
backup into an isolated target with the compatible binary. The old binary rejects
unknown migrations: changing only its image is not a valid rollback to schema 95.
Never overwrite newer writes with the retained pre-cutover dump.

## Backend verification

The full local backend matrix used unique disposable tmpfs PostGIS/private S3
projects, never the live database. Local disk was limited, so private copies of
the runner used the maintained checkout and its existing Go/Rust caches; test
assertions were retained. Commands and raw reports remain in
`/tmp/abastevo-update-backend-report`, with a reconciled summary identifying the
packages rerun after prerequisite repairs.

- Go race unit matrix: 2,414 passing test outcomes, two opt-in skips.
- Integrated matrix after replacing affected package results with their repaired
  runs: 3,352 passing outcomes, six opt-in skips, no failed packages. This matrix
  includes ordinary unit cases as well as integration cases; counts are not a
  count of distinct end-to-end journeys.
- Initial failures exposed a missing enforcement CLI in the runner. The runner
  now builds/exports it. Charging read/moderation test setup now uses the explicit
  disposable DSN, and unavailable required databases fail instead of skipping.
- Initial restore failure correctly refused erasure without storage. The harness
  now asserts that refusal, then configures the private disposable S3 fixture;
  restore 10/10, backup 10/10, deploy 9/9 and infrastructure 7/7 passed.
- Go vet, staticcheck under Go 1.27.1, sqlc vet, vulnerability and static secret
  review passed. `bash -n` and `git diff --check` passed on the corrections.
- Local load: 76,948 requests, zero unexpected 5xx, p95 2.6 ms; seven bounded
  database/storage/disk fault checks passed. These are local measurements, not a
  production capacity or combined mobile concurrency claim.
- Explicit `-tags='integration ee27' -run '^TestEE27RustToPublicHTTPChain$'` passed
  with the real admitted `/tmp/ee04-out` artifact and disposable PostGIS: 1,298
  places, 2,089 connection groups, complete public page traversal. Real OCM/PBE
  parity and storage cases also passed in the repaired integration run.

The six integrated skips remain named obligations: the opt-in whole enforcement
worker/source oracle, long evidence soak, vector generation, physical Android
consent interoperability and live PMQC methodology checks. Earlier matching-source
evidence is preserved; these were not represented as freshly executed here.

Against the deployed HTTPS origin, public health, station search/detail, fuel
prices and malformed-filter refusals passed. The existing `staging-consent`
binary passed 16 checks in 31 requests: published terms/collection OFF, signed
grant/revoke, owner isolation, forged-signature rejection, replay and stale/wrong
terms refusal. Collection remained unavailable, as configured.

A separate account/session probe passed 30 checks in 32 requests: free key account
creation/login, cloud default-OFF state, isolated empty vehicle/favorite/preference
and refuel/expense reads, free premium status/provider list, mixed-token and
caller-owner injection refusal, unsigned writes refused, refresh rotation and old
token refusal. Responses stayed no-store and never became 304; cloud/ledger
omitted ETags. Account replies may carry an ETag under their existing contract.
Synthetic account deletion requests were acknowledged and their worker erasures
were observed separately; no real account was used.

Transport/probe errors were retained as diagnostics, not backend passes. One local
connection reset interrupted a request; the account probe was completed from the
VPS. An earlier probe stopped on an over-strict account ETag assumption, leaving
one empty synthetic account without any session. Its narrowly guarded cleanup
retained the durable erasure request and removed the unused key verifier.

## Runtime gaps confirmed, not accepted

- `POST /v1/charging/observations`, its owner-list path, tariff ranking and public
  observation history return 404: their component handlers exist but are not
  registered by `backend/cmd/api/main.go`.
- `POST /v1/ledger/records` with `dataset=charge` returns 400. The store's read
  and individual-delete selectors still admit only refuel/expense, despite charge
  command/storage support. Refuel and expense reads passed.
- Charging places return an empty page. OCM/PBE coverage returns 503
  `charging.unavailable`: no admitted batches are published in this environment.
  Local source preparation/acceptance did not activate deployment imports.
- Enforcement revisions/actions and sanctions return empty pages; native coverage
  returns `quality.bad-coverage` for the existing source state. New source
  publication, operator contracts and source-import activation remain separate.
- Existing photo-intake budget remains zero; no camera/storage collection gate,
  paid catalog, Play purchase or licensed operator access was activated.

These observations limit the prior component/local acceptance claims. They do
not justify marking the complete electric/community/private journeys ready.

## Physical Android installation and manual handoff

The connected physical phone already had `com.anpfuel.app.debug`. The new debug
APK was built with `:app:assembleDebug --offline`, verified against the installed
signing certificate and installed with `adb install -r`. A pull of the installed
base APK matched the build's SHA-256 exactly:
`67d469e164f2e2281309b99c0fb98f971701c35fe49d5cdcb18ccc4e384ccb22`.
Version remains `3.1.0-debug`, code 4; replacement was confirmed by file hash,
not by assuming the unchanged version string proves installation.

No uninstall, data clear, UI automation or instrumentation was performed.
Android testing is explicitly delegated to the user. Suggested manual checks:
existing profile/vehicles/history survive; online fuel search/detail still work;
create/edit only a test vehicle with its propulsion/battery fields; reopen offline
and verify retained local data; inspect account premium/backup status and cancel a
restore preview without changing real records. Reconnect and verify pending/error
states remain honest. Electric community, charge reads and unpublished source
coverage have the runtime limits above; physical two-device restore, accessibility
and process-death acceptance remain owed.

Next: a bounded source task to repair runtime composition/charge read-delete
coverage with process-level negative/owner tests, then a separately authorized
source-admission/publication step. No remote CI, PR, merge or wiki publication was
performed for these local test-harness corrections.
