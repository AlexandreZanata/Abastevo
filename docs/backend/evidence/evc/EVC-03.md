# EVC-03 — Deterministic EV source conflation

Status: LOCAL_DONE / INTEGRATION_PENDING
Owner imagery acceptance: OWED

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

## EVC-03B opening

B-BR-EV-MATCH-03B: Admit only a complete hash-bound normalized v2 generation
with the expected source pins, profile hashes, file set and accepted decision
identities. Verify all file bytes/hashes before matching; source/site/equipment
counts must reconcile. Country boundaries use the same pinned IBGE bytes.
Keep catalog, pair reviews, conflicts, hierarchy reviews, assignments and per-UF
counts distinct. Unresolved source equipment stays reviewable, never a new site.
The owner imagery sample is marked PENDING until the owner actually reviews it.

BUC-EVC-03B: Test repeatable private file preparation, changed/stale/missing
input, inconsistent source decisions, cancellation, existing-output preservation
and private modes before real execution. Emit manifest-last owner-only output,
with each file hash and both input/behavior profile identities. Run the approved
local corpus twice, independently reconcile source/queue/UF counts and generate
30 deterministic source-linked imagery candidates. No network, DB or staging.


## EVC-03B checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent 1981531d plus this child. The private CLI admits only hash-bound
normalized v2, matching source/profile pins, the complete input file set and
reconciled accepted decisions. Four file/CLI test groups cover repeatability,
private modes, changed/stale/missing input, decision inconsistency, cancellation,
last-good preservation and staging/rule-override refusal. Two offline runner
groups cover hash-bound receipts and changed profile/input/output refusal.
The full Rust fast gate (fmt, all-target clippy, tests) and all 18 offline Python
runner/acquisition groups pass. No new dependencies, SQL, API or staging changes.

Two real local runs produce eight byte-identical generation files, including the
manifest SHA-256 `d3b5d0ab7b0b0b05933decd285d012a052210391c4f46ea10a93d2befb80f44c`.
The accepted input is normalized v2 SHA-256
`61a0d64aaf2eda85dae247b3948a9d0df49a49baa1c019a83bab2bca1af4877b`.
Replay: 0.311 s, 59,670,528 peak RSS bytes, 9,175,055 working bytes;
all frozen budgets pass. [Measured receipt](EVC-03-execution-20261103.json).

The catalog contains **1,658 sites**: five two-source pairs, 400 OSM-only and
1,253 OCM-only. There are three probable pairs, 38 socket-conflict pairs and no
consumed confirmed candidates. Only two of 64 equipment records have supported
parent assignments; 62 remain `unparented_equipment` for review, never added as
new sites. All 1,727 input source records partition into 1,663 roots, two attached
equipment and 62 unresolved records. The 1,665 catalog source assertions retain
all source fields, clocks, rights, groups, quantities and powers verbatim.

An independent Python audit exhaustively compared all 509,490 OSM/OCM root
pairs with a separate haversine implementation: exact confirmed/probable/conflict
identities and rounded distances match the indexed Rust output. A separate
ray-crossing polygon implementation on the pinned IBGE malha reproduces every
catalog per-UF count. All file hashes, unique one-to-one source consumption,
record partitions and per-UF category sums reconcile.
[Independent audit](EVC-03-independent-audit-20261103.json).

[Per-UF census](EVC-03-census-20261103.json) covers 25 UFs with admitted sites;
11 OCM-only sites remain UNASSIGNED. The dated OCM snapshot and missing OSM socket
tags limit confirmation: 37 of 38 conflicts have no OSM connector groups, while
one has disagreeing/unknown families. This is source catalog coverage, never a
complete physical Brazil inventory or a current availability claim. Queue counts
are pairs; equipment counts are separate units, never summed connector totals.

Thirty deterministic source-linked owner imagery candidates span 25 UFs plus
UNASSIGNED. The private file is
`/tmp/abastevo-evc03-catalog-20261103/generation/owner-review-sample.json`;
its hash is in the generation manifest. Every imagery result/reviewer/time is
null and the status is `OWNER_IMAGERY_REVIEW_PENDING`. Generating this sample is
not a completed manual spot check. Owner review remains OWED before final EV
acceptance. Raw catalog/source-linked records stay outside Git. No DB import,
backfill, network collection or staging mutation occurred.

Next source construction: SNA-04..06 from this tested checkpoint under ADR-018.
Final source/publication/staging acceptance and the protected cumulative PR
remain pending; this local checkpoint does not declare them complete.
