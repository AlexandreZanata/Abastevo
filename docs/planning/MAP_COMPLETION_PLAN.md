# Map completion plan: official coverage, corrections, real offline map

Status: MC-00 and MC-A1 LOCAL_DONE. The SRC-ANP licence block was resolved
by maintainer decision D11 on 2026-10-27; Track A resumes at MC-A2. Tracks B
and C start now in parallel (see "Unblocked order" below). Base: `codex/map-completion` from
`codex/map-national-staging` `9036464d` (PR #162). Owner of the outcome: merge
every map PR into `main` after review.

Read first: `AGENTS.md`, `docs/planning/FAST_EXECUTION.md`,
`MAP_NATIONAL_STATIONS_PLAN.md` (tier ladder), `MAP_NATIONAL_FINAL_RECEIPT.md`
(current numbers), `docs/backend/evidence/maps/MAP_BATCH_INVENTORY.md`
(MAP-00..33 gates), `STATION_CATALOG_PLAN.md` (P27 suggestions).

## 0. Goal and honest ceiling

1. Every ANP-authorized reseller is in the catalog with all official
   information we may publish, and on the map whenever an official source gives
   a defensible point.
2. Any free account can suggest a correction from the app; abuse cannot
   damage official data, other users or the service.
3. The app shows the real base map of **Brazil only**, self-generated and
   downloaded on demand like CoMaps, at the best performance we can measure, draws stations with the two Abastevo SVG icons, keeps
   station points offline, and routes/guides to a station offline.
4. Staging VPS runs all of it; every MAP phase ends in an explicit state;
   every map PR is ready for the final merge into `main`.

Ceiling, stated up front: no official source gives a point for every station.
Current measured state: 40,388/45,688 registry stations pinned (88.4%); 5,300
have no open evidence. "100%" in this plan means **100% of the official
universe is accounted for**: each station is pinned from an official/open
source, or listed with a recorded reason and an open "help locate" request
that the correction flow (Track B) resolves through review. A station is never
pinned at a city centroid or an invented point to reach a number.

## 1. Decisions taken for this batch

| ID | Decision | Rationale |
| --- | --- | --- |
| D1, D2, D3 | Unchanged (ANP point = `anp-reported`; CNEFE exact number, species 6, level 1-3 = `address-point`; Overture evidence only). | Already applied and measured. |
| D4 | Add tier `street-segment`, the lowest pin rank (below `address-point`; existing ranks shift up by one): IBGE CNEFE level 4 (face de quadra) when the exact street and number range match. Labeled "Localização aproximada pela quadra (IBGE)". | Official IBGE geometry, block-face precision; reduces L5 without inventing points. Must stay visibly approximate. |
| D5 | ANP API points are validated against the IBGE municipal polygon (500 m buffer) and against shared-coordinate clusters; failures do not pin. | A wrong official point is worse than none. |
| D6 | Base map scope is **Brazil only**: generate MWM ourselves with the pinned CoMaps toolchain (MAP-08) for the 23 `Brazil_*` regions, plus `World` and `WorldCoasts` only as the low-zoom overview the engine needs. No neighbour or other-country packages. Base map regenerates monthly; station data stays weekly. | Self-generated packages keep ODbL attribution and our signature chain; Brazil-only keeps disk, generation time and egress small. |
| D10 | Map packages are static files: Caddy (or any mirror) serves them immutable with Range; the API serves only the signed manifest. | The app already verifies every package SHA-256 against the signed manifest before activation (MAP-15/MAP-24), so re-hashing ~86 MB per response in a 1-CPU API container is wasted capacity. |
| D7 | Station points ship offline as a signed package kind `station-overlay` per UF inside the map distribution. | One integrity chain (Ed25519) for map and points. |
| D8 | Corrections never overwrite official facts. They create community assertions; location corrections reach `reviewed` only by a recorded review decision. | Keeps official/community distinct (AGENTS rule 12). |
| D9 | Review happens through an operator CLI now and through the admin suite later (ADMIN_CONTRIBUTOR_PLAN A1/A4/B2). | Admin panel is not ready; the review port is built once. |

| D11 | SRC-ANP per-station facts are `MAINTAINER_ADMITTED`: verbatim, attributed, never republished as ANP files; basis Lei 9.610 art. 7º § 2º and art. 8º V, Decreto 8.777 art. 2º III. | Recorded in `MAP_EV_SOURCE_REGISTER.md`; optional Fala.BR confirmation does not block. |
| D12 | Universe: automotive (45,807) is the driver map; GLP (57,127) is catalog plus an off-by-default map layer pinned only by address tiers; TRR/aviation/marine are NOT_APPLICABLE. | Measured in MC-A1; no open source for the excluded classes. |
| D13 | ANP API politeness: ≤ 1 request per 20 s, honour `Retry-After`, stop at the first challenge, resume per-UF next run; CSV is the denominator. | The 1-second sweep hit a Cloudflare challenge after 17 UFs. |
| D14 | The private Paraná fixture is replaced by the self-generated Brazil packages from MC-C1 for device tests (pushed by script, never committed). | Removes the "admitted regional fixture is required" rows. |

If the user rejects D4 in chat, skip those tasks and record
NOT_PLANNED with the reason; nothing else depends on them.

## 2. Tracks and order

```text
MC-00 baseline
  ├─ Track A official data (Rust prep → Go import → VPS)     MC-A1..A7
  ├─ Track B corrections (backend → app)                     MC-B1..B7
  ├─ Track C real map (distribution → icons → offline → route) MC-C1..C8
  └─ Track D VPS deploy and operations                       MC-D1..D4
MC-E closure: phase states, P38 rows, PR consolidation, merge readiness
```

Unblocked order: A2→A7 continue; C1→C5 start in parallel (C1 needs no ANP
data); B1→B4 start on the existing directory fields (location, name, brand,
address, status) and pick up dossier fields when A5 lands. C6 waits for A7.
D deploys after each track's local gate. E runs last.

## 3. MC-00 Baseline

- Branch `codex/map-completion` from `9036464d`. Merge `origin/dev` if it moved.
- Fix the two route tests that fail on pure dev (recorded in
  `merged-head-device-matrix.txt`): `MapRouteInstrumentedTest` must accept the
  persistent host (assert the recenter control is not displayed instead of not
  existing), `NativeRouteScreenInstrumentedTest` must expand the compact panel
  before tapping `native-route-open`. Do not change product behavior to satisfy
  an old test; change the test only where the persistent-host/compact-panel
  design is the intended behavior (MAP-UI-ISOLATION / MAP-UI-REFINEMENT).
- Update `MAP_EV_PHASES.json` statuses from `MAP_BATCH_INVENTORY.md` (all
  phases still say PLANNED, which is stale).
- Done: both tests pass on the POCO; JSON reflects evidence.

## 4. Track A: official data, consolidated in Rust

All preparation is in `tools/station-prep` (Rust, `scripts/check-rust-fast.sh`
must stay green). Raw downloads live only on the VPS private root
`/opt/abastevo-temp/map-completion-<date>/` (mode 700) with SHA-256 manifests;
nothing raw enters Git.

### MC-A1 Official universe and source register
- Enumerate every endpoint of `https://revendedoresapi.anp.gov.br` (its
  OpenAPI/Swagger page) and every ANP open-data reseller dataset (automotive
  fuel, GLP, TRR, aviation if present). Record URL, licence, fields, update
  cadence, coordinate fields and SRID in `MAP_EV_SOURCE_REGISTER.md`.
- Define denominators per reseller kind (automotive fuel, GLP, other). The
  3,003 directory stations without UF (2,924 LPG-only) must land in a kind.
- Done: register updated; a table of kind → source → expected count.

### MC-A2 Harvest all kinds
- Extend the UF harvest (29 requests in 136 s for fuel) to every kind found
  in A1, same politeness: sequential, retry with backoff, stop on 429.
- Rust `prepare_anp_registry` parses raw pages into normalized NDJSON with a
  manifest (count, sha256, harvest time). Byte-identical on rerun.
- Done: counts per kind/UF; rerun identical; timing recorded.

### MC-A3 Point validation (D5)
- Add to `tools/station-prep`: IBGE municipal polygons (official malha
  municipal, simplified, licence recorded) and checks: point in its
  municipality (500 m buffer), lat/lon swap, zero/sentinel, ≥5 stations sharing
  one coordinate, distance to the CNEFE/address candidate. Each failure has a
  reason code; a failing ANP point falls to the next tier, never pins.
- Rule doc first: **B-BR-MAP-15 official point validation**.
- Tests: one per failure reason, plus a valid point; property test that
  validation never upgrades a tier.
- Done: report of points rejected per reason/UF.

### MC-A4 Street-segment tier (D4)
- **B-BR-MAP-16** first. Rust: CNEFE level 4 accepted only with exact street
  match and number inside the block-face range; label `street-segment`.
- Backend: append-only migration adding `street-segment` to the quality checks
  (same pattern as `000139`), `domain.PinRanks` renumbered so `street-segment` is the lowest pin rank (the SQL rank is generated from it),
  OpenAPI enum, `station-locate` D4 gate in the manifest (`D4_street_segment`).
- Android: `StationLocationQuality.STREET_SEGMENT` (approximate), string in the
  8 locales, overlay style = approximate.
- Tests: ladder (never replaces higher tiers), real PostGIS integration,
  manifest gate, Kotlin wire tolerance.

### MC-A5 Station dossier (all publishable information)
- Rust `prepare_station_dossier`: one record per CNPJ with field-level
  provenance: ANP (razão social, nome fantasia, bandeira, authorization number
  and date, address, products/kinds, any capacity field the API exposes),
  Receita CNPJ (situação cadastral, opening date, main CNAE), IBGE municipality,
  location tier and point, latest ANP price week.
- Exclude personal data: no partner (QSA) names, no CPF, no personal phone or
  e-mail. Record the exclusion list in the B-BR doc (**B-BR-MAP-17**).
- Go: append-only `directory_station_facts` revisions (migration), importer
  command `station-dossier` (manifest hash binding like `station-locate`,
  dry-run, ordered locking, idempotent rerun), `StationProfile` read model and
  OpenAPI fields with source and observed date per field.
- Tests: importer bad batches, concurrent batches, rerun unchanged, real
  PostGIS; read API contract tests.

### MC-A6 Residual accounting
- Every L5 station gets a reason (`missing_cnefe`, `missing_address`,
  `point_rejected:<reason>`, …) and a public flag `location_help_wanted`.
- Coverage report per UF and per kind: pinned by tier, residual by reason.
- Done: denominators add up exactly; report in the receipt.

### MC-A7 Weekly refresh pipeline
- A single orchestrated run: harvest → registry → validate → match → geocode →
  coverage → locate → dossier → overlay. Each step bounded, idempotent,
  manifest-chained, timed.
- Runs as a Kubernetes CronJob in `abastevo-temp` only, resource limits within
  the namespace quota, Monday 04:00 BRT, concurrency `Forbid`, failure leaves
  the previous release active.
- Done: two consecutive runs; the second applies zero changes when inputs are
  unchanged.

### MC-A8 EV and hybrid sources
- Admit, per `MAP_EV_SOURCE_REGISTER.md` "EV and hybrid driver sources":
  OSM chargers (pinned Brazil extract), OCM POIs whose provider licence is
  CC BY 4.0, Inmetro PBEV current edition, ANEEL distributor tariffs. Record
  URL, bytes, SHA-256, licence evidence; stop on unclear terms.
- Rust `prepare_chargers`: normalize OSM and OCM chargers, match within 50 m
  and same socket family, emit `confirmed_by` (1 or 2 sources), keep each
  source's attribution.
- Done: charger counts per UF by confirmation level; PBEV/tariff admission
  states.

Track A gate: measured coverage per kind, zero bad pins (outside Brazil, in
the wrong municipality, centroid), reruns idempotent, timings recorded.

## 5. Track B: "Sugerir correção" with abuse controls

Builds on P27-T01 (`/v1/stations/suggestions`, signed, idempotent, owner-only,
20/day). Corrections target an existing station UUID.

### MC-B1 Rules (before code)
Write **B-BR-COR-01..08** in `docs/product/STATION_CATALOG.md`:
correctable fields (location pin, display name, brand, address, operating
status, fuel products/services, EV connectors), what each correction can and
cannot change (D8), consensus and review outcomes, cooldowns, retention and
erasure.

### MC-B2 Storage
- Append-only migration: `station_corrections` (id, station_id, account_id,
  field, proposed value JSON validated per field, optional private evidence
  ref, client proximity bucket, idempotency key, status, reason codes,
  timestamps) and `station_correction_decisions` (append-only audit: decider
  kind system/staff, decider id, reason, evidence).
- Statuses: `pending`, `auto_verified`, `accepted`, `rejected`, `expired`,
  `cancelled`, `superseded`, `frozen`.

### MC-B3 API and review port
- `POST /v1/stations/{id}/corrections` (signed, idempotent),
  `GET /v1/stations/corrections/mine`, `GET …/{cid}/status`,
  `POST …/{cid}/cancel`. OpenAPI first.
- Review port (application interface): `ListPending(filter)`,
  `Decide(id, decision, reason)`. Exposed now by operator CLI
  `station-review` (list/decide, audit mandatory); the admin suite later binds
  the same port to admin HTTP routes. No admin UI in this batch.
- Accepted corrections: location → new `reviewed` revision through the
  existing ladder; other fields → community-layer fact shown with its own
  label, official field unchanged.

### MC-B4 Abuse model (each line needs a test)

| Abuse | Control |
| --- | --- |
| Anonymous or forged writes | Live session + device-key signature; anonymous reads only. |
| Replay / double submit | Idempotency key per account; same key, other body → conflict. |
| Flooding | Per account/day (existing 20), one open correction per account+station+field, per device key, per IP bucket at the API limiter; bounded queue per station. |
| Sybil / brigading | Consensus counts distinct accounts and distinct devices; new accounts weigh less; correlated submissions (same device/network/time window) count once; community consensus alone never changes official fields. |
| Location vandalism | Proposed point inside Brazil, inside the station municipality polygon (+buffer), max distance from the current point unless evidence; coarse proximity bucket only; no raw contributor GPS stored (P16). |
| Competitor sabotage ("closed") | Closed/permanently-closed never hides a station automatically; needs ANP revocation evidence or staff review. |
| Edit wars | Cooldown per station+field after a decision (e.g. 30 days) unless new evidence; flip-flop detection freezes the field (`frozen`) for review. |
| Spam / PII in text | Structured fields only; optional note ≤ 280 chars, normalized, URLs/e-mails/phones/CPF rejected, HTML escaped on every output. |
| Oversized / malformed payload | Strict JSON (unknown fields rejected), body cap, per-field validators. |
| Repeat offenders | Rejection ratio over a window lowers weight and can pause intake for that account (reversible, audited). Payment never affects this. |
| Privacy leaks | Owner-only reads; no public list of who suggested; account erasure deletes or anonymizes rows (P13). |
| Evidence photo abuse | P15 24-hour private media rules, EXIF stripped, no production photos in Git. |
| Staff misuse (future) | Every decision audited with reason; decisions are append-only. |

Tests: negative per row, concurrent submissions on one station, idempotent
replay, quota boundaries, revoked session, erasure, real PostGIS.

### MC-B5 Automatic verification
A correction matching official evidence (e.g. ANP point within 50 m of the
proposed pin, ANP name/brand equal) becomes `auto_verified` with that evidence
attached; everything else waits for review. Auto-verified location becomes
the corresponding official tier, never `reviewed`.

### MC-B6 Android
- "Sugerir correção" in the map station sheet and the station profile: pick
  the field, edit structured value (location by dragging a pin on the map),
  optional note, submit; offline outbox with replay (existing outbox pattern);
  "Minhas sugestões" status list; 8 locales; TalkBack labels.
- L5 stations show "Ajude a localizar este posto" leading to the same flow.
- Tests: ViewModel unit tests, outbox replay, instrumented form test on POCO.

### MC-B7 Admin hand-off
Document the review port contract and status model in
`ADMIN_CONTRIBUTOR_PLAN.md` (A4 staff roles, B2 UI) as an open dependency.

Track B gate: all abuse tests pass; one correction per field type submitted
from the POCO against staging, decided via `station-review`, visible
correctly in the app.

## 6. Track C: real map in the app

### MC-C1 Signed Brazil map distribution (D6)
- Generate MWM with the pinned toolchain (`generator_tool` from
  `comaps-4587b8ad`, MAP-08) on the local workstation (16 cores, 31 GB RAM),
  not on the shared VPS. Input: the Geofabrik Brazil extract only (ODbL,
  recorded) and CoMaps borders (`Brazil_*`: 23 regions); `World` and
  `WorldCoasts` overview packages from the same toolchain.
- Generate regions in parallel up to the measured RAM limit; record time,
  peak RAM and size per region.
- Build the `abastevo.map-distribution-v1` envelope (MAP-12) with a new
  **staging** Ed25519 key generated locally; private key kept outside Git and
  outside the VPS; public key pinned.
- Upload to `/opt/abastevo-temp/map-dist/<generation>/`. The API mounts it
  read-only (`MapDistributionDir`/`MapDistributionKeys`) for the manifest
  route only; package bytes are served by Caddy (D10) from the same
  generation directory: immutable `Cache-Control`, ETag, `Accept-Ranges`,
  `206` resume, no compression of already-compressed MWM. Keep the URL shape
  the app requests, or change the app downloader in the same task with tests.
- Prove on staging: manifest ETag/304, full download, interrupted download
  resumed with Range, tampered package refused by the app (digest check).

### MC-C2 Brazil-only coverage and performance budgets
- Outside Brazil the app shows only the `World` overview and the message
  "Mapa detalhado disponível apenas no Brasil"; no fake tiles, no download
  offer for other countries.
- Budgets (measure on the POCO and staging, record in MAP-29 evidence; a miss
  is a finding to fix or report, never a silent pass):
  - map tab cold open to first rendered frame and warm return (MAP-29
    declared budgets);
  - pan/zoom frame pacing over dense São Paulo pins (no sustained jank);
  - station pins for the viewport drawn from the offline overlay without
    network;
  - `/v1/stations/nearby` p95 ≤ 50 ms at 100 req/s for 60 s, 0 errors;
  - package downloads use zero API CPU (static serving) and resume after a
    cut;
  - offline route to a station inside a downloaded region computed on the
    device without network.

### MC-C3 App configuration
`mapDistBaseUrl=https://teste.abastevo.com.br`, region catalog and public
keys provided as Gradle properties for staging builds (public keys are not
secrets; still not hard-coded in source). Done 2026-10-28: root
`gradle.properties` carries `mapDistBaseUrl` and the `staging-20261027`
public key so every build, release included, can download regions; `-P`
overrides both.

### MC-C4 On-demand download like CoMaps
- Region lookup from bundled Brazil border polygons (`packed_polygons`,
  `Brazil_*` only): on first
  location fix, offer the user's region; when the viewport enters an
  undownloaded region, show "Baixar <região> (N MB)"; honour the Wi-Fi
  preference (MAP-24); updates and deletion as in MAP-24.
- Tests: region lookup unit tests (borders, edges, ocean), ViewModel prompts,
  instrumented download against staging.

### MC-C5 Station icons on the map
- Use `docs/assets/map-icons/ic_map_station_combustion.svg` and
  `ic_map_station_electric.svg` (today only in the accessible list) as the
  native marker glyphs, plus the combined fuel+EV glyph (closes MAP-18
  PARTIAL). Approximate tiers use the same glyph with the approximate style.
- This changes the engine skin (`symbols.sdf`/drules), so rebuild the AAR with
  the MAP-14 reproducible procedure and rerun ENGINE_GATE (16 KB pages, ABIs,
  size budget).
- Done: screenshots of fuel, electric, combined and approximate markers in
  light/dark on the POCO.

### MC-C6 Offline station points (D7)
- `mapoverlay` output (per UF `.json.gz`) becomes a `station-overlay` package
  in the signed distribution; the app downloads it with the region, stores it
  in Room, renders pins from it with no network, and refreshes by release.
- Online `/v1/stations/nearby` stays the fresh path; offline shows the
  overlay week and an "offline" label.
- Tests: package verification, Room import idempotence, airplane-mode render.

### MC-C7 Route to the station, CoMaps-style
- Station sheet "Rota" → native route from current location to the station
  pin (MAP-22) → preview with distance/time → "Iniciar" → guidance (MAP-23):
  follow-me camera centred and rotated with heading, voice prompts, rerouting
  off-route, works offline when the region is downloaded; missing region →
  honest refusal with download offer.
- Tests: instrumented route+guidance with a mock location provider replaying
  a recorded path; airplane-mode run.

### MC-C8 Device acceptance on the POCO
Airplane-mode journey: download Mato Grosso + station overlay → airplane on →
open map at Sorriso → icons render → tap station → route → guidance replay →
arrive. Video or screenshot sequence plus timings. Repeat online at São Paulo
and Manaus.

Validation 2026-10-28 (branch
`codex/phase-map-regional-integration`, physical device, staging
`https://teste.abastevo.com.br`): fresh user → SORRISO (MT) → Map shows the
regional card "Download the Mato Grosso map (116 MB)" only → Download
completes → `files/261004` holds `World.mwm` (53,519,450),
`WorldCoasts.mwm` (8,489,900) and `Brazil_Mato Grosso.mwm` (53,816,563);
Downloads shows "Active: 20261028.1-brazil-mato-grosso", "Published". Pan/zoom
hold 30 s with no snap-back (overlay commits no longer reframe). The "renders
nothing" observation was wrong: the camera stayed at the Curitiba default
(startup race plus a nearby refresh cancelling the city load), and an
activated region was not redrawn until restart. After 5b3ff0b6, 91a00c21,
c220a4bb and 58186a2e, a fresh release install (R8, test-key signed) at
SORRISO shows the Mato Grosso card, downloads it and renders the generated
`Brazil_Mato Grosso.mwm` streets without restarting; the size gap to the
official table (no search/extra sections) does not affect drawing.
Screenshots are kept outside the repo. Open: airplane-mode leg, tap station
→ route, and São Paulo/Manaus online runs.

## 7. Track D: VPS (namespace `abastevo-temp` only)

- MC-D1 Before each migration: `pg_dump` backup with sha256, active map
  generation and overlay symlink recorded (same as 2026-10-27).
- MC-D2 Deploy: build images locally (`infra/docker/Dockerfile`, targets
  api/worker/migrate), `docker save`, import with `k3s ctr`, migrate Job,
  `kubectl set image`, rollout, `/health/ready` 200.
- MC-D3 Mount the map distribution, enable the CronJob (A7), and run the smoke
  set: nearby SP/Manaus/Sorriso, corrections submit/decide, manifest + Range,
  overlay package, static package Range/resume; k6 100 req/s for 60 s with
  0 failures.
- MC-D4 Rollback recipe tested once per track (restore images, generation,
  symlink); never touch other namespaces or other projects' files.

## 8. MC-E closure and PR consolidation

1. Set every MAP-00..33 phase to a real state (LOCAL_DONE, STAGING_DONE,
   EXTERNAL_PENDING, NOT_PLANNED with reason) in `MAP_EV_PHASES.json` and
   `MAP_BATCH_INVENTORY.md`. Remaining honest externals: MAP-30 car head unit,
   long field drive (MAP-23), API 26/ARMv7 hardware, two-device cloud restore
   (MAP-25).
2. P38: rerun the matrix on the final head; keep BLOCKED_MEDIA rows as they are.
3. PR order (no merge without required checks and user approval):
   - #157 (P38 test fixes) → `dev`
   - #162 (national map) → `dev`
   - map-completion PR → `dev`
   - #154 `dev` → `main` (cumulative)
   - #155 and #156: close as superseded with a comment linking the merged PRs,
     only after their commits are in `main`.
4. Final receipt `MAP_COMPLETION_RECEIPT.md`, PROGRESS entry, wiki sync after
   the `main` merge.

## 9. Test commands (every task)

```bash
bash scripts/check-rust-fast.sh
cd backend && go build ./... && go vet ./... && go test -count=1 ./...
cd backend && ANPFUEL_TEST_DATABASE_URL=<local disposable PostGIS> go test -count=1 -tags=integration ./internal/modules/directory/... ./internal/modules/mapdata/...
./gradlew --offline :domain:test :maps:engine:testDebugUnitTest :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
ANDROID_SERIAL=VKCQGAUCGYAMR4W4 ./gradlew --offline :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=<changed classes>
git diff --check
```

## 10. Stop conditions

Stop and report instead of working around when: a source licence is unclear;
a check needs a secret or credential not already provisioned; a test fails
and the fix would weaken it; an action would touch another namespace,
project, the main checkout or `main`; free VPS disk would drop below 100 GB;
an engine rebuild fails ENGINE_GATE; a migration cannot be made append-only.

## Coordination notes (2026-10-28, from the admin-suite/CD session)

The owner kept this plan's key policy: the map-distribution signing key stays
local, off the VPS; only public keys go to the app build and the API.

- **Branch synced with `main`** (`3c86c898`). `main` already holds #162/#156/#155/#157
  (via #177) and migrations `000139`–`000143`. Because `main` used `000140`
  for admin sessions, which is already applied on staging, MC-B2's
  `station_corrections` moved to **`000144`**. It was never applied on any
  real database.
- **Staging deploys automatically from `main`.** A systemd timer runs the
  deploy agent every 2 minutes. It pulls the signed release, then runs
  backup → migrations → api/worker image rollout → landing switch → smoke.
  - The agent changes only images and the landing. Map-distribution env vars
    (`ANPFUEL_MAP_DIST_DIR`, `ANPFUEL_MAP_DIST_PUBLIC_KEYS`) and any hostPath
    mount must also go into `infra/k8s/staging/api-deployment.yaml`, or the
    R2 zero-diff check reports drift. Apply them by hand.
  - Never place map-distribution files under the landing root; every deploy
    switches `current` to a new release tree.
  - For D10 static serving, start from `main`'s
    `infra/caddy/staging-web.Caddyfile` and its configmap mirror. They carry a
    `handle /admin/*` block that must stay. The configmap is applied by hand
    (`kubectl apply` plus a web rollout restart); the agent does not apply it.
- **Delivery:** deliver this stage as one PR to `main`
  (`DELIVERY_WORKFLOW.md`, "One PR per planning stage").
- **Fixed on 2026-10-28:**
  - The staging upload `brazil-mato-grosso-20261027/WorldCoasts.mwm` was
    corrupt (7,311,360 B). It was replaced with the verified file
    (8,489,900 B, sha `d42a71b1…`); all three packages now match
    `manifest.json`, and the corrupt copy is quarantined under `/root` on the
    host. The local MAP-12 test seed `key.b64` is now mode 600.
- **Owner decisions still open:**
  - ODbL redistribution of World/WorldCoasts.
  - The API serves one region per process (several regions need code or one
    national region).
  - Region names with spaces are rejected by the Go/Kotlin path rules.
