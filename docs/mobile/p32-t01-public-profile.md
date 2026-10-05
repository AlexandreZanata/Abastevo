# P32-T01 — Public profile, provenance and representation badge

Branch: `codex/phase-32-station-profile-app` (base P31 `1299e9c`). Backend contract: `profile-v1` (`ValidOperatorSource` registry/dou/review, `PublicBusinessKeys` opening_hours/services/phone/website/description).

## Implemented (domain/application/data/app, no Room migration, no network client)

- `domain/profile/StationProfile.kt` — public projection; `projectBusiness` drops CPF/keys/passwords/precise GPS/prices.
- `domain/profile/ProfileBadgeRule.kt` — pure resolver: blank/unknown → Unclaimed; pending/in_review/needs_info → PendingReview; verified + permitted source + fresh → Verified; stale/revoked → StaleUnverified. Representation only, never fuel/price quality.
- `domain/repository/StationProfilePorts.kt` — `StationProfileGateway.getProfile` (null = unknown, never synthetic).
- `application/usecase/profile/GetStationProfileUseCase.kt` — blank → Invalid (no gateway touch); null → Unclaimed; else Found.
- `data/remote/profile/StationProfileDto.kt` — blank id → null; business filtered on decode.
- `app/ui/stations/StationProfileUiMapper.kt` — pt-BR labels (Perfil não reivindicado / Em análise / Representação verificada / Verificação desatualizada), no price-quality claims.

## Validation (RED→GREEN per layer)

- RED: `StationProfileBadgeRuleTest` + usecase/DTO/UI tests failed on unresolved references; data DTO test caught nullable-receiver misuse (fixed in test).
- GREEN: `:domain:test` profile 7/7, `:application:test` profile 3/3, `:data:testDebugUnitTest` profile 2/2, `:app:testDebugUnitTest` StationProfileUiMapper 4/4 — BUILD SUCCESSFUL.
- `:app:assembleDebug` PASS. `git diff --check` PASS. `bash scripts/scan-secrets.sh` PASS.
- No backend change → no migration/PostGIS run; privacy covered by projection-filter tests (cpf/password/gps dropped).

## Limits / next

- Compose section wiring into station detail + navigation/public cache stays P32-T02-owned. Live VPS/device rows UNVERIFIED (Fortinet CA curl 60, no bypass); no PR/CI/merge/wiki per batch closure. iOS archived; G09 UNCERTIFIED.
