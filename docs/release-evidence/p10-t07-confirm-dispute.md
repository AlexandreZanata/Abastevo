# P10-T07 — Confirm and dispute flows

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, seventh task push). Issue: #51 (slice 7 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change, no
existing screen modified.

## Slice acceptance (frozen before coding)

- T07 (this commit) — Structured corroboration and correction
  (BUC-005, B-BR-005/006/007/011/012): domain `CommunityVotePorts`
  (gateway + receipt + typed `CommunityVoteException` + six wire
  dispute reasons) + `SubmitCommunityVoteUseCase` (flag-gated;
  shown amount/product/unit/condition snapshot required before either
  action; stable `client_submission_id` per intent so identical retries
  converge with `replayed`, already-recorded pairs reject instead of
  amplifying; PRICE_CHANGED accepts a distinct replacement id — the new
  price itself travels via the contribution outbox; detail capped at
  500 chars and never echoed in errors, B-BR-011) + data
  `CommunityVoteFlagStore` (default OFF) + `CommunityVoteHttpClient`
  (`POST /v1/observations/{id}/confirmations|disputes`, fixed error
  messages, `Idempotency-Key` per intent, preview base URL) + app
  `CommunityVoteDisplay` (pure summary/reason/kind labels),
  `CommunityVoteViewModel` (Disabled/Idle/Ready/Submitting/Confirmed/
  Disputed/Rejected, single flight, stable id across retry) and
  `CommunityVotePanel` (summary above both actions, six structured
  reasons, private detail + replacement fields, clear status + retry)
  + 12 new `community_vote_*` strings en + pt-BR. Rollback is flag OFF;
  submitted records stay preserved.

## What changed

- `domain/.../repository/CommunityVotePorts.kt` — vote ports only.
- `application/.../port/CommunityVoteFlagProvider.kt` +
  `application/.../usecase/community/SubmitCommunityVoteUseCase.kt` —
  validation/status mapping, no IO when disabled.
- `data/.../local/preferences/CommunityVoteFlagStore.kt` +
  `data/.../remote/CommunityVoteHttpClient.kt` — flag default OFF,
  network-only client with typed 403/409 mapping.
- `data/.../di/{CommunityModule,RepositoryModule,UseCaseModule}.kt` —
  provider/binds for the new client, flag and use case only.
- `app/.../community/CommunityVote{Display,ViewModel,Panel}.kt` —
  flag-gated UI; ANP + T06 panels untouched.
- `app/.../res/values/strings.xml` + `values-pt-rBR/strings.xml` —
  12 new `community_vote_*` keys each, no existing key touched.
- Tests: 8 application (disabled/self-denial/stable-id replay/
  replacement/distinctness/detail-cap+privacy/blank-ids) + 4 data
  (confirm/dispute POST shape + self/already/ineligible/conflict
  mapping, detail never in errors) + 4 display + 6 viewmodel
  (disabled-no-IO/snapshot/single-flight/self-reject/dispute).

## RED → GREEN

- RED proven by new symbols before implementation: `SubmitCommunityVoteUseCase`,
  `CommunityVoteGateway`, `CommunityVoteFlagProvider` did not exist;
  `SubmitCommunityVoteUseCaseTest` failed compilation with
  `Unresolved reference 'SubmitCommunityVoteUseCase'`. GREEN after adding
  the bounded ports/use case/client/UI only.

## Validation (exact commands, this host)

```sh
./gradlew :application:test --no-daemon
./gradlew :domain:test :data:testDebugUnitTest --no-daemon
./gradlew :app:testDebugUnitTest --no-daemon
./gradlew :app:assembleDebug --no-daemon
./gradlew :app:lintDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:application:test` green (8 new, 8/8 pass);
`:data:testDebugUnitTest` green (4 new, 4/4 pass);
`:app:testDebugUnitTest` green (10 new: 4 display + 6 viewmodel);
`assembleDebug` BUILD SUCCESSFUL; `check-mobile --static-only` ok;
`diff --check`/secrets clean.
Limits: `:app:lintDebug` FAILS with 44 errors — 32 pre-existing/T06
(23 on main + 9 T06 `MissingTranslation`) + 12 new `MissingTranslation`
for the `community_vote_*` keys in de/es/fr/ja/zh/ru (en + pt-BR
complete); zero lint findings in the new Kotlin files. Lint is not part
of required Quick verification (backend/contracts only); full-locale
backfill belongs to the P10-T08 device/i18n pass, never claimed here.
`:app:connectedDebugAndroidTest` + `:data:connectedDebugAndroidTest`
NOT run locally (no emulator); device coverage deferred to CI/phase
exit. No backend change; no production URL invented. B-BR-005/006/007:
one stable id per intent, retries never amplify, one latest vote per
contributor server-side. B-BR-011: private detail in body only.
