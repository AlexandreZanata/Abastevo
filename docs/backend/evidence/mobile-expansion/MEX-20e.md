# MEX-20e — Bounded native enforcement worker

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

State: LOCAL_DONE / INTEGRATION_PENDING. Parent:e10af8bc. B-BR-MEX-10/11/12/16, BUC-MEX-07.
Reuse the owned queue, serial source lease, HTTPS fetch policy, private verified
snapshot runner and guarded publisher. Exactly the pinned listing/resource,
no job paths/hosts/executables. Independent import/discovery flags default OFF;
disabled jobs retain attempts. Raw and prepared cleanup precedes publication.
Recovery refuses unsafe files and leaves the last verified revision visible.
No deployment/source activation or new dependency.

## Executed acceptance

The initial disabled-job regression claimed enforcement work through the general
dispatcher. Two reserved kinds now preserve queued attempts while OFF. Independent
`ANPFUEL_QUALITY_ENFORCEMENT_IMPORTS_ENABLED` (also requires the existing quality
imports switch) and `ANPFUEL_QUALITY_ENFORCEMENT_DISCOVERY_ENABLED` default false.
Discovery inspects one exact approved workbook; dedupe never renews dead budgets.

The existing HTTPS fetch policy/source lease/private snapshot runner are reused;
only the enforcement runner adds strict0600 artifact checks and its two-argument
CLI. Legacy private-workspace contracts stay unchanged. The writer now refuses
200MiB before disk write (boundary/overflow tests); Go uses the same budget and
a120s process/bridge deadline. Source fetch100MiB, listing budget, job4096B,
three attempts, fixed serial slots and five-minute queue leases remain bounded.

Actual Rust→worker→PostGIS discovery/correction/replay, lost watermark ack/retry
without fork, historical reads, cleanup-returning-nil with raw file retained
(refused), serial lease denial and dead-job preservation PASS. Payload/ports,
unknown profile, tamper, cancellation, download/prepare/cleanup failures and
private artifact tests PASS; no raw/error body enters logs.

Affected matrix quality10 packages+worker/config/source PASS after fixing the
new private-file requirement's effect on legacy PMQC: unchanged legacy files
live inside0700 workspaces. Final native+worker race/PostGIS9.6s PASS. Rust170
tests, fmt/clippy, Go vet/staticcheck, diff/secrets PASS. Image4b98d5e23064 built
all Go roles and four native preparers; standalone enforcement ran without
network as65532:65532, exit0, census1/1/0/0,0700/0600 outputs. ELF is static PIE
(no interpreter or NEEDED shared objects); static PIE's DYNAMIC relocation
header is not a shared-library dependency. The exact extracted image preparer
also passed the worker/PostGIS correction/ack/cleanup acceptance.

Logs: /tmp/abastevo-mex20e-{matrix,runtime,rust,image}.log (local only).
Source imports/schedule remain OFF, no VPS/network activation or deployment.
Next MEX-20f: pinned whole-source acceptance.
