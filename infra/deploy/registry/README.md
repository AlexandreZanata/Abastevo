# National ANP and monthly OpenCNPJ refresh

This is an offline source pipeline. No OpenCNPJ HTTP service is installed and
no website search API is introduced. The static portal consumes immutable
export files; its state/city filters run against release-addressed JSON.
The Android API and static generator share the canonical station identity.

## Sources and ownership

ANP's complete authorized-reseller CSV is checked daily at 03:17 America/Cuiaba.
IBGE supplies municipality aliases. The monthly RFB/OpenCNPJ source is checked
daily at 01:17 with a two-minute jitter, allowing the previous verified month
to remain available while a new complete month is built. The user explicitly
selected a VPS-local monthly snapshot. This schedule checks availability;
it does not imply that RFB publishes a new edition every day.

The source updater has no DB credentials. A UID-1000 private source directory
contains `owner.txt`, `.opencnpj.lock`, `work/`, `snapshots/`, `current`,
`previous` and the aggregate `source-state.json`. Only an empty root can be
initialized automatically. Never point it at a shared OpenCNPJ installation.
The mutable CLI build workspace is separate from published hash-named files:
a same-month upstream correction cannot overwrite the current snapshot inode.

`station-sync --rfb-update` invokes a pinned OpenCNPJ CLI with source ZIP
verification, two download workers, bounded retries and one workspace edition.
It independently verifies a changed snapshot against its source ZIPs, hashes
and copies it to an immutable file, then atomically changes `current`.
Failure leaves the published pointer and previous DB publication intact.
Source update takes an exclusive nonblocking lock; ANP name preparation takes
a shared lock. Overlap fails safely and retries at the next scheduled run.

OpenCNPJ is an explicitly requested reference dependency, MIT OR Apache-2.0;
use its MIT option and ship its MIT notice with the binary. It is not a new
Go/Rust package dependency or resident service. Record the upstream source
revision, CLI version, binary SHA-256 and snapshot SHA-256 in the private
installation receipt. Source URLs and hashes belong in provenance; do not
copy company records, partners, contact details or credentials into Git/logs.

## Daily admission and publication

The current ANP CNPJ set is exported from the local snapshot in exact full
establishment-CNPJ batches. Leading zeros and branches remain distinct.
Only CNPJ, trade name, active flag and observation date enter the versioned
name input; partner/contact fields are discarded in memory. A deterministic
cache is bound to both complete source hashes. A new ANP establishment or
new monthly snapshot automatically invalidates that cache. Pending jobs reuse
their frozen bytes, including the edition comparison report, after a crash.

The national CSV must be nonempty, bounded and structurally complete with
unique CNPJs. A drop greater than the existing documented 20% source threshold
requires operator review. Rust owns format/check-digit/address/name-policy
admission. The documented quarantine threshold is at most 1%; the manifest's
accepted/duplicate/quarantine accounting must cover its input before any DB
load. Invalid or interrupted editions never advance last success.

`edition-report.json` records only row counts and source hashes: additions,
changed rows, unchanged rows and identities absent from the previous edition.
An absence is not an official closure. Canonical station IDs, coordinates,
prices, historical assertions and accepted community corrections survive.
Operation status changes only through existing official evidence/correction
ports; operators review the missing count rather than bulk-delete stations.
The owned loader publishes and verifies idempotently; unrelated curated facts
are not overwritten. A name-only metadata change invalidates the enterprise
export fingerprint and therefore regenerates the static portal.

## Installation and resources

Install tested immutable tools (`station-sync`, `station-load`,
`prepare_registry`, `opencnpj`, license notice and checksums) under
`/opt/abastevo-temp/registry-sync/releases/REVISION`. Never replace a running
release directory. Render `__REVISION__` in the service with that exact tested
revision and verify the unit before enabling the timer. Provision a mode-700
`/opt/abastevo-temp/opencnpj` owned by UID/GID 1000. An initial verified snapshot
may be bootstrapped there with the same independent copy/hash/source receipt
rules; an incomplete upload must not create `current`.

The real full-national offline export peaked at 316 MiB RSS before container
overhead. Daily registry limits are therefore 512 MiB and 500m CPU, retaining
the existing 18-minute process/20-minute Job deadline and namespace quota.
Do not retain the older 256 MiB limit for the full snapshot adapter.

Set operator-only `REGISTRY_RFB_SOURCE_PATH` to that source root, then render
and server-dry-run the staging manifests. The registry CronJob mounts `/rfb`
read-only and its own state read-write. Missing render configuration refuses
publication. The systemd updater is a short-lived host process because the
existing namespace's total memory quota is 8 GiB; this does not expand it.

OpenCNPJ's repository reports 5.95 GB peak RSS and 18.3 GB temporary disk for
the balanced profile. The updater is limited to one CPU, 8 GiB RAM, no swap,
64 tasks, 110 minutes plus stop grace, and a 64 GiB private tree budget. It
checks the existing shared-host reserve every five seconds: at least 12 GiB
available RAM, 50 GiB free disk, load below 0.65 times CPU count and healthy
staging HTTPS probes. Guard failures cancel only this process and its children.
A guard refusal is a failed refresh, not a reason to relax host limits.

Keep the current and immediately previous immutable source, one CLI workspace
snapshot and the current month's raw ZIP directory. Independent published
copies retain rollback; duplicated older build inputs are retired to reserve
room for the next national build within the 64 GiB source budget.
Only owned hash-named snapshots and old raw directories with the exact official
source manifest can be pruned. Canonical DB history is never retention data.
Minimized caches retain two validated source pairs; frozen daily editions
retain their own inputs independently. Unknown neighboring paths survive.

## Monitoring, recovery and evidence

Inspect `systemctl status abastevo-rfb-refresh.timer` and the oneshot result;
`journalctl -u abastevo-rfb-refresh.service` exposes aggregate stage/count
receipts only. `source-state.json` records the last successful check, monthly
competence, content hash and whether it changed. The registry CronJob/job
status and its `state.json` expose last check, last success, pending edition
and source/name/report/manifest hashes. Review stale checks, failed jobs,
quarantine and missing counts; a running timer is not freshness evidence.
Public exports expose their actual generated edition/time separately.

A failed download/build/verification keeps the previous published source.
Retry the same bounded updater after resolving source or host availability;
never point `current` at `work/`. For a verified source rollback, stop the
oneshot, acquire the exclusive source lock, verify `previous` and its sidecar,
and atomically exchange the published pointers. Retain a private incident
receipt; the next ANP run creates a new hash-bound publication.

A DB load interruption resumes the frozen pending edition; do not edit its
files, CNPJ rows, name descriptor or report. An excessive-quarantine or source-
drop refusal requires inspection of the exact source hashes and preparation
report. Do not weaken tests/thresholds or erase pending state to force a green
job. Correct source/policy defects in a tested release; archive the rejected
private state under the worker lock before an explicitly reviewed new attempt.

Before first live publication verify the deployed backend SHA/schema and a
private backup. Use a bounded, uniquely named guarded Job. Compare station
identity/coordinates/history and prices before/after; verify trade/legal names
through the API, static files and actual Android app. Confirm source timer,
CronJob arguments/mounts, immutable release paths and last-success receipts.
Templates and local tests alone are not runtime activation or certification.
