# Mobile tutorial catalog for the landing

Date: 2026-10-11. Status: PLANNED; capture availability is tracked separately.
Owner: [LR01/LR13–LR25](../planning/LANDING_REDESIGN_EXECUTION_PLAN.md).
Execution branch: **landing**. Product instructions below are pt-BR; technical
contracts/evidence stay English. Every walkthrough uses **real app captures**.

## Frontend and coverage contract

The home opening contains no phone mockup. A separate #aplicativo section shows
actual screenshots in restrained frames. A separate #tutoriais section introduces
/como-usar/; full chapters live under /como-usar/<slug>/.
Marketing screenshots do not receive instructional red boxes. Tutorial frames
use transparent SVG overlays with numbered red rectangles, a Portuguese action,
expected result and recovery. Keep the original PNG pixels untouched.

Map every route and reachable control, including dialogs, menus, scroll/filter
options, empty/error/offline and permission states. Re-audit Routes/AnpNavGraph
and the actual installed build. Source-present is not device-ready. Missing
frames remain MEDIA_PENDING; never populate them with invented screens.

Versioned step fields: lessonId, stepId, appVersion, appSourceSha (nullable),
image, imageSha256, nativeWidth, nativeHeight, state, prerequisite,
instructionPtBr, controlLabelPtBr, expectedPtBr, recoveryPtBr, altPtBr,
target (x/y/width/height in original pixels), availability, claimClass,
sourceFiles, capturedAt, privacyReviewed and demoOnly. Result steps can omit target.
Validate hashes/dimensions, bounds, unique IDs, text and availability at build time.

For native W×H, normalize a rectangle as x/W, y/H, width/W, height/H. Prefer an
SVG with the same W×H viewBox over an intrinsic-ratio image; no cover cropping.
If contain/letterboxing is used, apply identical fit and offsets. Use a visible
non-scaling stroke, no opaque fill over controls, number/text besides red, and
verify targets at 320/375/768/1440px and 200% zoom. Inspect actual control bounds,
not approximate positions. Demo Next/Previous never calls a mobile write API.

Primary current account flow is username/key. Legacy providers/email, future
helpfulness, offline expansion and training consent need owner implementation
and actual UI capture before active instructions. Do not build them for marketing.

## Capability freeze (LR01) — 2026-10-11

Checked read-only against `app/.../navigation/Routes.kt` (25 route constants),
`AnpNavGraph.kt` (every constant wired to a screen) and
`ui/auth/AuthViewModel.kt` (username/key primary flow). Source-present is not
device-ready; capture availability stays in the media manifest.

| Route constant(s) | Wired screen | Chapter |
|---|---|---|
| ONBOARDING | OnboardingScreen | TU01 |
| LOCATION | LocationPickerScreen | TU02 |
| HOME / EXPLORE | HomeScreen | TU03 |
| SEARCH | SearchScreen | TU04 |
| PRICES | PricesScreen | TU05 |
| WEEK_PICKER | WeekPickerRoute | TU06 |
| HISTORY | HistoryScreen | TU07 |
| STATIONS / STATIONS_WITH_FUEL | StationsScreen | TU08 |
| STATION_PAGE | StationPageScreen | TU09 (+TU18/TU19/TU21 controls) |
| COMMUNITY | CommunityScreen | TU10 |
| AUTH | AuthRoute/AuthScreen | TU11/TU12 |
| ACCOUNT | AccountScreen | TU12/TU13 |
| CAPTURE / CAPTURE_WITH_TARGET | CaptureScreen | TU14 |
| (capture OCR/review components) | review step | TU15 |
| (PhotoSubmissionFeedback/outbox) | send states | TU16 |
| CONTRIBUTIONS | MyContributionsScreen | TU17 |
| SUGGEST | SuggestStationScreen | TU22 |
| STATION_PROFILE | StationProfileScreen | TU23 |
| STATION_CLAIM | StationClaimScreen | TU24 |
| STATION_MANAGEMENT | StationManagementScreen | TU25 |
| VEHICLES | VehicleScreen | TU26 (+TU27 toggle) |
| SETTINGS (+PROFILE entry) | SettingsScreen/ProfileScreen | TU28 (+TU13/TU17/TU26/TU30 entries) |
| (all routes) | cached/freshness behavior | TU29 |
| HELP (+PROFILE entry) | HelpScreen | TU30 |

Explicitly gated (no active lesson, no invented screens):

- **TU20 helpfulness:** PENDING. CS01 is domain/contract only; no Android
  helpfulness UI exists. VALID/INVALID feedback (TU18/TU19) stays distinct.
- **Offline city packs / durable social queues:** PLANNED (CS15–CS25). TU29
  teaches only actual per-route cached behavior.
- **Training consent switch:** PROPOSED, not active. TU28 keeps it pending.
- **Email/provider auth:** code-present (`AuthScreen` email field, AUTH
  provider deep-link args) but the native provider SDK is device-gated and the
  working primary flow is username + saved account key. Tutorials teach
  username/key; provider/email appendices stay pending runtime proof.
- **PMQC UI, paid benefits, iOS:** no mobile control evidence; pending.

## Driver and discovery lessons

### TU01 — Primeiros passos (primeiros-passos)

- **Route/source:** ONBOARDING; OnboardingScreen/LocationPromptStep.
- **Steps:** disposable fresh app → actual sync/download choice → progress/result
  → optional location or manual city → Explorar with survey date. Highlight the
  actual primary control in each frame.
- **Recovery:** offline/import failure/denied location; never reset the user's phone.

### TU02 — Escolher cidade (escolher-cidade)

- **Route/source:** LOCATION; HomeScreen/LocationPickerScreen/Filter.
- **Steps:** Explorar → Minha cidade → Buscar estado or state row → municipality
  search/list → municipality selection → verified city in Explorar. Red boxes
  surround each exact input/row; the final result needs no box.
- **Recovery:** GPS-free manual path, empty query/no survey coverage and return.

### TU03 — Entender Explorar (explorar)

- **Route/source:** HOME/EXPLORE alias; HomeScreen.
- **Steps:** city → active week → fuel cards/source/date → Ver todos → tabs →
  actual tank estimate when available → theme control.
- **Recovery:** no city/no data/stale/error. Never call a weekly reference live.

### TU04 — Buscar cidades e postos (buscar)

- **Route/source:** SEARCH; SearchScreen and actual entry point.
- **Steps:** observed entry → query → actual result types/filters → selection →
  return. Do not invent a home search bar or map.
- **Recovery:** empty query/results, unknown city and connection failure as applicable.

### TU05 — Consultar a tabela ANP (precos-anp)

- **Route/source:** PRICES; PricesScreen.
- **Steps:** Perfil/Tabela completa de preços or Explorar/Ver todos → city/week →
  fuel → average/min/max/count. Keep currency, units, date and scope readable.
- **Recovery:** empty survey/scope mismatch. A survey is not a current pump offer.

### TU06 — Trocar ou baixar uma semana (semanas-anp)

- **Route/source:** WEEK_PICKER; WeekPickerScreen/BottomSheet and week chip.
- **Steps:** open → latest/archive list → actual download confirmation/progress →
  active week. Expand operational notes where offered.
- **Recovery:** missing archive/offline/failed sync. No huge real-phone download
  merely to stage screenshots.

### TU07 — Ler o histórico (historico)

- **Route/source:** HISTORY; HistoryScreen.
- **Steps:** Perfil/Histórico de preços ANP → fuel/scope/period options → actual
  chart/legend/date/value interpretation.
- **Recovery:** insufficient weeks/history disabled; link TU06/TU28. Never invent
  charts when only one week exists.

### TU08 — Encontrar e navegar até um posto (postos)

- **Route/source:** STATIONS/STATIONS_WITH_FUEL; StationsScreen.
- **Steps:** entry/fuel → offered sort/filter/list/page → station → external
  navigation chooser where available. Explain unknown/approximate locations.
- **Recovery:** no detail download/no stations/no Maps/Waze-compatible handler.

### TU09 — Entender os preços de um posto (precos-do-posto)

- **Route/source:** STATION_PAGE; StationPageScreen/community components.
- **Steps:** identity → fuel → separate community/ANP → conditions/date/confidence
  → unknown/disputed/stale → update-price entry.
- **Recovery:** unavailable community/official data, while preserved browsing
  stays usable. Never blend references into a fictitious current cheapest price.

### TU10 — Usar a Comunidade (comunidade)

- **Route/source:** COMMUNITY; CommunityScreen/FeedViewModel.
- **Steps:** tab → city/Trocar → actual recent/cheapest/best-fuel ordering → fuel
  chips including horizontal scroll → refresh/next page → card/rules.
- **Recovery:** actual empty city/fuel, error/quota/cache states. An empty capture
  is labelled empty; do not invent people/prices to fill a feed.

## Free accounts and participation lessons

### TU11 — Criar uma conta gratuita (criar-conta)

- **Route/source:** AUTH; AuthScreen/AuthViewModel.
- **Steps:** guest Perfil → Entrar ou criar conta gratuita → username/rules →
  registration → protected backup acknowledgement → return, using disposable data.
- **Recovery:** invalid/taken name/offline. No payment or actual key in media.

### TU12 — Entrar com chave e guardar backup (chave-e-backup)

- **Route/source:** AUTH/ACCOUNT; AccountKeyCard/AccountScreen.
- **Steps:** Já tenho uma chave → key field/login → manage account → masked
  backup controls → copy/export/save, only in a disposable synthetic account.
- **Recovery:** invalid/lost key, offline saved session/expired access. Never
  promise unsupported email recovery or ask for a key on the landing. No real
  clipboard/share-sheet key content is retained.

### TU13 — Sessão, privacidade e exclusão (conta-e-privacidade)

- **Route/source:** ACCOUNT and genuine privacy/rights destinations.
- **Steps:** manage → saved/offline status → logout explanation → deletion
  confirmation/cancel → actual completed state in disposable environment.
- **Recovery:** unconfirmed/network request. Distinguish intent/receipt/completion
  and anonymized facts/retained data. No real-user logout/deletion/provider unlink.

### TU14 — Fotografar um preço (fotografar-preco)

- **Route/source:** CAPTURE/CAPTURE_WITH_TARGET; capture/location permissions.
- **Steps:** contribution entry → location/accuracy eligibility → station → camera
  permission → capture owned synthetic board.
- **Recovery:** denied camera/GPS, mocked/invalid location, no station, expired
  admission and camera error. Read active thresholds; never teach debug bypass.

### TU15 — Recortar e revisar valores (revisar-precos)

- **Route/source:** capture OCR/review components.
- **Steps:** photo → crop → OCR → edit detected fuels → remove optional rows →
  check source/condition → explicit send. Boxes target genuine controls.
- **Recovery:** OCR/blur/decimal/unsupported/empty fuel/size/missing station errors.
  Never generate a successful OCR screen or store production photos.

### TU16 — Entender envio e fila (envio-e-fila)

- **Route/source:** PhotoSubmissionFeedback and outbox UI.
- **Steps:** explicit send → queued → sending/retry → server receipt → validation
  pending → actual partial/refused/expired/cancelled states.
- **Recovery:** connection return/admission expiry. Queued is not sent; received
  is not validated; missing receipt is never success.

### TU17 — Minhas contribuições (minhas-contribuicoes)

- **Route/source:** CONTRIBUTIONS; MyContributionsScreen/ledger.
- **Steps:** Perfil entry → list/status/available receipt details → real cancel
  action and contribution shortcut. Use approved isolated fixtures for states.
- **Recovery:** guest/empty/offline/error/cancel rules. No fabricated edit/retry.

### TU18 — Avaliar, comentar e responder (avaliacoes-e-comentarios)

- **Route/source:** station experience/feedback within STATION_PAGE.
- **Steps:** sign in → station/fuel → stars → save/remove own rating → bounded
  comment/submit → available reply/comment vote.
- **Recovery:** length/empty/unauthorized/queued/refused. Edit/remove comment is
  taught only if exposed. VALID/INVALID feedback is distinct from helpfulness.

### TU19 — Confirmar ou contestar (confirmar-ou-contestar)

- **Route/source:** actual community voting controls/ViewModel.
- **Steps:** exact observation/value/condition → confirm or dispute → actual
  reason/private details/optional replacement → submit → result.
- **Recovery:** duplicate/stale/ineligible/self/quota as implemented. No vote
  transfer to another price; support is not a correctness guarantee.

### TU20 — Contribuição útil ou não útil (utilidade-da-contribuicao)

- **Opening availability:** PENDING. CS01 domain/contract is not a tested Android
  helpfulness UI. Do not create fake lesson screenshots.
- **After owner implementation/capture:** exact contribution/revision → helpful
  or not helpful → own choice/counts → change/remove/refused cases.
- **Rule:** separate helpfulness, correctness, station rating and moderation.

### TU21 — Denunciar abuso (denunciar-abuso)

- **Route/source:** comment overflow/report dialogs; HelpScreen.
- **Steps:** overflow → report → actual reason/explanation → send → receipt.
- **Recovery:** duplicate/quota/offline/refused. Explain restricted review/appeal;
  reporting never auto-deletes. Never report real users to stage screenshots.

### TU22 — Sugerir posto ausente (sugerir-posto)

- **Route/source:** SUGGEST; SuggestStationScreen.
- **Steps:** actual stations entry → sign-in → name/IBGE/UF/optional CNPJ →
  validation/duplicate hint → send → private status/refresh/cancel.
- **Recovery:** fields/conflict/quota/offline. Submission is not approval.

## Representatives and local tools

### TU23 — Consultar perfil do posto (perfil-do-posto)

- **Route/source:** STATION_PROFILE; StationProfileScreen.
- **Steps:** identity/anchors → real representation badges → business fields,
  services/hours/contact → representation note/claim entry.
- **Recovery:** unavailable/stale/empty. Badge is not ANP quality certification.

### TU24 — Solicitar representação (representar-posto)

- **Route/source:** STATION_CLAIM; StationClaimScreen.
- **Steps:** login → role/scope → request → export declaration → genuine external
  signature process → import PDF → submit → review/status/cancel/reissue.
- **Recovery:** invalid/missing/expired document, pending/denied/cancelled claim.
  No real ID/signature PDF in Git; no forged approved representative state.

### TU25 — Gerenciar informações autorizadas (gerenciar-posto)

- **Route/source:** STATION_MANAGEMENT; StationManagementScreen.
- **Steps:** scoped approved account → permitted fields/services → save → bounded
  representative fuel reply → status.
- **Recovery:** no/expired/revoked scope/stale conflict. Management never overwrites
  canonical identity, community/ANP prices or user ratings. Disabled stays pending.

### TU26 — Veículos e estimativa do tanque (veiculos)

- **Route/source:** VEHICLES; VehicleScreen/VehicleForm and Home estimate.
- **Steps:** Perfil/Meus veículos → add → name/liters/fuel/reference → actual
  specific-post selection → alerts → save/estimate → edit → delete-confirm/cancel.
- **Recovery:** required name/invalid liters/missing station/three-vehicle limit/
  no price reference. Use disposable data for save/delete. No license-plate field
  invented from AI artwork; tank fill remains a dated estimate.

### TU27 — Alertas locais (alertas)

- **Route/source:** vehicle toggle and Settings notifications section.
- **Steps:** vehicle → price-drop preference → OS permission → return → explain
  local evaluation after actual data import.
- **Recovery:** disabled/denied/no new import. Not guaranteed live monitoring;
  OS screenshots device-labelled, free of personal notifications.

### TU28 — Configurações (configuracoes)

- **Route/source:** SETTINGS/PROFILE; SettingsScreen/ProfileScreen.
- **Steps:** actual theme control, pt-BR/English, station sync, latest week,
  Wi-Fi-only, history, retention, nearby radius, storage, sync-now, week selection,
  cache-clear confirmation, share/about/privacy/auth if reachable.
- **Recovery:** permission/empty cache/sync error. Destructive actions only on
  disposable data. Future training-consent switch stays pending until real UI.

### TU29 — Offline e recuperação (offline-e-recuperacao)

- **Route/source:** each route owner, Room/outbox and actual notices.
- **Steps:** per-route saved data/freshness → unseen/unavailable remote data →
  queued action → reconnect/expiry/status; cross-link chapter recovery steps.
- **Recovery:** cache missing, network-only identity/admission, process restart,
  clock/media expiry according to actual version. CS city packs/social queues are
  planned, not current evidence. No airplane-mode changes on user phone by default.

### TU30 — Ajuda e regras (ajuda-e-regras)

- **Route/source:** HELP; HelpScreen/source dialogs/legal/contact/share links.
- **Steps:** Perfil help → contribution rules → moderation/appeal → privacy/source
  explanation → actual external legal/contact destination.
- **Recovery:** offline/no browser/share handler. No automatic message submission.

## Permission and impact matrix (LR23) — 2026-10-11

Verified on the test device (`appops`, 2026-10-11): camera and fine location
granted foreground-only; no background grant observed. Tutorials never ask the
reader to grant, deny or revoke anything; denial/rationale screens stay
pending on a disposable emulator.

| Capability | Lessons | Rule |
|---|---|---|
| Location (foreground) | TU02 (current), TU08 nearby, TU14 area check | Read-only use of an existing grant; no coordinates in pixels or text |
| Camera | TU14/15 photo | Shutter never pressed on the user phone; synthetic fixtures only |
| Notifications (OS) | TU27 alerts | Toggle viewed off; OS permission screen pending |
| Network/sync | TU06/TU28/TU29 settings | Read without toggling; airplane test restored immediately |
| Signed account | TU11–TU13/TU17–TU21/TU24–25 | Demo `zanatateste` session; no vote/report/upload/delete staged |

## Acceptance reference (LR24) — 2026-10-11

A chapter is done when: every step carries instruction, expected result and
recovery in full text without JS; control rectangles match inspected bounds;
result frames omit targets; no step promises success or teaches an action the
app does not perform; deletion/account impacts state intent, confirmation and
network need; search, status and result transitions resolve to emitted pages.
Bounded negative tests (`chapter-content`, `tutorial-data`, `hub-content`)
guard recovery text, target omission, success promises, dead links and
unverifiable claims on every shipped chapter.

## Data and annotation contract (LR13) — 2026-10-11

`lessonId, slug, titlePtBr, appVersion, appSourceSha (nullable), steps[]`.
Each step: `stepId, image, imageSha256 (hex64), nativeWidth, nativeHeight,
state, prerequisite, instructionPtBr, controlLabelPtBr, expectedPtBr,
recoveryPtBr, altPtBr, target? {x,y,width,height in native px},
availability (Disponível|Em breve|Planejado), claimClass, sourceFiles[],
capturedAt, privacyReviewed, demoOnly`.

Rules enforced by `landing/src/tutorial.ts` (`validateLesson`, pure, tested)
and by review:

- `target` rectangles come from inspected accessibility bounds or measured
  control boxes, never approximations. Result frames omit `target`.
- Every `target` must lie inside `nativeWidth × nativeHeight`; `stepId`
  unique per lesson; `imageSha256` must match the manifest master.
- Rendering is static HTML: full text and inline SVG overlays ship without
  JS. The SVG uses the image native `viewBox`, transparent fill, red
  `#df231b` non-scaling stroke, numbered badge beside (never over) the
  control. Shared styles live in `styles.css` ("Tutorial chapters").
- Previous/Next are plain anchor links between chapter pages/steps with
  equivalent readable text; they never call a mobile write API and never
  submit forms.
- Steps whose capture is pending ship `availability` Em breve/Planejado with
  a factual note, never an invented screen or synthetic success.

## IA freeze (LR02) — 2026-10-11

Hub `/como-usar/` groups chapters as driver/discovery (TU01–TU10),
accounts/participation (TU11–TU21), representatives/tools (TU22–TU28) and
help/offline (TU29–TU30). The parenthesized slug in each chapter heading is
the frozen route (`/como-usar/<slug>/`); never renumber accepted chapters,
add TU30a-style supplements only for newly evidenced controls. Home links to
the hub from `#tutoriais`; the hub links every chapter with step count and
`Disponível`/`Em breve`/`Planejado` status. No orphan lesson, no city doorway
page, no download CTA before a verified store listing.

## Conditional supplements and closure

Native provider/email legacy flows, PMQC UI, training consent, paid benefits and
new controls need fresh reachability/runtime evidence. Add stable TU30a-style
supplements as needed; never renumber accepted chapters. Backend PMQC data does
not prove a mobile control. iOS remains deferred without explicit resumption.

At LR25 record each route/control, source path, entry, TU/step, image/hash/build,
tested transition, availability, missing evidence and next owner. Every control
must be accounted for as actual or pending. That accounting never certifies a
complete live demonstration of unavailable functionality.
