# EVC-04 station links

## EVC-04A opening

B-BR-EV-STATION-01. A candidate can bind only to a canonical station with a
current CNPJ and a completed ANP registry assertion for that identity. An
admitted combined OSM source object may use the exact object type and ID from
an active, unrevoked fuel assertion. Otherwise a current pin in one of the
three eligible tiers must be within 75 metres and agree on sourced trade
name or brand. Ambiguous candidates remain unresolved. No identity merge.

BUC-EVC-04A. Exercise the meter band, name mismatch, missing registry
evidence, ineligible coordinates, and exact combined OSM identity using
disposable PostGIS. Candidate lookup does not record a link. EVC-04B will
reuse the decision inside the existing review transaction.

Status: LOCAL_DONE
Validation: PASS

The first real PostGIS regression failed at the eligible 74.999 m case
before the resolver existed. The linking suite now passes with race
detection. It covers both sides of 75 m, name and brand mismatch,
staff-accepted brand evidence, address-point exclusion, missing ANP
provenance, ambiguity, exact OSM type and ID without a distance requirement,
and revocation. The pin query uses indexed geography and never rounds
the threshold. The resolver reads existing records; it performs no import,
link insertion or staging action. Vet, format, diff and secret checks pass.

## EVC-04B opening

B-BR-EV-STATION-01. Record a source-backed decision through the existing
identity-link store. Serialise automated and manual decisions on the place
row. An existing human decision, including a rejection, remains authoritative.
A replay returns the existing reviewed link without adding history.

BUC-EVC-04B. On real PostGIS, replay an admitted source link concurrently,
verify manual supersession and preserve a rejection. Upgrade the existing
link table to distinguish exact OSM object evidence from name and address
evidence, preserving prior decisions.

Status: LOCAL_DONE
Validation: PASS

The write/replay regression initially found zero links. The shared place
lock now serialises source and manual decisions. Five concurrent replays
wrote one exact OSM link. Loss of source evidence superseded the automated
link with an unresolved decision and replay did not append another row.
Manual supersession and rejection remained authoritative. An outer rollback
removed the source link along with its transaction.

Migration 000150 distinguishes OSM_OBJECT from previous review methods.
A real upgrade preserved the charging place and its pre-existing manual
review, and the migration replay applied zero changes. All linking
integration tests and charging unit suites passed with the race detector.
Vet, whitespace and secret checks passed. National generation import and
serving the links remain in EVC-06 and the subsequent consumers.
