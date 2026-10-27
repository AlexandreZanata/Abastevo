# A5a — Staging script + local run against the A4 stack

Date: 2026-10-27. Branch `codex/adm-a5a-staging-script` from A4
(`codex/adm-a4-staff-roles`, `f5004a77`). PR stacked on #168 per plan
section 1a. A5b (real staging run) happens later with B3; nothing here
touches staging.

## Delivered

`scripts/tests/test-admin-staging.sh` (bash, `set +x`, `set -uo pipefail`):

- Reads `ANPFUEL_ADMIN_KEY`, `ANPFUEL_STAGING_URL` and the synthetic
  account (`ANPFUEL_ADMIN_ACCOUNT`, or a throwaway key account it creates)
  from the operator shell. Optional `ANPFUEL_ADMIN_ORIGIN` (defaults to the
  staging URL) and `ANPFUEL_PUBLIC_ORIGIN` (defaults to the landing origin).
- Prints step results only (`PASS: …`); the key, cookie, tokens and CSRF
  token travel in shell variables and piped stdin bodies, never in output.
  Secret bodies are piped to `curl --data-binary @-`. Correction: the
  original revision (`d5162019`) did interpolate the session cookie and
  CSRF token into curl `-H` arguments (visible in `ps`); fixed below.
- Flow: config (disabled-check skipped: routes are mounted) → login →
  whoami → wrong-key 401 → missing-CSRF 403 → account create/login →
  search → grant PRO 1 day → premium status shows the admin tier → revoke →
  status cleared → logout 204 → cookie reuse 401 → contributors CORS check.

## Local run (A4 backend, disposable PostGIS, synthetic data only)

Backend built from the A4 head on a fresh disposable database with all
migrations, one locally generated test admin key (memory only, never the
staging key), `ANPFUEL_ADMIN_ORIGIN=http://127.0.0.1:8081`,
`ANPFUEL_PUBLIC_WEB_ORIGINS=https://abastevo.com.br`:

```text
PASS: config (disabled-check skipped: admin routes are mounted)
PASS: login
PASS: whoami
PASS: wrong-key
PASS: missing-csrf
PASS: account-create
PASS: account-login
PASS: search
PASS: grant
PASS: status-tier
PASS: revoke
PASS: status-cleared
PASS: logout
PASS: reuse-cookie
PASS: contributors-cors
PASS: admin staging validation complete
```

Exit 0. The disposable database was dropped and the server stopped
afterwards; the API log contains no key material.

## Review fixes (2026-10-27)

- Target validation, before any network call (`--check-target` validates
  only): `ANPFUEL_STAGING_URL` must be a bare origin (no userinfo, path,
  query, fragment or percent-encoding). Loopback (`127.0.0.1`, `::1`,
  `localhost`) may use http or https; the only remote host accepted is the
  documented staging origin `https://teste.abastevo.com.br` (ADR-019,
  `docs/backend/RST_VPS_HARDENING.md`) on the default port; `abastevo.com.br`
  and every other `*.abastevo.com.br` name are refused as production.
  Refusals never echo the URL.
- Secrets out of argv: the session cookie and CSRF token are written with
  the `printf` builtin to 0600 files in a private `mktemp -d` directory
  (umask 077) and passed as `curl -H @file`; bodies stay on stdin. An
  EXIT/INT/TERM trap revokes a grant the run created and logs the admin
  session out when still active, then removes the directory.
- Harness `scripts/tests/test-admin-staging-guard.sh` (`make
  test-admin-staging`), no network: 69 checks covering 9 accepted and 39
  rejected targets with zero curl calls; a static check that no curl
  command line references a secret variable (and that it flags 4 planted
  leaks); and a python fake API standing in for curl that runs the full
  flow, a failure after the grant and a SIGTERM mid-request, asserting no
  secret in any curl argv or output, private header files, temp directory
  removed, and grant revoked plus session logged out by the trap. Against
  the pre-fix script the same harness fails 9 checks.
- Real curl 8.5.0 was checked against a loopback echo server to send
  headers read from `-H @file`. The script was not re-run against a live
  API or any remote host.
- Open decision: the remote allowlist is exactly `teste.abastevo.com.br`;
  any other staging name needs a reviewed change to the script.

## Not done

- A5b: the same script against real staging with B3, after A1–A4 and the
  map PRs are in `main` and deployed; evidence goes in the B3 PR as
  `backend/evidence/admin/A5-STAGING.md`. Key generation and rotation stay
  with the user.
