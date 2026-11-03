# EVC-06 publication evidence

## EVC-06A opening

B-BR-EVC-06A. Compare counts of canonical sites in each Brazilian UF
against the last published generation. Refuse a drop strictly greater
than 15 percent in any UF, including complete disappearance. Unassigned
sites remain a separate diagnostic and never compensate for a UF drop.
An owner override is a separate publication decision.

BUC-EVC-06A. Check first publication, exact threshold, small counts,
disappearance and simultaneous loss/growth across UFs. Reject invalid
census input instead of treating it as a first publication.

Status: LOCAL_DONE
Validation: PASS

The initial tests failed on both missing-state coverage and invalid
input. The comparison now uses integer arithmetic with the exact
strict threshold, without overflow. State changes cannot be hidden
by gains elsewhere or by unassigned counts. Charging domain tests
with race detection, vet, formatting and secret review passed.

## Pinned source recovery opening

B-BR-EV-SRC-01. An HTTP content coding must not change the admitted IBGE
document bytes or bypass the acquisition byte ceiling.

BUC-EVC-SRC-RECOVERY. Recover the previously authorized IBGE edition
when the publisher applies gzip. Refuse corrupted and over-budget
compressed responses before writing a source file.

Status: LOCAL_DONE
Validation: PASS

The official IBGE endpoint returned gzip with HTTP 200 and the old
collector rejected the compressed body as invalid JSON. The collector
now bounds both transport and decoded bytes for this endpoint and
refuses corrupt gzip. Eight acquisition test groups passed. A fresh
authorized retrieval produced 3,998,504 decoded bytes with the exact
EVC-01 SHA-256 5491bd548fe381cd388ba81c9edfbf4cf2749891e2fd32c1079ac71f7aba1c10.
No source edition changed.

## EVC-06B opening

B-BR-EVC-06B. One national publication pointer exposes the last admitted
batch. Serialise publication under its row lock. A hold, missing rights
receipt, or an unapproved UF regression leaves the pointer unchanged.
An owner exception binds both generations and carries an audit reason.

BUC-EVC-06B. Publish synthetic batches on disposable PostGIS, exercise
rollback, hold, stale and valid overrides, and verify idempotent replay.

Status: LOCAL_DONE
Validation: PASS

The activation regression first failed before the gate existed. The new
append-only schema retains a single current pointer and an activation
audit. On disposable PostGIS, five concurrent calls produced one
activation. Missing rights metadata, excessive UF loss, stale override
and publication hold left the prior pointer intact. A valid override
recorded its actor and reason. Rolling back the caller transaction
removed both pointer change and audit. Superseded automatic replay was
refused. Race, vet, format, whitespace and secret review passed.

The next import step must derive and verify the per-UF counts and offer
bytes before registering this metadata and calling the gate in the same
transaction. This checkpoint does not provide a serving path or authorize
a real import.

## EVC-06C opening

B-BR-EV-RIGHTS-01. The backend admits only a pinned rights receipt for
the expected conflation manifest. The required national ODbL data and
notice must both match their exact descriptors. The verified offer
payload is the catalog input, so import cannot silently substitute
different data.

BUC-EVC-06C. Admit a synthetic national bundle and refuse missing,
modified, oversized, unexpected, or misbound artifacts before storage.

Status: LOCAL_DONE
Validation: PASS

The synthetic bundle first failed on the absent backend verifier. Go
now requires independently supplied receipt and conflation manifest
hashes, the exact file set and sizes, and a notice whose ODbL and data
descriptor match the receipt. The catalog returned to the importer is
the verified offer payload. Missing, modified, extra, misbound and
over-budget artifacts are refused. Prepared race tests, vet,
staticcheck, formatting and secret checks passed. This boundary reads
files only. Parsing the national rows, deriving the UF census and
staging the batch are the next import step.

## EVC-06D opening

B-BR-EVC-06D. A new charging generation keeps the public IDs associated
with admitted source records. If sources converge into a single site,
old IDs remain aliases. If a prior public ID would address multiple
candidate sites, refuse the generation until the conflict is reviewed.

BUC-EVC-06D. Exercise a continuing legacy OCM identity, a source merge,
loss of one source, a split conflict, and deterministic fresh identity.

Status: LOCAL_DONE
Validation: PASS

The merge regression initially failed because no public identity was
resolved. Existing public IDs and historical aliases now follow the
root source IDs. A merge retains every old ID, a disappearing source
does not drop aliases while another root continues, and a conflicting
split refuses admission. New sites have deterministic IDs. Prepared
race tests, vet, staticcheck, whitespace and secret checks passed.
The national store still needs to persist this mapping and use it
for active-generation lookup.

## EVC-06E opening

B-BR-EVC-06E. Decode the verified offer into national storage inputs
without filling unknown names or combining connector quantities.
Every group keeps the root or equipment source that supplied it.

BUC-EVC-06E. Decode a synthetic two-source place with no name and two
groups of the same family, then refuse malformed attribution and
duplicate source-group identity before returning any rows.

Status: LOCAL_DONE
Validation: PASS

The initial projection test failed before any rows could be decoded.
The verified offer now decodes into storage inputs with nullable source
name, source-backed point and access, and each root or equipment
assertion's rights and lineage. Same-family connector counts remain
separate. Misowned groups, missing provider identity, inconsistent
confidence and duplicate identities refuse the entire projection.
Prepared race tests, vet, staticcheck, whitespace and secret review
passed. Database persistence and the pinned geography sidecar are
pending.

## EVC-06F opening

B-BR-EVC-06F. Classify each canonical point with the pinned IBGE
polygon index. Keep unassigned locations explicit. Bind each result
to the site key and exact catalog and boundary digests. Derive the
publication census from this one-to-one mapping.

BUC-EVC-06F. Classify a synthetic site with a polygon and a site
outside it. Refuse a sidecar with missing or duplicate rows, bad UFs,
wrong hashes, or a site key from another catalog.

Status: LOCAL_DONE
Validation: PASS

The initial Rust and Go tests failed without the classifier sidecar.
The local command now uses the pinned polygon index and emits
deterministically sorted decisions. The Go binder checks the exact
sidecar, catalog and boundary digests and joins every row exactly
once. Its output derives the UF totals, with UNASSIGNED separate.
Missing or repeated sites, invalid UFs and conflicting reasons were
refused. Rust rights tests and all-target Clippy, Go prepared race
tests, vet, staticcheck, format and secret review passed. The real
catalog replay remains separate from these synthetic tests.

## EVC-06G1 opening

B-BR-EVC-06G1. Reuse charging batch, place and connector storage. National
rows preserve an absent name, their UF and confidence, and each connector's
source identity. A public alias refers to one physical place in the same
generation. Legacy row constraints remain intact.

BUC-EVC-06G1. In disposable PostGIS, store legacy and national shapes,
refuse nameless legacy rows and extended legacy connector codes, and
prevent ambiguous or cross-generation alias references.

Status: LOCAL_DONE
Validation: PASS

The initial PostGIS test failed on the absent national storage fields.
Migration 000152 retains legacy name and connector checks when no
national metadata is present. National rows can preserve a missing
name and the wider connector vocabulary. The alias key is unique per
generation and its foreign key refuses a place from another batch.
PostGIS storage and activation race tests, SQLc generation and vet,
whitespace and secret review passed. The importer transaction is the
next step.

## Real catalog replay and source identity correction

Status: LOCAL_DONE
Validation: PASS

Two replays from the same recovered source hashes reproduced the previous
normalization and conflation manifests byte-for-byte. The optimized
normalization took 122.914 seconds. The real rights gate exposed two
fixture mismatches: OSM uses a digest-qualified revision, and OCM root
keys use the ocm:poi namespace. Both verifiers now check the emitted
identity exactly. Migration 000153 admits that key for national groups.

The local ODbL offer contains 1,658 sites and 3,041,372 bytes. It includes
407 OSM source assertions (including equipment) and 1,258 OCM assertions,
with OCM provider IDs 1, 40 and 57. Catalog SHA-256
7b105a1eb91471629e4af9de1690001d3af1e3670cc26b2e7707aa60c4a0dabb.
Rights receipt SHA-256
52713c1cc28af777d53627a50486339ed3a31fa1c548faed0c5e5f278496215c.
Geography SHA-256
a8f120b421613f7ae7a6f088f80a87cb8eae173588d5a579b785a776bdecfc3d.

Go verified the files and re-derived the previous catalog site counts
across 27 UF buckets and UNASSIGNED. Eleven sites remain unassigned.
The run read private local artifacts only. No real database import or
staging publication was performed.

## EVC-06G2 opening

B-BR-EVC-06G2. Admit the pinned national offer and IBGE evidence before
database writes. Serialise staging and activation with the publication
row lock. A failed hold, coverage check, source-link write or alias
decision rolls back the entire import. An identical replay is idempotent.

BUC-EVC-06G2. Import a synthetic two-source catalog, check the persisted
separate groups and alias, then verify a refused import leaves no rows
or pointer change.

Status: LOCAL_DONE
Validation: PASS

The initial synthetic catalog failed because the import entry point
was absent. The verified catalog, per-UF sidecar, stable aliases,
source-owned connectors and source links now pass through one
transaction. The publication lock prevents concurrent duplicate
imports. A synthetic group-write failure after place and alias
insertion left zero national batches. A hold refused admission.
Five concurrent replays returned the same active batch, retaining an
existing legacy OCM public ID. PostGIS race tests, vet, staticcheck,
whitespace and secret review passed.

This checkpoint has no public serving or operator command. Carrying
human decisions to the new physical rows and serving active aliases
and the rights offer are still pending. The national corpus was
verified in memory only.
