# P32-T02 — Free-account claim export-sign-import and private status

Branch: `codex/phase-32-station-profile-app`. Server declaration/claim/review authority stays in P30/P31 backend; the client never rewrites PDFs, collects certificate keys, or certifies powers.

## Implemented

- `domain/profile/ClaimExportImportRule.kt` — `ClaimDeclaration` (version/account/station/operator/scopes/nonce/TTL 900s), `MAX_SIGNED_BYTES` 5 MiB, PDF-only import; decisions: guest/expired/owner-mismatch/file-type/too-large/replay denials, `Accepted(certifiesPowers=false)`.
- `application/usecase/profile/ClaimExportImportUseCases.kt` — `RequestClaimExportUseCase` (guest/invalid denied pre-I/O, else store-issued declaration) and `SubmitSignedClaimUseCase` (nonce replay checked via store, accepted → pending review only) with `ClaimDeclarationStore` port.
- `app/ui/stations/ClaimStatusUiMapper.kt` — private labels: Rascunho privado / Enviado · aguardando análise / Ação necessária / Em análise / Recurso disponível.
- No PDF rewrite, no key collection, no new backend contract, no Room migration; retry is account-bound via one-use nonces.

## Validation (RED→GREEN)

- RED: rule/usecase/mapper tests failed on unresolved references.
- GREEN: domain 7/7 + application 3/3 + app 3/3 — BUILD SUCCESSFUL; `:app:assembleDebug` PASS; `git diff --check` PASS; `scan-secrets.sh` PASS.
- Risk coverage: guest, expired, owner mismatch, image/png type, oversize, replayed nonce, retry-after-accept; existing media/social flows untouched.

## Limits / next

File-picker/document-URI wiring and offline submit/status retry UI stay P32-T03/T04-owned with device rows. Live/device UNVERIFIED; no PR/CI/merge/wiki per batch closure. iOS archived; G09 UNCERTIFIED.
