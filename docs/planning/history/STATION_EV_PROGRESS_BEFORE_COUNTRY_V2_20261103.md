# Station/EV execution checkpoint before country-consistency v2

Historical current-state record retained during compaction on 2026-11-03.
Superseded by current PROGRESS and the linked per-task evidence.

- Station/EV ingestor 2026-11-03: IN_PROGRESS on isolated
  `codex/station-ev-ingestor`, synchronized with main d0eb9c79. Plan first
  commit 415c5af0; SNA-01A/B, SNA-02A/B/C and SNA-03 LOCAL_DONE / INTEGRATION_PENDING.
  [SNA-01](../../backend/evidence/sna/SNA-01.md),
  [SNA-02](../../backend/evidence/sna/SNA-02.md): named pinned Rust batches, nullable
  source metadata, audited name/address projection, edition fences and explicit
  queued re-projection pass real PostGIS/race (two dispatchers, page rollback,
  replay, source/identity negatives, audit concurrency, migrations 146..148).
  Tested parent 91305b9e plus SNA-03: additive API/source separation/CPF privacy,
  all affected PostGIS, unit/race, build/vet/staticcheck/sqlc, OpenAPI, secret
  and diff checks pass. [SNA-03](../../backend/evidence/sna/SNA-03.md).
  EVC-00 inventory/register reconciliation LOCAL_DONE: staging has zero EV
  catalog/overlay assertions; active overlay holds 66 OSM fuel rows. Historical
  OCM admission restored with scope; provider-57 digest is corrected in the
  new EVC-01 pin while historical artifact identity stays unchanged. [EVC-00](../../backend/evidence/evc/EVC-00.md).
  EVC-01A acquisition code LOCAL_DONE: strict pins/budgets/private output and
  OCM header/pagination tests pass. Owner-approved local OSM/IBGE downloads
  and dated OCM fallback completed; no API key required.
  EVC-01B national profile/geography/census/resource fences LOCAL_DONE: full Rust
  gate and ten offline Python groups pass (parent 188e7bea plus child). OSM/IBGE
  downloads verified; owner approved dated OCM export fallback (22 Apr 2026).
  EVC-01C LOCAL_DONE: all 1,298 BR OCM blobs/licenses verified (dated fallback),
  national OSM 363,988,420 elements → 28,579 fuel / 407 charging / 0 combined raw
  objects; 129 unassigned UF, one quarantine. Two outputs byte-identical; accepted
  replay 109.163 s / 926.7 MB RSS within frozen budgets. Fourteen offline Python
  groups pass; source pins/UF census/receipts in EVC-01 evidence. No import.
  EVC-02A LOCAL_DONE: frozen socket/current mapping and exact decimal power;
  corrected legacy OCM 20→AC / 30→DC, no rounded/fabricated W. Full Rust gate
  and 14 offline Python groups pass (parent fa9e43cf plus child); lockfile unchanged.
  [EVC-02](../../backend/evidence/evc/EVC-02.md). EVC-02B1 LOCAL_DONE: minimized
  common site DTO/OCM reader, pinned rights, access/status exclusions, per-path
  exact numerics, privacy/duplicate/clock negatives; 12 focused tests + all-target
  clippy/fmt pass. EVC-02B2 LOCAL_DONE: typed OSM site/equipment units,
  lifecycle/private filters, source versions/clocks/geometry, parent sets,
  noninvented source fields and exact separate socket groups. Full Rust gate,
  then 13 affected tests/all-target clippy after final filter/refactor pass
  (parent 62366afb plus child). EVC-02C LOCAL_DONE: hash-bound private preparation
  and two byte-identical real runs (parent ccc52734 plus child); 16 offline Python
  groups/full Rust gate pass. OCM 1,259 source sites; OSM 400 charging + 6 combined,
  64 equipment; 46 exclusions. All 1,775 decisions reconcile; dual-role audit
  preserved ten explicit sites/44 closed footprints. Accepted replay 70.747 s /
  370.4 MB RSS. These are pre-conflation source assertions, not unique locations.
  EVC-03A1 LOCAL_DONE: deterministic parent hierarchy/footprint grid, explicit
  derivation sidecar, six pure geometry/chain/privacy negatives + clippy/fmt pass.
  [EVC-03](../../backend/evidence/evc/EVC-03.md). Before matching, data audit found one
  OCM coordinate far outside Brazil despite CountryID=36: a versioned IBGE extent
  guard/regeneration is next; v1 private output is superseded for dependent work.
  Then EVC-03A2/B matching/real counts, SNA-04..06. Owner imagery check remains owed.
  Read-only staging baseline: 48,770 station rows, 40,388 with points, 3,003
  without UF; zero charging batches/links. This is not a national EV coverage
  result. PR #183 merged d0eb9c79 with all four checks green; signed release
  deployed automatically at 12:39:49Z, ready API/worker imageIDs match its
  manifest, public health HTTP 200. ZD live strategy/drills remain OWED.
  No ingestor migration/backfill ran on staging. Approved downloads are local;
  any new download/key, import/backfill or staging mutation needs approval. Final cumulative dev →
  main PR, per-UF reconciliation and staging acceptance remain owed.
