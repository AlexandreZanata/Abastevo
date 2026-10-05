# P25-T04 — Canonical reconciliation and zero-price public reads

Status: LOCAL_DONE on `codex/phase-25-national-registry`. Task: P25-T04.
Binds B-BR-D01–D06/D12 and BUC-D01/D05/D06. Publishes verified catalog
identities from complete runs only; no price invention, no
address-only merge, no closure by missing rows.

## Behavior (TDD, critical-first)

- Migration `000032_registry_assertion_coords.sql` (append-only):
  nullable latitude/longitude/CRS on staged assertions. CSV rows stay
  NULL (honest unknown); API reviewed points stage explicitly.
- sqlc: `ListRegistryAssertions`, `SetAssertionStation` (NULL-guarded),
  `UpdateStationStatus` with anti-reactivation guard (suspended/revoked
  never cleared back to `active` by a snapshot; reactivation is
  audited-only). `sqlc vet` + `generate` clean.
- `registry/reconcile.go` `ReconcileRun`: refuses incomplete runs;
  resolves each asserted CNPJ to its stable UUID (same CNPJ converges,
  distinct CNPJs never merge — succession stays an audited decision);
  links `assertion.station_id`; projects explicit status
  (unknown evidence never overwrites); records + projects reviewed
  points through the existing revision chain. Row failures skip
  honestly and surface (report + error, never hidden green). Only
  asserted CNPJs are touched.
- `adapters/canonicalize.go` `RegistryCanonicalizer` implements the
  port over the canonical repository (compile-time asserted).
- Reads: anonymous list/detail needed no change (no price joins by
  construction) — proven by integration below, not by assertion.

## Validation

- Unit 4/4 PASS (`-race`, fake store+canon): one identity per CNPJ
  across CSV+API, reviewed point + active status projected, revocation
  sticks against later active snapshots with only asserted CNPJs
  touched, incomplete runs refused, row failures counted + surfaced.
- Integration (real PostGIS, `-race`): stage→reconcile→stable UUIDs,
  zero-price station searchable + detail-readable anonymously,
  re-reconcile converges on identical UUIDs with exactly 2 active
  identifiers; 4-way concurrent first creation converges on exactly 2
  stations + 2 identifiers. Fresh disposable DBs (migrations incl.
  000031/000032).
- Regression: full `directory/...` unit + integration PASS (8 pkgs
  each); `go vet` clean; `vacuum lint` + `apicontract` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic CNPJs).

## Limits and next

- Status chronology beyond stickiness (DOU corrections, succession)
  belongs to P26/T05 operator review, not auto-resolution.
- Next: P25-T05 registry jobs, outage recovery and phase acceptance.
