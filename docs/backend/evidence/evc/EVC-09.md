# EVC-09 coverage and honest presentation

## EVC-09A geographic coverage

B-BR-EVC-09A. Count one canonical charging site once. Use the pinned
IBGE polygons for municipality identity, retain ambiguous sites as
unassigned, and include zero-count geographies. Separate published
ABVE/Tupi point counts from catalog site counts and omit a coverage ratio.

BUC-EVC-09A. Classify an inside and outside site using synthetic IBGE
geometry, preserve existing UF results, and reconcile the approved
national replay by municipality and UF.

The file-only coverage command classifies one canonical site at a time
and emits all 27 UFs and all 5,571 municipality codes present in the
pinned IBGE file. Ambiguity inside one UF preserves the UF while leaving
the municipality unassigned. The regenerated EVC-06 geography sidecar
was byte-identical to its pinned predecessor.

The already approved OSM and OCM replay yielded 1,658 canonical sites.
The IBGE model assigned 1,647 of them to 567 municipality codes in 25
UFs. Eleven sites have no UF or municipality classification. AC and AP
have zero catalog sites. Zero denotes an absence in this catalog.
The [UF census](EVC-09-uf-census.json) agrees with the independent Go
import plan. The complete municipality report SHA-256 is
`28fbf748f8cf4fd20c31f2f76d202ab61f4dadeee4b13d2bb2de5bced7648de2`.

For context, ABVE/Tupi reports 29,866 public and semipublic charging
points for August 2026 and provisionally identifies 1,911 municipalities
with charging infrastructure. Its published examples include 7,864
points in São Paulo state and 3,101 in the city of São Paulo, where
this catalog maps 346 and 108 sites respectively. The report counts
points, while our catalog counts locations with different source dates
and admission rules. No percentage of the benchmark or equipment total
is derived. [ABVE/Tupi release](https://abve.org.br/recarga-rapida-dc-quase-triplica-em-12-meses-e-ja-responde-por-38-da-rede-brasileira/)

Status: LOCAL_DONE / INTEGRATION_PENDING. Nineteen focused Rust tests,
including boundary and same-UF overlap, formatting, all-target Clippy,
whitespace and secret review passed.

## EVC-09B app coverage wording

B-BR-EVC-09B. Label charging catalog results as mapped by open sources.
An empty filtered or local view describes what the catalog found and
cannot imply the absence of real infrastructure. Keep missing snapshots
and failed requests distinct from a geographic gap.

BUC-EVC-09B. Render the open-source attribution and honest empty copy in
Explore and use the same distinction on the map. The current charging
observation route requires an existing place ID. Adding a previously
unknown site requires a separate typed intake proposal.

The Android map and Explore use the same open-source notice. An empty
result describes only the currently available catalog and filters. A
missing cache or failed charging query uses separate unavailable copy.
All four coverage strings are available in eight locales.

Owner-only source candidates already in the project register include
Tupi, EZVolt and Zletric, the historical Raízen/Shell Recharge lead,
operator OCPI agreements, and potential commercial catalog providers
PlugShare, TomTom and HERE. Review regional scope, refresh, attribution,
offline use, redistribution, contact authorization and cost before
admission. These are research leads with no access or current feed
claimed. [Existing source register](../../../planning/EV_SOURCE_RESEARCH.md)

The current charging observation endpoint accepts an existing place ID.
The fuel suggestion form and moderation queue require a distinct proposal
type before being offered as a way to add missing chargers. That typed
intake remains open for the next app checkpoint.

RED failed because the coverage notice was missing. GREEN passed
29 map ViewModel tests and four Compose tests on the disposable emulator.
The first combined device run was interrupted. The isolated regression
and the subsequent full suite passed. Debug APK and test APK builds,
whitespace and secret review passed.

Status: LOCAL_DONE / INTEGRATION_PENDING.
