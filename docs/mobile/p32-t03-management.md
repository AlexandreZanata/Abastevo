# P32-T03 — Representative management, invitations and contested access

Branch: `codex/phase-32-station-profile-app`. Server (P31 manage/review backend) stays authoritative; the client never grants, never approves optimistically, and never replays stale privilege.

## Implemented

- `domain/profile/ManagementCapabilityRule.kt` — `ManagementGrant` (role/scopes/fresh-auth/revoked/suspended/stale-operator) × `ManagementAction` (edit/reply/invite/contest/reverify); denials: revoked/suspended/stale-operator/fresh-auth/scope/invalid; allowed → `AllowedRequiresServer` only. `checkReplyText` enforces the 280-scalar boundary.
- `application/usecase/profile/ManagedActionUseCase.kt` — capability checked pre-I/O; denied never touches gateway; server refusal recovers as `ServerRefused` without replay.
- `app/ui/stations/ManagementActionUiMapper.kt` — pt-BR labels with review limitations (independent review / 280 chars / server decision).

## Validation (RED→GREEN)

- RED: capability/usecase/mapper tests on unresolved references.
- GREEN: domain 7/7 + application 3/3 + app 2/2 — BUILD SUCCESSFUL; `:app:assembleDebug` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Risk coverage: revoked mid-edit, suspended reply, stale operator, invite without fresh-auth, out-of-scope reply, 281-scalar reply, server refusal without replay.

## Limits / next

Full invitation/contest/reverification screens and file-URI/device rows stay P32-T04-owned (offline/accessibility/device acceptance). No backend/Room change. Live/device UNVERIFIED; no PR/CI/merge/wiki per batch closure. iOS archived; G09 UNCERTIFIED.
