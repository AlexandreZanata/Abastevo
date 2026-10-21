# Full electric and hybrid vehicle data support contract

Date: 2026-10-20. State: PLANNED / NOT_IMPLEMENTED_BY_THIS_AMENDMENT.
Owner: [EE execution](../planning/EV_FULL_SUPPORT_EXECUTION_PLAN.md), integrated
with [MEX](../planning/MOBILE_EXPANSION_EXECUTION_PLAN.md).
Sources/access limits: [research register](../planning/EV_SOURCE_RESEARCH.md).
Math: [EV financial method](EV_FINANCIAL_METHOD.md).

## Meaning of complete support

The requested target is complete product support for declared BEV, PHEV and
non-plug-in hybrid journeys across Rust preparation, backend, Android local data,
community prices and private costs/sync. It is not guaranteed coverage of every
Brazilian charger/model, every live connector or every private operator's tariff.
Unknown input/support/data is explicit, not a fabricated answer or silent fallback.
Source/API rights, real operator access and runtime/device proofs have separate
gates. Do not call core journeys complete while a required local path is missing.

This adds energy/propulsion support without replacing existing MEX/PRM/FQ/OPT,
ANP fuel models or source history. Public catalog/compatible search/community stay
free. One vehicle free and Premium for additional vehicles applies per physical
vehicle, including PHEV, not per fuel/energy carrier. Retain existing source
ceiling, legacy data and PRM professional/financial/cloud capability matrix.

## Business rules

- **B-BR-EV-01 — Propulsion and capabilities:** represent ICE, BEV, PHEV, HEV/MHEV
  explicitly; range-extender/other variants retain source wording and actual
  charging/fuel capabilities rather than guessed equivalence. A BEV need not
  have a liquid tank or liquid FuelProduct. A PHEV can hold fuel plus battery;
  HEV/MHEV must not be offered external charging unless explicitly supported.
  Unknown capabilities remain editable/unknown, not forced to a fake default.
- **B-BR-EV-02 — Retained identity:** keep vehicle IDs, fuel enums and historical
  persisted meanings through append-only migration. Add propulsion/carriers and
  optional capacities; never reinterpret a historical premium-gasoline value,
  assign an EV to all old rows or invent battery size on migration.
- **B-BR-EV-03 — Specifications:** model market/year/trim and source edition;
  gross versus usable battery capacity, connector support and AC/DC limits are
  distinct optional facts. PBE laboratory reference, manufacturer assertion,
  manual estimate and user-measured history remain separate. Corrections retain
  lineage and do not silently change old calculations or every vehicle model.
- **B-BR-EV-04 — Exact units:** energy Wh, power W, elapsed seconds, SOC with
  declared precision/basis, distance metres and money in centavos (actual total)
  have separate types. Rates use frozen exact decimal/rational semantics shared
  by Rust/Go/Kotlin. No float money, kW treated as kWh, L added to kWh, zero for
  missing values, unsupported conversion or overflow. Equivalent km/L on an EV
  label is not physical liquid consumption. MJ/km conversion preserves basis.
- **B-BR-EV-05 — Place/equipment/connector identity:** public charging location,
  EVSE/equipment and connector are separate entities with stable internal IDs
  and source-scoped external IDs/versions. Location count, EVSE count, sockets
  and simultaneous vehicle capacity are not interchangeable. One CNPJ/brand can
  operate many places; do not use CNPJ as the unique charger key. Unknown child
  equipment stays unknown, not a synthetic connector falsely counted as real.
- **B-BR-EV-06 — Safe reconciliation:** external source IDs and reviewed evidence
  govern merge/split; names/coordinates alone create candidates, not automatic
  identity. Keep source assertions, conflicting facts, duplicate lineage and
  operator changes. OCM API/export or OSM/Geofabrik are not independent witnesses.
  A partial import/missing row never silently closes or deletes a location.
- **B-BR-EV-07 — Sources and rights:** freeze source/profile/license, raw hash,
  fields, attribution, clocks, boundaries and limits before ingestion. Open
  protocol/public webpage does not grant commercial, offline or redistribution
  rights. Preserve each provider's license across backend, exports and Room;
  code MIT notices do not license third-party data. Restricted provider content
  cannot enter an unrestricted downloadable national pack.
- **B-BR-EV-08 — Deterministic Rust batches:** extend station-prep's local-file,
  no-network/no-database preparation pattern. Validate actual source bytes/profile
  and stream bounds; minimize fields; emit typed versioned manifests/records,
  complete independent census, duplicates/quarantine and field provenance. Unknown
  schemas, required artifact tampering or fatal framing produce no admitted
  partial batch. Go owns fetch/lease/atomic publication and recovery, not Rust.
- **B-BR-EV-09 — Source clocks and access:** fetched time does not refresh source
  status/tariff evidence. Distinguish operator update, observation, source edition,
  validity interval, successful check and private session time. Public/restricted/
  customer-only/home access and hours are independent of catalog existence. No
  default open-24h/free/public assertion from absent provider tags.
  Residential/home candidates are not automatically eligible for public maps or
  reports. Require documented public-access/publication eligibility; minimize
  contributor/contact fields and never publish a private user's home coordinates.
- **B-BR-EV-10 — Compatibility:** reason from supported connector/format, AC/DC,
  known vehicle limits and access restrictions. Show compatible/incompatible/
  unknown with explanation. A matching plug or advertised station power does not
  prove actual charging power, safe adapter use or guaranteed session duration.
  Unknown/unsupported types survive wire evolution without crashing clients.
- **B-BR-EV-11 — Tariff context:** tariff version, location/EVSE/connector scope,
  currency, tax inclusion, eligibility/membership/payment channel, timezone and
  effective interval belong to every offer. Components can include energy,
  charging time, idle/parking time, fixed session and explicitly separate parking
  fees, with steps/grace/min/max/restrictions only when sourced. Unknown is not
  free. A member/brand benefit is never shown as a universal standard price.
  A provider's free-text cost description remains a bounded attributed note;
  it is not a complete structured tariff inferred by regex or missing-fee defaults.
- **B-BR-EV-12 — Price comparison:** rank like-for-like energy rates only when
  those fields/conditions are complete, explicitly label excluded fees; otherwise
  compare estimated total for the same declared energy/time/parking scenario.
  Unknown mandatory components produce incomplete/unrankable, not cheapest. Save
  tariff revision and assumptions; a simulation is not an actual bill or guarantee.
- **B-BR-EV-13 — Community reports:** signed free-account writes target an eligible
  stable public charging entity and exact tariff/condition/component scope. Store
  reported/observed times, provenance and bounded content; support correction,
  withdrawal, replay, owner reads, quotas and existing moderation/privacy.
  No compulsory GPS, payment, document upload or public home-location admission.
  Ordinary optional media follows existing accepted retention/proof policy only.
- **B-BR-EV-14 — Independent signals:** price report, successful/failed charging
  experience, access report and provider availability are different evidence.
  Helpfulness/votes and repeated snapshots do not certify price or independence.
  Freeze EV-specific eligibility/freshness/conflict policy with evidence before
  current-price promotion; do not blindly reuse liquid-price thresholds. Until
  accepted, reports remain dated source-separated claims, with no invented score.
- **B-BR-EV-15 — Operational availability:** occupied/available/fault/out-of-service
  states require an authorized fresh per-equipment source and clear last update.
  Expiry/outage/unknown becomes stale or unknown, not available. A check-in,
  static listing, protocol support or old status cannot imply a reservable/live
  connector. No live-status promise for Brazil from global API marketing.
- **B-BR-EV-16 — Private charging ledger:** owner/origin-bound session records
  include source/measurement basis, actual energy/amount when known, times,
  odometer, optional SOC and tariff snapshot/fees. Home/work/public context is
  private unless the user separately submits an eligible public observation.
  A private charge must never automatically publish price, address or SOC history.
- **B-BR-EV-17 — Honest energy metrics:** bought/metered energy per km, measured
  vehicle efficiency, PBE reference and modeled battery estimate are different
  methods. Compute only on matched complete distance/energy windows with declared
  SOC/anchor/gap policy; otherwise explain UNKNOWN. SOC percentage times nominal
  battery size is an estimate, not metered energy. Never assume a fixed loss rate,
  measure battery health from bills or present nominal range as guaranteed travel.
- **B-BR-EV-18 — Hybrids and reports:** retain liquid entries/quantities plus
  electric sessions in one vehicle. Sum actual compatible monetary costs over
  the same distance window for R$/km, without double-counting wallet top-ups,
  fuel, session fees or parking. Do not assign all PHEV kilometres to liquid or
  electric propulsion without evidence. Monthly timezone/correction/coverage and
  supported versus unknown methodology follow PRM and EV financial vectors.
- **B-BR-EV-19 — Home energy cost:** user-entered known effective rate/actual total
  has declared basis and dates. Distributor TE/TUSD reference is not the final
  household bill, public charger price or automatically zero-cost solar energy.
  Fixed charges, taxes, flags and allocation assumptions remain explicit when
  applicable; no utility-account/document scraping or solar savings guarantee.
- **B-BR-EV-20 — Local-first processing:** Android imports validated bounded
  typed public snapshots/pages into Room with manifest/schema/source/licensing
  metadata, atomic generations and safe deltas/full-resnapshot. Preserve old
  accepted data on interrupted/inconsistent download; no whole-user DB wipe.
  Lists/search/filter/detail/estimates/allowed private entry work offline with
  honest age. No full national PBF/PDF scan or network work on UI thread, no
  requirement to embed Rust into the APK without a separately proven need.
- **B-BR-EV-21 — Sync/privacy:** extend PRM's explicit allowlist for vehicle energy
  fields and eligible private sessions/reports; keys, tokens, raw bills/photos,
  precise home/route location and public packs remain excluded. Durable receipts,
  conflicts, owner switch/lapse, erasure and restore fences apply. No pretending
  local writes are synced or restored; held-data exit survives subscription loss.
- **B-BR-EV-22 — Shared discovery and official context:** reuse MEX city/discovery,
  scoped filters/cursors and rankings; public EV community/catalog do not gain a
  paid gate. Keep electric tariffs distinct from ANP liquid prices and official
  fuel occurrences. Co-located fuel/charging entities can link only with safe
  evidence; an ANP fuel infraction is not an EVSE violation. Community complaint
  is not an official sanction or electrical safety inspection.
- **B-BR-EV-23 — Bounded runtime:** existing Go monolith/jobs/PostGIS, Rust batch
  tool and Kotlin layers remain. Typed ports separate modules. Licensed partner
  reads are bounded/cancellable/cached only as permitted. No ingestion/fan-out to
  providers per search/keystroke, leaked API key or uncontrolled polling. Query
  filters apply before pagination, source vectors bind cursors/ETags, and mutable
  status/tariff expiry invalidates results even without a write.
- **B-BR-EV-24 — Scope and acceptance:** every declared local user journey requires
  meaningful tests and atomic evidence/commit. Full support means these journeys
  are complete, not all models/chargers/network coverage. Track rights/provider/
  source/device/deployment proof separately. D receipts/business portal remains
  held; no vehicle tracking/control, OCPP server, payment/start/stop/reservation,
  iOS, commercial activation, connector safety certification or automatic routing
  feasibility engine is introduced by this data-support amendment.

## User journeys

- **BUC-EV-01:** add/restore/update ICE, BEV, PHEV and HEV/MHEV, optional unknown
  specs, existing IDs and one-free/additional-Premium lifecycle.
- **BUC-EV-02:** select exact model/version reference, correct a capability,
  compare lab/manual/measured values without changing history silently.
- **BUC-EV-03:** browse/search compatible public charging places, equipment and
  connectors by city/region/access/power/type; offline/no-price/unknown included.
- **BUC-EV-04:** inspect complete tariff conditions/fees or unknown components,
  compare a declared scenario and return to source/history.
- **BUC-EV-05:** signed community tariff/access/experience report, replay,
  correction/withdrawal, helpfulness/report and moderated public/private states.
- **BUC-EV-06:** private home/work/public session entry, incomplete measurements,
  offline draft/save/reopen, correction and no automatic public disclosure.
- **BUC-EV-07:** BEV/PHEV month/cost/efficiency reports, matched windows/gaps/SOC,
  supported units, comparisons and explicit unknown results.
- **BUC-EV-08:** authorized operator status/tariff read, expiry/outage/unknown,
  inaccessible provider and no fake live/reservation status.
- **BUC-EV-09:** signed public pack/page import, schema/rights/clock metadata,
  cancellation/corruption/old cursor, atomic replacement, local filters/reconnect.
- **BUC-EV-10:** Premium sync/import conflicts, late account switch, held-data exit,
  deletion/restore and exclusion of keys/home location/public cache.
- **BUC-EV-11:** master questions for compatible chargers and contextual cost,
  source-separated geography/period, incomplete tariffs and no fake national best.
- **BUC-EV-12:** source correction/alias/removal, full census/lineage, retained
  fuel/ANP functionality, accessibility/locales, bounds and scoped rollback.
