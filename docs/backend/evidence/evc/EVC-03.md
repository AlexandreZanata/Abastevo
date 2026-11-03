# EVC-03 — Deterministic EV source conflation

Status: IN_PROGRESS

Bounded children: EVC-03A1 source hierarchy/spatial primitives; EVC-03A2
one-to-one cross-source matching/field lineage; EVC-03B private CLI, real per-UF
reconciliation and owner imagery-review sample. Finish construction before
SNA-04; imagery acceptance remains an explicit owner gate, never fabricated.

B-BR-EV-MATCH-03A1: Equipment-only records do not create sites. Preserve explicit
site/equipment markers; an unparented dual-marked object retains its explicit
site evidence. Aggregate equipment into an existing OSM site only through a
unique explicit source parent or unambiguous source-footprint containment.
No nearest-parent guess. Parent chains may resolve to one root; multiple roots,
missing declared parents, boundary ambiguity and cycles go to review. Equipment
and parent source assertions stay distinct, and reported quantities are never
added or mistaken for physical EVSE counts. Ordinary nested site records are
not automatically merged merely because a polygon contains their point.

BUC-EVC-03A1: Build bounded spatial indexes, validate source points/geometry,
resolve explicit/contained source parents deterministically, retain the root
site plus separately attributed equipment, and emit sorted review reasons for
unresolved equipment. Test explicit membership, containment, holes/borders,
ambiguous parents, parent chains/cycles, dual markers, unparented equipment,
input order and invalid coordinates. No file/network/DB access in the domain.

## EVC-03A1 checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent 4450023d plus this child. Six pure synthetic test groups pass:
explicit membership and footprint containment, holes/borders, ambiguous and
missing parents, source chains/cycles, dual markers, source order, bad points,
invalid/self-crossing parent topology and partially overlapping child footprints.
A bounded footprint grid avoids a source-wide parent pair scan. Parent depth,
candidate and grid budgets refuse/review without nearest-parent guessing.

Output preserves root source assertions and separate equipment records. The
sorted assignment sidecar records source_membership or source_footprint plus
intermediate candidate IDs; original source parent fields are unchanged. Shared
source identities are not quantity additions. Unresolved equipment stays in a
sorted review queue. Source sites are not automatically merged merely because
their point lies in another source footprint. Fmt/all-target clippy with denied
warnings, diff and secret checks pass; no dependency, file/DB/network/API change.

Next source audit action before matching: an independent broad coordinate sanity
check found one accepted OCM record outside a conservative Brazil extent despite
CountryID=36. Its coordinate is not repaired/guessed. Add a new versioned country-
extent guard from the already-pinned IBGE boundaries and regenerate the private
normalization before EVC-03A2. Existing v1 artifacts/profile stay unchanged and
are superseded for dependent work; the remaining UNASSIGNED coastal/boundary
records remain honest pending review, never assigned a nearest UF.
