# MAP-06 — Admitted charging and vehicle reference preparation

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS (existing admitted readers plus focused unknown/mixed-rights case)
Integration: INTEGRATION_PENDING

Tested source: `codex/map-ev-implementation` after MAP-05. Date: 2026-10-24.

## Behavior rules and use cases

- OCM charging keeps `PLACE` and `CONNECTION_GROUP` separate. A reported
  quantity is a group count, never proof of individual EVSE identities.
- Only the already-admitted open providers 1 (CC-BY-4.0), 40 (CC-0) and 57
  (CC0) are accepted. Any other provider quarantines; the `opendata` flag alone
  is not admission. Foreign places quarantine.
- Unknown connector/current codes stay `UNKNOWN` with the raw source code
  preserved. Missing/invalid power or quantity stays null; it is never zero,
  free or available. No live status is inferred.
- Contacts, telephones, emails, comments, media, private coordinates and
  contributor identity never enter entities. Coordinates remain exact source
  lexemes where the admitted profile requires them.
- PBE vehicle references stay optional. The admitted PBE profile emits only
  model identity and exact energy lexemes; manual useful capacity/consumption
  keeps the trip engine functional without a proprietary catalog. No
  manufacturer supplement is required or invented.
- No OCM/PBE row activates a source, publishes a price or claims current charger
  availability.

## Reuse and actual checks

The existing EE-04/EE-05 readers and preparers are reused unchanged except for a
focused test:

- `tools/station-prep/src/ev_ocm.rs`: admitted-provider/foreign/unknown
  connector/power/quantity behavior. The new test
  `keeps_unknown_connector_power_and_quantity_honest` confirms unknown code
  `9999` stays `UNKNOWN`/raw, null power/quantity, while TYPE2 7.4 kW → 7400 W
  and quantity 1.
- `tools/station-prep/src/ev_pbe.rs` and `bin/prepare_vehicle.rs`: optional
  vehicle references with exact energy lexemes.
- `contracts/ev/preparation-profiles-v1.json`: `ocm-br-20261021-v1` remains the
  admitted profile for providers 1/40/57 with license hashes/attribution.
  No new provider or source was added.

Commands and results:

```sh
cargo test --locked --manifest-path tools/station-prep/Cargo.toml --lib ev_ocm
# 5 passed (admission, place/groups, unlicensed/foreign, corrupt identity,
# unknown connector/power/quantity)
cargo test --locked --manifest-path tools/station-prep/Cargo.toml --lib ev_pbe
# 3 passed (electric rows, combustion/malformed quarantine, minimal entities)
cargo test --locked --manifest-path tools/station-prep/Cargo.toml \
  --test ev_prepared_contract
# 2 passed (frozen bytes/source identity, bounded refusals)
cargo test --locked --manifest-path tools/station-prep/Cargo.toml \
  --test ev_contract_vectors --test ev_tariff_vectors
# 1 + 1 passed
cargo clippy --locked --manifest-path tools/station-prep/Cargo.toml \
  --all-targets -- -D warnings
# finished, no warnings
git diff --check
```

Existing EE-04 evidence (outside this task) recorded a real admitted OCM snapshot:
1,298 places / 2,089 connection groups from pinned revision `8e3bedca…`,
1,816,497 bytes, providers 1/40/57 only, zero emails/PII fields in the prepared
artifacts. That snapshot remains outside Git and was not re-downloaded here.

## Limits and ownership

- OCM has no per-EVSE equipment identity in this profile; the reader preserves
  place + connection group and reported quantity. Canonical Abastevo
  place/equipment/connector UUIDs and aliases are MAP-07/MAP-09 work, not
  fabricated here.
- OCM/PBE do not carry station electricity tariffs. Community tariff units and
  conditions remain MAP-21/MAP-03 contract work.
- PBE/manufacturer data is optional and only admitted reference data; no
  proprietary variant matching or closed catalog is required.
- No Go/PostGIS publication, API, map package, Android UI or device evidence.
- No national charging coverage, live availability, production or release claim.

Next: MAP-07 identity reconciliation and navigation entrances.
