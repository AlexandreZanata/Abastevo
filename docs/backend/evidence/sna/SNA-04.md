# SNA-04 — Source-backed address enrichment

Status: IN_PROGRESS

Bounded children: SNA-04A pure address selection and proof/rights gates;
SNA-04B pinned private preparation and additive loader provenance;
SNA-04C authorized real before/after coverage. No live import is implied.

B-BR-STATION-ADDRESS-04A: Preserve the first usable source address in the
order ANP, active RFB exact CNPJ, an OSM POI already matched to the station,
an exact CNEFE establishment/address match, then municipality name and UF.
Choose one source address object, never mix street and number across sources.
Coordinates and municipality codes are not address text. Enrichment changes
neither authorization nor location quality/points, identity or station counts.

Every non-ANP candidate needs a source reference, input hash, observation date,
matching proof and affirmative reuse decision. Missing/restricted rights are
skipped with a reason, never treated as permission. Reuse evidence is distinct
from transport approval and factual identity. RFB's newest observation must be
active; a newer inactive row cannot revive an older active address. An ambiguous
newest observation is reviewed/skipped, not selected by file order. OSM links
must identify the same station; no nearest-point join. CNEFE requires the same
municipality, exact normalized street/number and establishment evidence; no
centroid or street-only join. Preserve the selected source reference in output.

BUC-SNA-04A: Before implementation, prove all precedence bands, complete
source-object selection, inactive/newer and conflicting evidence, wrong identity,
unmatched OSM, CNEFE exactness/establishment gates, missing/restricted rights,
coordinate/code-only rejection, input-order independence and unchanged station
denominators. Domain remains pure; file/DB/application behavior follows only
after this tested checkpoint. Fixtures are synthetic and contain no contacts.

## Source-rights inspection opening

On 2026-11-03 the official RFB [cadastros page](https://www.gov.br/receitafederal/pt-br/acesso-a-informacao/dados-abertos/cadastros)
links the CNPJ dataset and carries a CC BY-ND 3.0 site footer. That footer is
not recorded as a fresh dataset-derived-database admission. The existing
historical RFB address pipeline does not by itself prove this new publication
scope. The private file adapter must skip an unadmitted RFB source, retain the
reason and continue with admitted sources. Do not fabricate a permissive licence.
OSM ODbL attribution/offer obligations and the IBGE attribution remain applicable.
No new national download, source transfer, backfill or staging action here.


## SNA-04A checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent f08a4df6 plus this child. Six pure Rust test groups pass after
RED for the missing domain and a second adversarial RED for order-dependent
ambiguity references. The corrected result includes every conflicting reference,
sorts/deduplicates review reasons and refuses to revive an older observation
when a source clock/provenance cannot be established. Fmt and all-target clippy
with denied warnings pass. No dependency addition or file/SQL/API/app change.

The selector preserves one whole address object and its provenance; it never
fills an official missing house number from another source. Tests cover every
precedence band, inactive/newer RFB, tied disagreements, wrong CNPJ, missing OSM
link, CNEFE municipality/number/suffix/species/level/match proof, missing/restricted
reuse, invalid hash/date/reference, coordinate/code-only text and the 256-candidate
per-station budget. Municipal fallback requires a readable name and a valid UF;
no IBGE code, coordinate pair, authorization or location point is fabricated.

Reuse decisions and matched-source proofs are explicit ports. This domain does
not certify an arbitrary caller's rights declaration: SNA-04B must bind them to
actual admitted profiles and local file hashes before any prepared import.
No real enrichment or before/after coverage is claimed at this child checkpoint.

### Primary-source follow-up

The RFB page redirects its CNPJ link to the [official dataset catalog](https://dados.gov.br/dados/conjuntos-dados/cadastro-nacional-da-pessoa-juridica---cnpj).
Its rendered INFO section explicitly declares **Creative Commons Attribution**,
with no version shown. This dataset declaration, rather than the generic site's
ND footer, is the reuse evidence for the CNPJ source. Preserve RFB attribution
and the unversioned declaration; do not invent CC BY 4.0. The catalog is marked
outdated (metadata modified 2026-02-09, stated extraction 2026-01-11), so it is
not proof of a particular local file's edition or freshness. Those require
separate immutable byte/edition pins in SNA-04B/C.

The official [CNEFE product page](https://www.ibge.gov.br/estatisticas/sociais/populacao/38734-cadastro-nacional-de-enderecos-para-fins-estatisticos.html)
links the IBGE portal privacy/use policy but that policy does not establish a
specific dataset reuse licence. The inspected [2024 CNEFE methodological note](https://www.ibge.gov.br/biblioteca/visualizacao/livros/liv102091.pdf)
is 719,762 bytes; its opening copyright page alone does not prove CC BY. Do not
transfer the separately verified malha publication's licence to CNEFE by analogy.
CNEFE real-source admission remains PENDING; skip it unless primary dataset
permission is verified. Synthetic matching tests are not a rights grant.

Next: SNA-04B pinned local inputs, additive prepared schema/loader provenance,
negative admission tests and immediate real PostGIS tests for persistence.
SNA-04C real coverage needs authorized source files; do not initiate a national
transfer or run a staging backfill on this local checkpoint's authority.

## SNA-04B opening

B-BR-STATION-ADDRESS-04B: Opt in to a new assertion v4/parser identity; v1/v2/v3
bytes and loading remain supported. Hash-bound minimized local inputs carry
source, profile, edition, licence and attribution. The compiled source policy
admits RFB's exact unversioned Attribution declaration and OSM ODbL; pending
CNEFE is skipped without reading source rows. Neither an input's self-declared
reuse flag nor a transport approval can override the policy. Reject changed
bytes, escaped paths, extra/contact fields and mismatched row/manifest provenance.
Preserve original ANP address fields, selected whole-source address facts and
immutable provenance separately. No point or authorization change. Reconcile
before/after address-source coverage on the same station denominator per UF.

BUC-SNA-04B: RED the actual Rust CLI and both Go loaders, then prove exact input
pins, old-profile compatibility, address/proof/rights binding, deterministic
output and path/byte/row bounds. Add nullable JSONB provenance only through a
new migration; test failed upgrade/recovery and legacy preservation on actual
PostGIS. Test replay, concurrent prepared loads and mid-stream rollback. Original
ANP components remain separate; canonical identity geography may add municipality
metadata, never another source's street/number. Public OSM-derived publication
still owes its ODbL offer/attribution at the final release gate (EVC-05).

## SNA-04B checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent c4a5b1d6 plus this child. The opt-in v4 preparer binds minimized
RFB/OSM address evidence to a frozen rights policy and input bytes; CNEFE with
unverified reuse is skipped without opening its source file. Original ANP
fields stay intact in the prepared assertion. Four actual CLI test groups
prove repeatability, all stated file and rights refusals, OSM match provenance,
combined name/address flags and one count per current CNPJ despite repeats and
history. The six pure groups also cover conflicting candidate geography. Full
Rust fast gate and its 18 offline Python groups pass. No new dependency.

Both Go loaders require the v4 parser/policy and admitted pins, validate whole
address components, canonical geography and source-specific proof, and reject
legacy schema smuggling. Coverage buckets reconcile per UF/nationally and the
manifest cannot claim more current stations than accepted assertions. Existing
v1/v2/v3 loads and Rust legacy artifacts remain compatible. The private evidence
object is retained on the assertion in nullable JSONB via append-only migration
149; public API payloads are unchanged.

The directory race/integration suite passes against the disposable PostGIS 18
fixture. It executes the actual Rust v4 artifact, four concurrent loads,
provenance readback, projection query eligibility, wrong proof after one staged
row with whole-attempt rollback, in-memory refusal, JSONB/source constraint
negatives, failed-upgrade recovery and legacy assertion preservation. SQLc
regeneration/vet, Go vet/staticcheck and API/worker/ops/station-load builds pass.
Diff/secret review passes. No real station import, backfill or staging mutation.

[Contract and CLI](../../../../contracts/station-prep/station-addresses-v4.md).
Next SNA-04C needs actual authorized station-source inputs and independent
aggregate census verification. No qualifying national station input was found
in the local checkout. The previous national-run receipts identify existing
staging-era ANP/OSM/RFB cached sources, but copying or reading that cache needs
specific owner approval; the earlier VPS inventory approval covered images,
receipts and aggregate charging counts only. Raw temporary EV artifacts were
also absent after the host session changed; their committed fingerprints and
censuses remain evidence, and exact-pin recovery is needed before review/import.
