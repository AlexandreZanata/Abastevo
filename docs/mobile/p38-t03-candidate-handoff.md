# P38-T03 — Candidate, final integration and G09 handoff

Status: LOCAL_DONE on `codex/phase-38-app-acceptance`. Task: P38-T03.
Records the actual P34–P38 source scope, trust state and launch
blockers. Required final PR/CI/reviews/merge/wiki happen at complete
project batch closure (after the selected catalog/profile phases), not
here; G09 certification remains its own gate afterward.

## Candidate scope (code-ready, INTEGRATION_PENDING)

- Base: P34 checkpoint `6e032d7` → P35 (`48f456b`, `b284ca2`,
  `567f9c0`, exit `c5da7f6`) → P36 (`9efd2d5`, `0ed3d17`, `c04fb0d`,
  exit `fd5cb7a`) → P37 (`8545440`, `75e0c01`, `368597f`, exit
  `72cdc24`) → P38-T01 (`7e274d3`) + P38-T02 (this branch head).
- Deltas: canonical Directory UUID ports/cache/gateway + flag-gated
  server discovery/detail/nearby in Explore; staging origin guard;
  UUID discussion section; session-bound discussion account; contextual
  capture + outbox submit; 19 en/pt-BR strings; 9 evidence docs + 4
  exit records. No backend migration, no new service, no dependency
  added; `com.anpfuel`, MIT notices, KMP ports, expert/offline tools
  and brand preserved.
- Config: staging origin `https://teste.abastevo.com.br` via shared
  `ApiEnvironment.STAGING`; all feature flags default OFF (rollback
  OFF); release target explicit and separately certified — the test
  origin must never ship as production.
- Fixtures: `contracts/testdata/p34/catalog.json` v1 (3 owned
  `[P34-TEST]` stations + 9 exercise cases), NOT SEEDED; population
  explicitly unclaimed.

## Trust and blocker state

- TLS: staging chain unverified from this runner (Fortinet middlebox
  CA `FG6H0FTB23902129`, `curl` 60) — BLOCKED_LIVE, not an outage
  diagnosis; no bypass shipped.
- Providers: email/Google/Apple delivery + client IDs + callbacks OWED
  (no creds in Git, verified).
- Storage: no attached VPS private media — photo journey
  BLOCKED_MEDIA (isolated 24h matrix green).
- Export endpoint: contract frozen, implementation OWED.
- Manual/device: full matrix OWED per P38-T02 (directive: after all
  selected source phases).
- Catalog/profile: P25 → P26 → P27 → P29 → P30 → P31 → P32 → P33 remain
  PLANNED (P28 optional); their affected-device rows join the end batch.

## Handoff

- Next authorized phase: P25 (`codex/phase-25-national-registry` via
  `--from-checkpoint`) after this P38 checkpoint. P25 entry needs the
  reconciled P38 source checkpoint recorded here.
- Final integration (ADR-018) runs once at complete project batch
  closure: acceptance union (incl. owed manual/device batch) →
  cumulative PR → required current Quick verification/reviews →
  guarded merge preserving task commits → owned wiki mirror once.
- G09 then owns real-production TLS/storage/off-host restore/load/
  privacy certification; the staging domain never releases the app and
  never authorizes a public pilot (P10-T09 waits for G09
  RELEASE_CERTIFIED; P11 stays optional; iOS stays archived).

## Validation

- P38-T01 regression sweep green (see its record); T02 matrix planned;
  this handoff is docs-only on top of the tested tree.
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits

- Code-ready is not acceptance; no release, deployment, tag, pilot or
  production claim follows from this checkpoint.
