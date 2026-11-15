# Google Play readiness — Abastevo Android

Audit date: 2026-11-15. Audited source: `origin/main` `93f3a88d` (tree equal to
`origin/dev` `f17f57ec`) plus the fixes in this branch. Official requirements
were read on 2026-11-15 from the sources listed in [Sources](#sources). Recheck
them at submission time; Play policy changes often.

**Overall verdict: NOT READY for a production Play submission.** The store
listing assets, privacy/deletion URLs, 16 KB alignment and the API 36 target
are in place. Three items still block a first submission: user-generated-content
reporting/blocking, a signed AAB (release channel or a manual upload signed
with the owner's upload key), and the Play Console declarations (Data safety,
foreground services, content rating). None of these can be completed by code in this
branch alone.

Legend: **VALID** — meets the requirement with evidence; **FIXED** — was
failing, corrected in this branch; **BLOCKING** — must be resolved before
submission; **TO CONFIRM** — needs an owner decision or a Play Console answer.

Privacy depth (Data safety inventory, LGPD duties, Apple privacy) lives in
[STORE_PRIVACY_READINESS.md](../release-evidence/STORE_PRIVACY_READINESS.md);
this checklist does not repeat it. Apple asset specs for a future iOS release:
[APPLE_APP_STORE_ASSETS.md](APPLE_APP_STORE_ASSETS.md).

## 1. Build and technical requirements

| # | Requirement | Verdict | Evidence / action |
|---|---|---|---|
| 1.1 | Target API level: new apps and updates must target **API 36** since 2026-08-31; the extension window closed 2026-11-01 [S1][S2] | **FIXED** | Was `targetSdk = 35` (submission would be rejected). Now `compileSdk = 36`, `targetSdk = 36` in `gradle/libs.versions.toml`. `:app:assembleDebug` and `:app:bundleRelease` (R8, resource shrinking, lint vital) pass offline; `aapt2 dump badging` reports `targetSdkVersion:'36'`. The API 36 build was installed on the API 36 emulator and driven through the tablet captures (home, map, forum, history, stations, week sheet) without crashes. |
| 1.2 | Android 16 behavior changes that apply at targetSdk 36 [S3] | **VALID** (TO CONFIRM on a physical Android 16 device) | No `onBackPressed` override, no `KEYCODE_BACK`, no `screenOrientation`/`setRequestedOrientation`, no `resizeableActivity`, no `windowOptOutEdgeToEdgeEnforcement`, no `elegantTextHeight`, no `scheduleAtFixedRate`. Back handling is Compose `BackHandler`; edge-to-edge is already enabled in `MainActivity`. Predictive back on a real device was not exercised. |
| 1.3 | Android Gradle Plugin support for compileSdk 36 | **TO CONFIRM** | AGP 8.7.3 builds with compileSdk 36 but prints *"We recommend using a newer Android Gradle plugin to use compileSdk = 36"*. Upgrade AGP (≥ 8.9, Gradle wrapper ≥ 8.11) as its own task with the full Android gate; it is not needed to produce a valid API 36 artifact. |
| 1.4 | New apps must be published as an **Android App Bundle**, enrolled in Play App Signing [S30][S31] | **TO CONFIRM** (owner signs) | `:app:bundleRelease` builds locally (unsigned, 53.9 MB `app-release.aab`, versionName/versionCode passed as `-PandroidReleaseVersionName`/`-PandroidReleaseVersionCode`). The owner signs it with their upload key and uploads it manually. The automated channel (`android-release`, [release-build.md](../release-build.md)) still signs an APK only; an AAB job is a follow-up. |
| 1.5 | Signing continuity with the existing sideloaded APK | **VALID** (owner decision) | Owner decided on 2026-11-15 to use Play App Signing with a **new upload key** instead of the GitHub-release key. Consequence: APKs installed from GitHub releases (signed by the pinned key in `release/android/`) cannot be updated by the Play build; those users uninstall once to move to Play. The upload key and its password stay with the owner, outside Git. |
| 1.6 | 16 KB memory page size for native libraries [S32][S33] | **VALID** | Every `.so` in the APK (arm64-v8a and armeabi-v7a: `liborganicmaps`, `libmonocypher_ed25519`, AndroidX/CameraX libs) has ELF `LOAD` alignment 16384, and `zipalign -c -P 16 -v 4` reports *Verification successful*. |
| 1.7 | Size limits: 500 MB base module, warning above 200 MB on mobile data [S34] | **VALID** | AAB 53.9 MB; Play serves per-ABI splits. The existing 80 MiB APK ceiling stays enforced for the sideload channel. |
| 1.8 | ABIs | **VALID** (note) | arm64-v8a and armeabi-v7a only. x86_64 devices (some Chromebooks) cannot install. Acceptable for a phone-first launch. |
| 1.9 | Package name is permanent after the first upload | **TO CONFIRM** (owner) | `com.anpfuel.app` (kept per AGENTS.md). Once uploaded it can never change. |
| 1.10 | versionCode monotonic, ≤ 2,100,000,000 [S7] | **VALID** | Derived by `scripts/android/release_flow.py`; Play's first upload must use a code ≥ every sideloaded build users have. |
| 1.11 | Release hygiene | **VALID** | Non-debuggable release, `usesCleartextTraffic=false`, base network config forbids cleartext, auth keys/sealed vault/Room DB excluded from backup and device transfer. |

## 2. App content declarations (Play Console → Policy → App content)

| # | Requirement | Verdict | Evidence / action |
|---|---|---|---|
| 2.1 | Privacy policy: public, non-PDF, non-geofenced URL in the Console **and** inside the app [S9] | **VALID** | `https://abastevo.com.br/privacidade/` returns HTTP 200 HTML. In-app: Terms gate (`TermsGate.kt`) and Privacy screen (`PrivacyDocumentLinks.kt`). [docs/privacy-policy.md](../privacy-policy.md) now points at the official domain (it still linked the retired `teste.` host). |
| 2.2 | Account deletion: in-app path **and** a web resource naming the app, how to request, what is retained [S10] | **VALID** (operational TO CONFIRM) | In app: Perfil → Gerenciar conta → Excluir minha conta → Sim, excluir (`POST /v1/accounts/deletion`). Web: `https://abastevo.com.br/excluir-conta/` (HTTP 200) names Abastevo and the controller (ZAFLAS INOVA SIMPLES), the in-app path, a WhatsApp/phone/post route and retained records. Completed production erasure is still an open obligation in STORE_PRIVACY_READINESS. |
| 2.3 | Data safety form [S8] | **TO CONFIRM** (Console) | Draft answers in [§5](#5-data-safety-draft). Must be completed by the owner from the final AAB; never declare "no data collected". |
| 2.4 | Government information / misrepresentation: non-government apps showing government data must cite the source and state non-affiliation in the listing; the Government apps declaration is mandatory [S21][S22] | **FIXED** (listing) | The old [listing draft](../play-store-listing.md) claimed "no account, no backend, 100% on-device" and linked the privacy policy on GitHub — false for the current app. It is rewritten: the description cites `https://www.gov.br/anp/pt-br` as the source and says the app is not affiliated with ANP or any government body. The app shows the same statement on the first-launch Terms gate. App name "Abastevo" has no "ANP"/"oficial". Declaration answer: **not a government app**. |
| 2.5 | User-generated content: terms before posting, defined objectionable content, moderation, **in-app reporting of content and users, and blocking of users** for publicly accessible UGC [S19] | **BLOCKING** | Present: first-launch terms acceptance; Terms §2–3 prohibit offensive content/spam and describe moderation; forum topics and comments are pre-moderated (`PENDING_REVIEW`); reporting exists for forum comments/replies, station comments and charging observations. **Missing:** reporting a *user*, blocking/muting a user, and a report action on forum *topics* (the HTTP client supports it, the screen does not). Needs a documented B-BR/BUC, backend and app task. |
| 2.6 | Location permissions: prefer foreground; background needs a declaration [S12][S13] | **VALID** (TO CONFIRM fine-location form) | Only `ACCESS_COARSE/FINE_LOCATION`; no `ACCESS_BACKGROUND_LOCATION`. Onboarding shows its own prompt ("Encontrar sua cidade automaticamente?") with a manual alternative before the system dialog. Google opened an `ACCESS_FINE_LOCATION` declaration in Nov 2026 — answer it if the Console asks. City detection sends full-precision coordinates to OSM Nominatim; rounding them (or proxying server-side) is recommended and must be reflected in Data safety. |
| 2.7 | Foreground service types must be declared with description, impact and a video [S14] | **TO CONFIRM** (Console) | Three `foregroundServiceType="location"` services, all user-started with a visible notification and a stop action: `GuidanceService` (turn-by-turn navigation), `TripRecordingService` (record a trip), `DriverGpsService` (driver shift GPS, reachable from Veículos → Central do motorista even with enterprise disabled server-side). Record one short video per use case. |
| 2.8 | Camera | **VALID** | Used only when the user photographs a price board; OCR runs on-device (ML Kit). No Play declaration; uploaded photos go in Data safety. |
| 2.9 | Photo and video permissions policy [S11] | **VALID** | No `READ_MEDIA_*`; the vehicle photo uses the system Photo Picker and stays on the device. |
| 2.10 | Notifications | **VALID** | `POST_NOTIFICATIONS` requested when the user enables a price-drop alert; alerts are local (WorkManager); no push/FCM. |
| 2.11 | Exact alarms | **VALID** | Not used. |
| 2.12 | Ads declaration [S20] | **VALID** | No ads SDK, no house ads → declare **No ads**. |
| 2.13 | In-app purchases / payments policy | **VALID** (TO CONFIRM before any sale) | No Play Billing and no in-app checkout in the app; the premium screen only shows status. The backend has Stripe/Asaas checkout code: enabling any in-app sale of digital features first needs a Play Payments policy decision. |
| 2.14 | Financial features declaration [S23] | **VALID** | Answer "My app doesn't provide any financial features" (the private fuel/expense log is personal record keeping). |
| 2.15 | Target audience and content [S24] | **TO CONFIRM** (owner) | Recommended: 18+ only (drivers; public forum), which keeps the app out of the Families policy. No age gate exists in the app; the privacy page says the service is not presented as a children's service. |
| 2.16 | Content rating (IARC; ClassInd in Brazil) [S17][S18] | **TO CONFIRM** (Console) | Answer truthfully: no violence/sexual content/gambling; **users can interact** (forum, comments); no location sharing with other users; no digital purchases. |
| 2.17 | News apps declaration | **TO CONFIRM** | The forum carries editorial articles, but the app is not a news publisher; expected answer "No". |
| 2.18 | Health apps / other declarations | **VALID** | Not applicable; answer "No". |
| 2.19 | App access for review | **TO CONFIRM** | Prices, map and forum reading need no account. Give reviewers the username-only sign-up steps for posting; never share a real person's account. |

## 3. Developer account and release track

| # | Requirement | Verdict | Evidence / action |
|---|---|---|---|
| 3.1 | Account type: organization accounts need a **D-U-N-S number** [S26] | **VALID** (owner report) | The owner reported on 2026-11-15 that the organization developer account already exists (controller ZAFLAS INOVA SIMPLES (I.S.), CNPJ 67.852.667/0001-93). Not verified from here. |
| 3.2 | New personal accounts: closed test with **≥ 12 opted-in testers for 14 consecutive days** before production [S25] | **n/a** | Applies only to personal accounts; the app is published from an organization account. An internal or closed test before production is still advisable. |
| 3.3 | Android developer verification, enforced in Brazil since 2026-09-30 [S27][S28][S29] | **TO CONFIRM** (owner) | Play-distributed apps are registered through Play App Signing. Direct APK sideloads are not enforced yet (global rollout planned for 2027) and ADB installs are exempt, but register the package and the existing signing key in the Android Developer Console to keep the GitHub APK installable later. |
| 3.4 | Store listing contact email (required) and website | **TO CONFIRM** (owner) | Website: `https://abastevo.com.br`. No public contact email is documented, and the project rules forbid inventing one — the owner must supply it. |

## 4. Store listing and graphics

| # | Requirement | Verdict | Evidence / action |
|---|---|---|---|
| 4.1 | App name ≤ 30, short description ≤ 80, full description ≤ 4000 characters [S7] | **VALID** | [play-store-listing.md](../play-store-listing.md) (pt-BR default, en-US): lengths are checked in the doc. |
| 4.2 | App icon 512 × 512, 32-bit PNG, ≤ 1024 KB, full square (Play applies the mask) [S4][S5] | **VALID** | `listing-assets/icon-512.png`, rendered from the approved `docs/assets/brand/abastevo-app-icon-1024.png`. |
| 4.3 | Feature graphic 1024 × 500, JPEG or 24-bit PNG without alpha, key content centered, no pure white/black background, no badges [S4] | **VALID** | `listing-assets/feature-graphic-1024x500.png` (RGB, brand navy gradient, logo + name + tagline centered, decoration only at the edges). |
| 4.4 | Phone screenshots: 2–8, 320–3840 px, ≤ 2:1; for promotion ≥ 4 at ≥ 1080 px in 9:16/16:9 [S4] | **VALID** | 8 cards at 1080 × 1920 (9:16, RGB) in `listing-assets/phone/`. Raw captions-free captures in `screenshots/raw/phone/`. |
| 4.5 | 7-inch and 10-inch tablet screenshots: ≥ 4, 1080–7680 px, 9:16 or 16:9 [S4] | **VALID** | 6 cards each: `listing-assets/tablet-7in/` (1080 × 1920, São Paulo) and `listing-assets/tablet-10in/` (1440 × 2560, Sorriso), captured on the emulator at tablet densities (sw ≈ 617 dp and 720 dp). |
| 4.6 | Screenshot content rules: real UI, captions ≤ 20 % of the image, no device frames, no "best/#1/new/free/sale", no calls to action, no store badges, no cut-off text on large screens [S4][S6] | **VALID** | Captions are one short pt-BR line pair in the top band (well under 20 %), the UI is an unedited emulator capture, there is no device frame, and no claim/CTA wording is used. If the large-screen preview ever crops the caption band, upload the raw tablet captures instead. |
| 4.7 | Screenshots privacy | **VALID** | Production data (public ANP station names/addresses), a synthetic emulator location, the generic vehicle label "Meu carro", no account signed in, no personal data, no photos of people. |
| 4.8 | Category and tags | **TO CONFIRM** | Proposed: Auto & Vehicles. |
| 4.9 | Promo video (optional) | n/a | Not produced. |

## 5. Data safety draft

Derived from a source audit of this revision — key files:
`CaptureOcrViewModel.kt` / `DirectoryStationHttpClient.kt` (nearby lookup),
`NominatimReverseGeocodeClient.kt`, `AccountHttpApi.kt`,
`AnonymousProofHttpClient.kt`, `ContributionUploadHttpClient.kt`,
`AndroidPhotoCodec.kt`, `LedgerSyncFlagStore.kt`, `data/build.gradle.kts` (ML Kit)
— and the deeper inventory in STORE_PRIVACY_READINESS. Encryption in transit:
**yes** (HTTPS only). Users can request deletion: **yes** (in-app and web).
Independent security review: **no**.

| Data type (Play category) | Collected? | Shared? | Optional? | Purpose | Notes |
|---|---|---|---|---|---|
| Location — approximate and precise | Yes (ephemeral) | TO CONFIRM | Optional | App functionality | Price capture sends the last fix to `/v1/stations/nearby` (server logs only the route template). City detection sends coordinates to OSM Nominatim — treat as sharing unless a service-provider exception is confirmed, or round/proxy it first. Navigation/recording stay on the device. |
| Personal info — user IDs | Yes | No | Optional | App functionality, account management, fraud prevention | Username-only account; pseudonymous contributor key (P-256 public key fingerprint). |
| Photos | Yes | No | Optional | App functionality, fraud prevention | Price-board photo attached to a contribution; EXIF removed; access window 24 h. Vehicle photo stays on the device. |
| Other user-generated content | Yes | No (published to other users) | Optional | App functionality | Forum topics/comments, station comments/ratings, price contributions, quality reports, charging observations. |
| Financial info — other | Yes, only if private sync is enabled | No | Optional (default off) | App functionality | Private fuel/expense log sync (`LedgerSyncFlagStore` default off). |
| App activity — app interactions | Yes | No | Optional | App functionality, fraud prevention | Votes, reports, confirmations. |
| App info and performance — diagnostics; Device or other IDs | Yes (Google ML Kit / Play services SDK) | No | Required when OCR is used | Analytics/diagnostics by the SDK | Per the [ML Kit data disclosure](https://developers.google.com/ml-kit/android-data-disclosure); image content stays on the device. |

Not collected: contacts, SMS, call logs, calendar, audio, health, web history,
advertising ID, payment card data (no in-app sale is enabled).

## 6. Changes made in this branch

- `compileSdk`/`targetSdk` 35 → 36; the `android-release` workflow now installs
  `platforms;android-36`. Build docs updated.
- The Settings "Share app" message pointed at a GitHub APK download under the old
  repository/app name. A Play-distributed app should not steer users to install
  APKs from another source (Device and Network Abuse), so it now shares
  `https://abastevo.com.br/` with the current name in all 8 locales.
- [play-store-listing.md](../play-store-listing.md) rewritten for the current app.
- [privacy-policy.md](../privacy-policy.md) links the official domain.
- Store screenshots, cards, feature graphic and icon added under `docs/store/`
  with a reproducible generator ([README](README.md)).

## 7. Follow-ups (not done here)

1. **UGC compliance** — report users, block users, report forum topics, and make
   the Terms list of objectionable content explicit (backend + app, B-BR/BUC first).
2. **AAB release channel** — automate `bundleRelease` signing with the owner's
   upload key (CI secret) and run the artifact checks on the AAB-generated APKs.
3. **Console declarations** — Data safety (§5), foreground-service videos (2.7),
   content rating, target audience, government apps, financial features, ads,
   app access.
4. **Account path** — developer verification for the GitHub APK channel and the public contact email (the organization account exists).
5. **Nominatim precision** — round or proxy coordinates before reverse geocoding.
6. **AGP upgrade** to a version that supports compileSdk 36 without the warning.

## Sources

Fetched 2026-11-15.

- S1 [Target API level requirements (Play Console Help)](https://support.google.com/googleplay/android-developer/answer/11926878)
- S2 [Meet Google Play's target API level requirement](https://developer.android.com/google/play/requirements/target-sdk)
- S3 [Android 16 behavior changes for apps targeting 16](https://developer.android.com/about/versions/16/behavior-changes-16)
- S4 [Add preview assets to showcase your app](https://support.google.com/googleplay/android-developer/answer/9866151)
- S5 [Google Play icon design specifications](https://developer.android.com/distribute/google-play/resources/icon-design-specifications)
- S6 [Metadata policy](https://support.google.com/googleplay/android-developer/answer/9898842)
- S7 [Create and set up your app](https://support.google.com/googleplay/android-developer/answer/9859152)
- S8 [Data safety section](https://support.google.com/googleplay/android-developer/answer/10787469)
- S9 [User Data policy / privacy policy](https://support.google.com/googleplay/android-developer/answer/10144311)
- S10 [Account deletion requirements](https://support.google.com/googleplay/android-developer/answer/13327111)
- S11 [Photo and video permissions](https://support.google.com/googleplay/android-developer/answer/14115180)
- S12 [Location permissions](https://support.google.com/googleplay/android-developer/answer/9799150)
- S13 [Location permission updates](https://support.google.com/googleplay/android-developer/answer/17033915)
- S14 [Foreground service declarations](https://support.google.com/googleplay/android-developer/answer/13392821)
- S17 [Content ratings](https://support.google.com/googleplay/android-developer/answer/9898843)
- S18 [Rating systems by region](https://support.google.com/googleplay/android-developer/answer/188189)
- S19 [User-generated content policy](https://support.google.com/googleplay/android-developer/answer/9876937)
- S20 [Ads declaration](https://support.google.com/googleplay/android-developer/answer/9859455)
- S21 [Government apps / government information](https://support.google.com/googleplay/android-developer/answer/9514050)
- S22 [Misrepresentation and government affiliation](https://support.google.com/googleplay/android-developer/answer/9887877)
- S23 [Financial features declaration](https://support.google.com/googleplay/android-developer/answer/13849271)
- S24 [Target audience and content](https://support.google.com/googleplay/android-developer/answer/9867159)
- S25 [Testing requirements for new personal accounts](https://support.google.com/googleplay/android-developer/answer/14151465)
- S26 [D-U-N-S number for organization accounts](https://support.google.com/googleplay/android-developer/answer/13628312)
- S27 [Android developer verification](https://developer.android.com/developer-verification)
- S28 [Developer verification guides](https://developer.android.com/developer-verification/guides)
- S29 [Developer verification FAQ](https://developer.android.com/developer-verification/guides/faq)
- S30 [Android App Bundles](https://developer.android.com/guide/app-bundle)
- S31 [Play App Signing](https://support.google.com/googleplay/android-developer/answer/9842756)
- S32 [Support 16 KB page sizes](https://developer.android.com/guide/practices/page-sizes)
- S33 [Technical quality requirements](https://support.google.com/googleplay/android-developer/answer/17492799)
- S34 [App size limits](https://support.google.com/googleplay/android-developer/answer/9859372)
