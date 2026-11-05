# Store privacy policy and release alignment

Status: LOCAL_DONE
Validation: PASS

## Scope and provenance

The maintainer requested a comprehensive static HTML privacy policy for the
Abastevo Android launch and future Apple publication, grounded in actual data
flows and current LGPD/store requirements. This task does not submit to a store,
activate future data processing, merge PR #195, or resume native iOS work.
Opening baseline: main 972df4f4; isolated codex/store-privacy-policy preserves occupied dev
and other worktrees. The maintainer reconfirmed ZAFLAS INOVA SIMPLES (I.S.),
CNPJ 67.852.667/0001-93, the official privacy WhatsApp (66) 9631-4240, and Brazil
for the intended production hosting. During construction the remote main moved
to f7bec13f through PR #195, verified read-only; this task did not perform that
merge or override the earlier hold. Final validation must use the reconciled
main. Do not invent an email, append a telephone
digit, name an unknown hosting provider, or infer that every third-party service
processes only in Brazil. No message is sent to validate the contact.

## Behavior contract

B-BR-STOREPRIV-01 / BUC-STOREPRIV-01: The public Portuguese privacy HTML keeps the
canonical home header/footer and static build, is readable without JavaScript,
uses navigable section anchors and real controller/contact details, and covers
purposes, data categories, lawful bases, local/server/public distinctions,
recipients, permissions, international transfers, retention/deletion, rights,
children, security/incidents, future changes and policy version. No search API,
login, PDF-only notice, tracking script or general advance consent is introduced.

B-BR-STOREPRIV-02: State the actual Android account/community/driver/quality/map
flows, precise-location foreground navigation and optional track recording,
local/offline storage, optional private sync, on-device OCR and Google ML Kit
metrics. Public facts and pseudonymous identities are distinct. Never label
pseudonymous IDs as anonymous or claim there is no backend/personal-data transfer.
Operational photo access/expiry, training collection OFF and every-copy physical
purge acceptance are distinct; planned controls are not certified runtime facts.

B-BR-STOREPRIV-03 / BUC-STOREPRIV-02: Account deletion instructions use the actual
in-app path and a discoverable private outside-app contact route. Do not send
rights requests, usernames, credentials or documents to public GitHub issues.
Explain proportional identity verification, receipt versus completed purge,
retained legal/security records and local/device/backup limitations. Do not
promise immediate erasure of all copies or unsupported fixed retention periods.

B-BR-STOREPRIV-04: Link the complete HTML policy and external deletion route from
Android's existing Privacy screen. Preserve its consent controls and account
removal path. Links do not themselves grant consent or submit a deletion request.
If no browser can handle the link, show a recoverable error and the exact URL
instead of crashing; no request is submitted. No author-credit strip, permission
expansion or native iOS implementation.

B-BR-STOREPRIV-05: Keep a source-backed release checklist for Google Data safety,
Apple App Privacy/manifests/required-reason APIs, SDK declarations, disclosures,
delete/export tests, regions/processors, controller/DPO governance and production
retention/backup proof. A policy edit cannot certify production or guarantee
store approval. Future features require a new inventory/policy/store declaration
and specific permission/consent where applicable before activation.

## Initial findings (not a compliance verdict)

- The live HTML has five brief sections and a global 24-hour physical-erasure
  statement unsupported by complete production acceptance. The external deletion
  page routes requests to public GitHub and promises immediate erasure.
- docs/privacy-policy.md is a June 2026 no-backend/no-account/no-personal-data
  policy, contradicted by the current account/community/private-ledger code.
- Android declares foreground location, camera and notification permissions;
  GuidanceService, TripRecordingService and DriverGpsService use explicit
  foreground GPS. MAP-25 stores recordings locally; its cloud trip backup is
  explicitly not delivered. LedgerSyncHttpClient sends private refuel/expense/
  charge records when the corresponding optional sync is enabled.
- ML Kit text recognition 19.0.1 and Play Services base 18.11.0 are dependencies.
  Google's documentation states on-device image/text processing separately from
  SDK device/app/install-ID/performance diagnostics sent to Google.
- NominatimReverseGeocodeClient sends latitude/longitude to OSMF when the user
  requests city resolution. Native routing/voice/recording are separate flows.
- Optional image-training terms say collection/selection are OFF; changing this
  policy does not enable them. Raw/sanitized photo expiry is distinct from local
  vehicle pictures, saved tracks, signed claim PDFs and private finance records.
- Encarregado appointment/exemption, exact hosting/backups/processors, real rights
  handling, production artifact traffic and complete retention/restore acceptance
  need verification; company identity and Brazil alone do not establish them.

## Source inventory and disclosure decisions

Reviewed against main 972df4f4 and this task's working tree, not a future store
binary. Source presence is not proof that a deployed feature is enabled.

- `app/src/main/AndroidManifest.xml`: camera, coarse/fine location, notifications
  and location foreground services. The owner-signed reference APK from the held
  release work has no ACCESS_BACKGROUND_LOCATION, broad media-library, contacts,
  microphone or AD_ID permission. It does contain Play Services/dependency
  permissions, including ACCESS_LOCATION_EXTRA_COMMANDS. Permission absence does
  not establish absence of collection; re-inspect the final AAB-generated APK.
- `data/.../remote/NominatimReverseGeocodeClient.kt` sends latitude/longitude to
  Nominatim when requested. Local CoMaps routing/voice is distinct from this
  geocoding request. Explicit navigation, recording and driver GPS foreground
  services can continue under a notification while the app is minimized/locked.
  Do not describe all GPS as a single one-time lookup or hidden background GPS.
- `data/.../local/preferences/LedgerSyncFlagStore.kt` defaults private sync OFF;
  eligible financial commands can leave the device after it is enabled.
  `docs/backend/evidence/maps/MAP-25.md` explicitly leaves cloud track backup
  undelivered. Favorites' backend capability does not establish app sync wiring.
- `data/.../local/media/VehiclePhotoStore.kt` stores vehicle JPEGs in filesDir.
  `app/src/main/res/xml/{backup_rules,data_extraction_rules}.xml` excludes
  databases and shared preferences, but not all filesDir contents. Consequently
  eligible vehicle images can participate in OS backup/device transfer. The
  policy discloses this; neither "all data stays on-device" nor "all files are
  encrypted/excluded from backup" is supported. A future exclusion change needs
  its own restore/transfer regression, not an undocumented claim here.
- `data/build.gradle.kts` uses ML Kit text recognition 19.0.1 and Play Services
  base 18.11.0. ML Kit input/output processing is on-device; Google's Android
  disclosure separately lists device/app information, identifiers and operational
  metrics. No Firebase Analytics/Crashlytics/Sentry/advertising SDK was found in
  the inspected dependency declarations. This is not a network-traffic audit.
- `backend/internal/modules/stationprofile/domain/proof.go`: scan evidence has a
  24-hour policy; authorization documents have a default two-year policy. Do not
  apply ordinary photo lifetime to every PDF, vehicle photo or private record.
- Account/session validity, cleanup grace, temporary photo access deadlines,
  generated export download windows and completed physical erasure are distinct.
  `backend/internal/modules/privacy/application` contains their owning operations;
  no backend policy, SQL, deletion job or consent digest is changed by this task.
- `backend/internal/modules/payments/adapters/{stripe,asaas}.go`: Stripe checkout
  shares account/order/offer references; Asaas customer creation can send name,
  CPF/CNPJ and optional email. Conditional billing disclosure covers these flows
  without enabling sales or claiming all payment methods are active. Card details
  go to the processor's checkout. Store purchase-channel eligibility is separate.
- The immutable optional training terms state collection/selection OFF. The full
  policy does not alter those terms, their digest or the user's consent record.
- Static site/search remain static files. HTTP requests still expose connection
  information and a search term placed in a URL may appear in browser/server
  history. No advertising cookie banner or tracking script is added.
- Public ANP/RFB/OSM records and CNPJs can identify natural persons/sole traders;
  public provenance does not remove minimization and data-subject obligations.

The read-only staging probe inspected selected inline deployment flags but found
configuration supplied through envFrom; it did not resolve those sources.
Therefore no conclusion about active sales, driver, media or quality flags is
claimed. Do not publish environment dumps or credentials to complete this audit.

## Publication requirements — Google Play

Official sources checked 2026-11-05. These are actionable release checks, not
confirmation that the Play Console has been configured.

1. Publish the final policy at the active, public, non-geoblocked HTTPS HTML URL
   `https://teste.abastevo.com.br/privacidade/`. It must work without login, an
   app install or a PDF reader. Match the app/developer identity, controller and
   active contact in policy, listing and Console. Keep the document accessible
   from inside the app; this slice adds explicit full-policy/deletion links.
2. Set that exact URL in Play Console's Privacy Policy field. A link alone is not
   the Data safety form. Review all active versions/tracks, SDKs and server-side
   processing, including optional processing, against the final artifact/profile.
3. Supply `https://teste.abastevo.com.br/excluir-conta/` as the outside-app
   account/data deletion resource. Preserve the app's direct self-service path:
   Profile → Manage account → Delete my account → confirmation. Do not replace
   deletion with logout/deactivation, require reinstall, send requests to public
   GitHub, or require support for the existing in-app path. The web contact path
   needs real private receipt, ownership verification and completion handling.
4. For Data safety, determine collection, sharing, purposes, required/optional,
   ephemeral processing, linkage, transit encryption and deletion per data type.
   Do not label a stable pseudonymous ID anonymous, all SDK traffic optional,
   persistent logs ephemeral, or the whole app "no data collected" because
   images are processed on-device. Processor/user-directed exceptions to the
   store's sharing definition need actual contractual/flow evidence; LGPD
   disclosure of recipients is still required.
5. Before unexpected personal/sensitive processing, present a prominent in-app
   disclosure of data, purpose and use (including while minimized when relevant),
   then affirmative consent and the appropriate OS permission. A long policy,
   installation, Android permission alone or continuing use cannot substitute.
   Test denied/revoked location, approximate-only, denied camera/notifications,
   explicit start/stop, and manual-city/public-read alternatives. Foreground
   location services are distinct from ACCESS_BACKGROUND_LOCATION; inspect the
   merged release manifest and Console declarations, not only source XML.
6. Complete target audience/content rating, permissions/foreground-service and
   app-access review declarations from real behavior. Provide safe reviewer
   access/instructions without a real person's private history. If paid digital
   features are offered, verify the applicable Play payment policy/program and
   storefront before enabling the current external checkout. This task does not
   decide eligibility or enable any sale.

### Candidate Data safety inventory — verify before submission

This list maps implementation to likely form categories. It is deliberately not
an import file or a claim that unpublished/deferred features are collecting data.

- Personal information: username/user/account IDs; email and provider identifiers
  when the chosen authentication method collects them. Billing, claims and
  user-authored support content can additionally contain name, CPF/CNPJ, address
  or contact details. Audit each actually offered flow, not just default signup.
- Location: approximate city/map area and precise coordinates for the geocoding
  and enabled contribution flows that transmit them. Local-only navigation and
  unuploaded recordings are distinct. Check whether requests/IDs link location
  to an account and whether any recipient/log retains it.
- Photos: evidence actually uploaded; other user-generated content includes
  price contributions, descriptions, corrections, quality reports and claim
  documents when offered. An EXIF scrub does not remove visible personal data.
  Local vehicle images are distinct, including the OS-backup caveat above.
- Financial information: private expense/refuel/charge records when optional sync
  sends them, plus actual purchase/subscription/payment history and information
  collected by a configured payment flow. Do not treat locally calculated totals
  as server collection unless the flow actually transmits them.
- App activity: contributions, votes/reports, settings or interactions received
  by the service/SDK. Consider searches only where transmitted, not every local
  keystroke or offline route.
- App information/performance and device or other IDs: ML Kit/Play Services
  diagnostics, installation/device identifiers and operational metrics per the
  SDK disclosure, plus actual first-party technical records. Absence of AD_ID is
  not a reason to omit these categories.
- For each entry record recipient, endpoint, purpose (functionality, account,
  safety/fraud/compliance or supported diagnostics), optionality, retention and
  deletion. Validate with fresh-install/anonymous/authenticated/permission-denied
  traffic and a release dependency inventory. Do not declare advertising or
  cross-app tracking without evidence, nor automatically declare them absent.

## Publication requirements — future Apple release

Native iOS implementation remains deferred. Before a separate iOS launch:

1. Link the public policy in App Store Connect metadata and prominently in the
   app; align controller, data flows, rights, retention and deletion with the
   actual iOS build rather than copying Android SDK disclosures as if identical.
2. Fill App Privacy for first-party and integrated third-party collection,
   purposes, data linked to identity and tracking using Apple's definitions.
   Local-only data and actual collection are different. Audit every SDK and web
   checkout; do not select Data Not Collected from a policy statement alone.
3. Implement and test direct in-app account deletion. A non-regulated app cannot
   force a call/email/chat to finish the in-app path. If Sign in with Apple is
   offered, test provider token revocation as part of deletion, as well as backend
   sessions, personal records, failures and local state. An external request page
   supplements this behavior and cannot replace it.
4. Inspect the final dependency set for required SDK privacy manifests/signatures
   and required-reason API declarations. Add accurate camera/location/photo and
   other purpose strings only for functions actually used. Permission denial must
   preserve appropriate alternatives. Required manifests are not replaced by a
   web notice, and are not proof of runtime behavior on their own.
5. Request App Tracking Transparency only if the actual build performs Apple's
   defined tracking. No IDFA/ATT assumption follows from Android permissions or
   the presence of diagnostics. Future sharing with third-party AI needs the
   specific advance disclosure/explicit permission required by guideline 5.1.2;
   this policy grants no advance blanket consent.
6. Verify age/audience, location/background modes, SDK requirements, subscriptions
   and the applicable purchase rules with that release's storefront and features.
   No iOS build, App Store form, reviewer submission or acceptance is claimed here.

## LGPD and operational obligations before real launch

Owner/controller actions and real production evidence remain OPEN. They are not
made compliant merely by being described in HTML.

- Confirm an appointed encarregado's public identity/contact, or document the
  legally applicable exemption and responsible service channel. Do not assume
  that the Inova Simples legal form alone qualifies for every small-agent rule.
  The maintainer's official number is confirmed; private receipt/response,
  accessibility, recovery and identity-verification practice were not exercised.
- Maintain a processing register: purpose, minimum fields, lawful basis, subjects,
  recipients, locations, controls and lifetime per operation. Assess legitimate
  interests before relying on them; assess high-risk location/financial/minor/AI
  operations and the need for an impact assessment with qualified legal support.
- Identify contracted hosting/DB/object storage/backup and support operators,
  actual primary/replica/off-host regions, subprocessors and access controls.
  Brazil is the owner-declared intended main production region, not a measured
  current VPS country or a promise of Brazil-only Google/OSMF/WhatsApp processing.
  Record valid international-transfer mechanisms under LGPD/ANPD Res. 19/2024;
  an external provider's generic policy or foreign clauses alone do not prove
  the controller's applicable Brazilian mechanism and transparency obligations.
- Prove account deletion/export against the exact production candidate: valid
  owner, invalid/expired proof, another owner's denial, retries, concurrency,
  provider failure, disconnected device and legal-retention exceptions. Inspect
  media, replicas, S3 versions, derivatives, caches and restored backups. A receipt
  is not completion. Report partial failures privately and prevent erased data
  from returning to normal use after restore.
- The code's photo access window is 24 hours; all-copy physical purge/queue/storage
  acceptance remains owed. Export access is 24 hours after readiness; access
  expiry is not a universal erasure deadline. Authorization documents, private
  ledger records and saved tracks have different purposes/lifetimes. Observation
  horizon metrics do not prove timed deletion of every observation.
- `infra/scripts/backup.sh` defaults to seven daily/four weekly copies and prunes
  whole-day ages greater than 35. This is source policy, not verified scheduling,
  off-host region, encryption, boundary timing or restore-erasure evidence. Do
  not advertise a hard "every copy gone by exactly 35 days" SLA from this code.
  Verify and publish actual environment retention/criteria before launch; stop
  indefinite failed-job retention and maintain an erasure replay ledger safely.
- Assess applicability of Marco Civil art. 15: the application access IP/time
  records of an organized commercial provider can require six-month confidential
  retention. A 14-day diagnostic log setting/truncated IP is not evidence that
  this separate duty is met. Determine the minimal legally required record and
  controls with counsel; do not indiscriminately extend photos/GPS/all logs.
- Establish rights handling, response records and escalation. LGPD art. 19's
  simplified immediate/full 15-day confirmation/access rule is not a universal
  15-day deletion promise. Explain any justified refusal/conservation and enable
  correction, access, revocation and review of solely automated decisions.
- Document incident detection/response, processors' notice duties, ANPD/titular
  notifications and Res. 15/2024 timing (generally three business days for the
  relevant risk, subject to applicable legal rules). Do not infer an exemption
  from size without assessing eligibility. Maintain evidence with restricted
  access, not public incident payloads containing personal data.
- Set a truthful audience and assess LGPD art. 14/ECA Digital obligations for
  likely child/adolescent access, not only child-directed marketing. There is no
  delivered universal 18+ verification gate to claim. Assess applicable consent,
  age assurance, best-interest safeguards and reporting without collecting
  excessive identity data or inventing a future SDK.
- Set actual distribution territories and assess additional local obligations
  before expansion (for example, an EU launch is not certified by this LGPD
  inventory). Keep store age/audience and controller metadata consistent.
- Reconcile Terms, app's short notices, immutable optional consent terms and
  store labels with the selected release profile. Keep training collection OFF.
  A new SDK/purpose/platform requires review, new notice and specific consent
  when applicable before activation; policy publication is not advance consent.

## Adjacent Android packaging blockers found

The request concerns privacy, not a target-SDK migration or a replacement release
pipeline. Nevertheless, the intended first Play publication cannot use the APK
pipeline as sufficient proof of store readiness:

- `gradle/libs.versions.toml` currently sets compileSdk/targetSdk 35. Google's
  current official schedule requires new phone apps/updates to target API 36
  starting 2026-08-31 (extension deadline 2026-11-01). Prepare a separately tested
  migration; changing the number alone is not migration acceptance.
- New Google Play apps require an Android App Bundle (AAB), with Play App Signing
  and the correct signing/upload-key continuity. PR #195
  prepares a staging APK release, not an accepted first Play submission/AAB.
  It was held at opening and later found already merged by a separate operation;
  this task does not undo that merge, dispatch releases or alter its signing key.
- Validate the exact store-delivered native libraries, 16 KiB compatibility,
  permissions, endpoints/production profile and data disclosures after packaging.
  These are release requirements, not claims this HTML change has satisfied them.

## Official references

Checked 2026-11-05; recheck at submission. These sources define requirements;
implementation/runtime conclusions above come from this repository, not from a
provider's generic marketing statement.

- [Google User Data / Privacy Policy](https://support.google.com/googleplay/android-developer/answer/10144311?hl=pt-BR)
- [Google Data safety definitions and declarations](https://support.google.com/googleplay/android-developer/answer/10787469?hl=pt-BR)
- [Google account/data deletion](https://support.google.com/googleplay/android-developer/answer/13327111?hl=pt-BR)
- [Google target API schedule](https://support.google.com/googleplay/android-developer/answer/11926878?hl=en)
- [Android App Bundle publishing format](https://developer.android.com/guide/app-bundle)
- [Android runtime permissions](https://developer.android.com/training/permissions/requesting)
- [Apple App Review Guidelines, especially 5.1](https://developer.apple.com/app-store/review/guidelines/)
- [Apple App Privacy definitions](https://developer.apple.com/app-store/app-privacy-details/)
- [Apple account deletion](https://developer.apple.com/help/app-review/guideline-reference/5-1-1-account-deletion)
- [Apple SDK requirements](https://developer.apple.com/support/third-party-SDK-requirements/)
- [Apple required-reason APIs](https://developer.apple.com/documentation/bundleresources/describing-use-of-required-reason-api)
- [LGPD, official updated text](https://www2.camara.leg.br/legin/fed/lei/2018/lei-13709-14-agosto-2018-787077-normaatualizada-pl.html)
- [ANPD data-subject rights](https://www.gov.br/anpd/pt-br/assuntos/titular-de-dados/direito-dos-titulares)
- [ANPD international transfers, Res. 19/2024](https://www.gov.br/anpd/pt-br/acesso-a-informacao/institucional/atos-normativos/regulamentacoes_anpd/resolucao-cd-anpd-no-19-de-23-de-agosto-de-2024)
- [ANPD incident guidance, Res. 15/2024](https://www.gov.br/anpd/pt-br/canais_atendimento/agente-de-tratamento/comunicado-de-incidente-de-seguranca-cis)
- [ANPD encarregado guidance, Res. 18/2024](https://www.gov.br/anpd/pt-br/centrais-de-conteudo/materiais-educativos-e-publicacoes/copy_of_guia_da_atuacao_do_encarregado_anpd.pdf)
- [ANPD ECA Digital](https://www.gov.br/anpd/pt-br/assuntos/eca-digital/)
- [Marco Civil official text, art. 15](https://www.gov.br/mj/pt-br/assuntos/sua-protecao/sedigi/LEIN12.965DE23DEABRILDE2014.pdf)
- [ML Kit terms and on-device processing](https://developers.google.com/ml-kit/terms)
- [ML Kit Android technical-data disclosure](https://developers.google.com/ml-kit/android-data-disclosure)
- [OSMF privacy](https://osmfoundation.org/wiki/Privacy_Policy)
- [Nominatim usage](https://operations.osmfoundation.org/policies/nominatim/)
- [WhatsApp privacy](https://www.whatsapp.com/legal/privacy-policy)
- [Stripe privacy](https://stripe.com/br/privacy)
- [Asaas privacy](https://central.ajuda.asaas.com/hc/pt-br/articles/32098003163035-Pol%C3%ADtica-de-Privacidade)

## Local acceptance and delivery state

Opening-baseline source acceptance (972df4f4 plus this task):

- Real RED: the three new static notice/deletion tests failed against the old
  pages; GREEN: 3/3 pass. The browser-unavailable device case first failed with
  the uncaught Android URI exception; after recovery handling all 3 device cases
  pass, including normal/light, 200% font/dark, explicit-click-only exact URLs,
  absent browser, changed destination and successful retry clearing the error.
- `npm --prefix landing run typecheck`, `npm --prefix landing test` (81/81),
  `npm --prefix landing run build` and `npm --prefix landing run check` pass.
  A sandbox-only subprocess EPERM was rerun with the required process permission;
  no test or compiler restriction was weakened.
- `./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug
  --no-daemon --max-workers=1` passes. Lint reports zero errors (326 existing
  warnings, two informational entries). The final test-only assertion/retry
  update was recompiled and run with AndroidJUnitRunner on the isolated API-30
  emulator, package `com.anpfuel.app.debug.test`: OK (3 tests).
- Browser QA on built static files: 320/390/1440px without horizontal overflow;
  shared home header/footer, full static text, 13 section targets, native anchor
  navigation, touch actions and private outside-app route inspected. The 390px
  policy actions measure 301x52px. No WhatsApp message/deletion was sent.
- `bash scripts/check-mobile.sh --static-only`, local documentation link checks,
  `bash scripts/scan-secrets.sh` and `git diff --check` pass. No permission,
  dependency, auth/SQL/job/backend implementation changed; this is not a fresh
  backend deletion/production acceptance campaign.

Reconciliation with newly observed main f7bec13f and its affected final checks
must be recorded before publication. No remote policy deployment,
store form submission, production certification or merge is implied. PR #195
remains an independent change; its externally completed merge is not attributed
to this task. Rollback is a normal reviewed revert of the static
notice/link commit; no database or key migration is introduced.
