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
  Secret bodies are piped to `curl --data-binary @-`, never interpolated
  into argv.
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

## Not done

- A5b: the same script against real staging with B3, after A1–A4 and the
  map PRs are in `main` and deployed; evidence goes in the B3 PR as
  `backend/evidence/admin/A5-STAGING.md`. Key generation and rotation stay
  with the user.
