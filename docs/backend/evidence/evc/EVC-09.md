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
