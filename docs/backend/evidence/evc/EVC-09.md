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

## EVC-09C typed community charging intake

B-BR-EVC-09C. A missing charger is a typed private proposal through
the existing authenticated, quota-limited, idempotent intake. A public
name or street address is required. A charger proposal carries neither
a fuel CNPJ nor contributor coordinates. Fuel directory moderation cannot
approve it as a fuel station. A proposal never becomes a public pin.

BUC-EVC-09C. Submit an address-only charging proposal, replay it, refuse
a changed payload or invalid target, and verify persistence and owner
isolation in PostGIS. A forged charging proposal with a fuel identifier
cannot pass either fuel verification path.

RED received HTTP 400 for a typed address-only charging proposal. The
existing private intake now accepts the additive `charging_site` kind
and keeps prior fuel payloads and idempotent replay compatible. Typed
records stay outside the automatic fuel queue and cannot be linked by
the fuel approval path. The existing owner status and cancellation
remain available. This checkpoint creates no public charging record.

Status: LOCAL_DONE / INTEGRATION_PENDING. Real PostGIS covered typed
persistence, replay, owner isolation and exclusion from the fuel queue.
HTTP and application tests covered invalid fields and approval refusal.
Race checks, SQLc generation/vet, Go vet, staticcheck, OpenAPI lint with
zero errors, whitespace and secret review passed. Android submission
and the gap action are the next task.

## EVC-09D Android gap action

B-BR-EVC-09D. A genuine empty catalog result can offer Add charging site.
Use the existing private community intake form in charging mode. Keep
the owner session and retry identity, allow name or public address, and
send no fuel CNPJ or device coordinates. Unavailable data is a separate
state and is not shown as a local gap.

BUC-EVC-09D. Open the charging form from the gap action, submit a
synthetic address-only proposal, and verify the typed payload, session
gate, and stable retry key. Existing fuel suggestions remain compatible.

Status: LOCAL_DONE / INTEGRATION_PENDING. The Explore and map gap
actions open charging mode, with four new strings in each supported locale.
The test-first compilation failed on the missing charging form contract.
After implementation, 11 targeted JVM tests passed and both debug APKs
built. Four Compose tests passed on emulator-5570, including activation
of the gap action. The form submits an authenticated synthetic address-only
proposal with an unchanged retry key. Whitespace and secret checks passed.
