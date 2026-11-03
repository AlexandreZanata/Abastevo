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

## EVC-03A2 opening

B-BR-EV-MATCH-03A2: Use the reviewed project haversine metric (radius 6,371,000 m)
and a bounded three-dimensional spatial grid, never all source pairs. Confirm
only OSM site ↔ OCM POI pairs at distance <=50 m with a non-UNKNOWN shared socket
family, including equipment explicitly/uniquely assigned to the OSM parent.
At <=50 m without a shared family, queue a conflict; at >50 and <=150 m with a
shared normalized operator/network/name, queue probable review without merging.
Exactly 50 m belongs to the first band. Select confirmed edges by actual distance,
then qualified OSM and OCM source IDs, consuming each site once. Unselected
confirmed candidates are reviewed, never merged again. Pair-queue counts are not
site or connector counts. Private/removed/planned input is refused defensively.

B-BR-EV-FIELDS-03A2: Prefer the OSM point and source fields, then the OCM values;
operator prefers OSM operator/network, then OCM operator. Choose one usable
source address object, never combine unrelated street/number components. Both
source assertions, group quantities/powers, versions/clocks/rights and separate
equipment remain intact; no total/max connector/power field is invented.
Field lineage records the selected source and original field. Confidence is
only two-source or single-source, never live equipment availability.

BUC-EVC-03A2: Prove every band, unknown/conflicting sockets, one-to-one ties,
input order, parent-equipment capability, source-field precedence, private/
removed refusal and sparse-grid work. Keep deterministic canonical IDs/records,
sorted probable/conflict/ambiguity queues and counters that reconcile source
records and catalog sites. The later CLI must admit only normalized v2 inputs.

## EVC-03A2 checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent 7ec04382 plus this child. Seven pure matching groups plus all six
hierarchy groups pass; fmt and all-target clippy with denied warnings pass.
RED preceded the missing engine, then exposed unreviewed equal family labels
incorrectly confirming; GREEN admits shared families only from the frozen socket
contract. Reviewed JSON goldens pin canonical identity/confidence/field lineage,
source keys, distance and review/conflict outputs. Tests cover 0/49.999/50/50.001/
100/150/150.001 m, unknown/conflicting sockets, all-zero-distance one-to-one ties,
input order, parent-assigned equipment capability, exact per-source quantity/
power preservation, whole-source address selection, private/inactive refusal
and a 300+300 sparse dataset with fewer than 3,000 candidate comparisons.

The frozen [conflation profile](../../../../contracts/ev/conflation-profile-v1.json)
records the reported-point haversine metric, numerical boundary tolerance,
three-dimensional grid/candidate budget, edge ordering, source fields, hierarchy
bounds and identity seed. Queue counts describe pairs; they are not site totals.
Both source versions/clocks/rights and separate equipment remain unchanged.
Canonical IDs derive from a stable primary source anchor; public source/ID alias
preservation is an EVC-06 publication responsibility, not claimed here. No
source address components, connector quantities or maximum power are fabricated.

No new dependency, I/O/DB/API/staging behavior. Diff/secret review pass. Next:
EVC-03B admits only normalized v2 hash-bound input, emits reproducible private
catalog/queues/census, runs real conflation and prepares the owner's 30-site
imagery sample. Physical/manual/public acceptance is still pending.
