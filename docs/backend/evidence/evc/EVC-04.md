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
