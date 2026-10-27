# MC-B1 correction rules

Status: LOCAL_DONE. Date: 2026-10-27. Base: `f6abff83` plus this task.
Time: ~15 minutes.

## Delivered

`docs/product/STATION_CATALOG.md` now carries **B-BR-COR-01..08** before any
code:

- target and one-field-only corrections against an existing station UUID;
- official/community separation (D8): official facts and canonical identity
  are never overwritten; accepted locations become `reviewed` revisions and
  other accepted fields become labelled community assertions;
- signed free-account proof, 20/day quota, one open correction per
  account+station+field and idempotency semantics;
- per-field validation, Brazil/municipality bounds, coarse proximity bucket
  only and no contributor GPS;
- consensus, auto-verification and reviewed decisions with mandatory reasons;
- the frozen status lifecycle `pending`, `auto_verified`, `accepted`,
  `rejected`, `expired`, `cancelled`, `superseded`, `frozen`;
- privacy/erasure, evidence expiry, text/PII rejection and abuse/audit rules.

## Verification

- Documentation-only change; no runtime or migration.
- `git diff --check` clean.
- The field list in this batch is location, display name, brand, address and
  operating status; dossier and EV fields join after MC-A5/MC-A8.
