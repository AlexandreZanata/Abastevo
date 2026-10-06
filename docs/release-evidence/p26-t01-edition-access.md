# P26-T01 — Bounded INLABS editions and access

Status: LOCAL_DONE on `codex/phase-26-regulatory-discovery`. Task: P26-T01.
Binds B-BR-D03/D10/D11 and BUC-D02. Access decision + guarded fetch +
safe parsing + checkpoints. No live INLABS access exists here: absent
credentials fail explicitly and no live fetch is claimed; fixture
schemas are provisional.

## Behavior

- `directory/adapters/dou` package: `Config` carries operator-owned
  credentials (memory only — never Git/logs/payloads); `FetchEdition`
  refuses `ErrNoAccess` before any network, enforces the exact
  production host (`inlabs.in.gov.br`, loopback solely for tests),
  HTTPS-only outside tests, dial-time IP inspection (no rebinding
  gap), TLS 1.2+, per-edition byte caps.
- `ParseEdition`: rejects `<!DOCTYPE`/`<!ENTITY` before decoding,
  enforces the byte cap pre-allocation, requires edition date + act
  ids, checksums the edition identity. Unknown wording stays raw text
  for the T02 classifier — never guessed.
- `MemoryCheckpoints`: checksum-seen/mark convergence for duplicate
  editions (durable checkpoints arrive with scheduled wiring).
- Access decision (frozen): INLABS needs an operator account; no
  credential exists in this environment, none was invented, none is
  stored. Fixture-based tests never touch the network except
  loopback httptest servers.

## Validation

- RED→GREEN: suite failed to build before `dou.go` existed; GREEN 7/7
  after (`-race`): access denial pre-network, off-allowlist refusal,
  index parsing (2 acts + checksum), XXE/oversize/dateless rejection,
  checkpoint dedup, loopback success (basic-auth + path + parse),
  timeout.
- `go vet` clean; `gofmt` clean.
- `git diff --check` PASS; `scan-secrets.sh` PASS (test password is
  the literal `"p"` against loopback only; no real credentials).

## Limits and next

- BLOCKED_ACCESS: live edition fetch unverified (no INLABS account);
  fixture schema provisional — reconfirm at the owning task's opening.
- Next: P26-T02 ANP acts and chronological reconciliation.
