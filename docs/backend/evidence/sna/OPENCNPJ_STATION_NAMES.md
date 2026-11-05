# OpenCNPJ station names and map control hierarchy — 2026-11-05

Status: LOCAL_DONE
Validation: PASS

Integration: INTEGRATION_PENDING. Staging publication and current-head remote
checks are separate operational acceptance steps, not implied by this source
checkpoint. The tested behavior is the source in this commit, based on
`b8e93a41` (merged PR #190).

## Behavior contract

B-BR-SNA-OCNPJ-01: Join the existing ANP establishment universe to the local
OpenCNPJ RFB snapshot by the complete 14-character CNPJ, preserving leading
zeros and branches. Export only CNPJ, trade name, active status and snapshot
observation date into the existing `rfb-station-names-v1` input. Bind the
snapshot and minimized output by SHA-256. No owner, partner, contact or GPS
data enters the import, logs or public API. Refuse unexpected identities,
duplicate/conflicting rows, invalid metadata and incomplete subprocesses.

B-BR-SNA-OCNPJ-02: Reuse SNA's approved-correction → usable trade name →
legal name precedence and CPF minimization. Keep ANP's legal source fact
separate, suppress equal secondary names and preserve the station UUID,
coordinates, address, prices and social attribution. A missing trade name
must not invent one. Both public names remain searchable. The daily registry
refresh freezes and fingerprints optional name inputs alongside ANP bytes;
replay, failure and concurrent refreshes preserve the last completed edition.

B-BR-SNA-OCNPJ-03: Public station headings use the canonical display name;
show a distinct public legal name as smaller secondary text, followed by the
address. Keep additive/legacy wire compatibility and offline cached names.
Price values, source provenance and collection periods are independent of
this identity presentation.

B-BR-SNA-OCNPJ-04: Source ownership lookups use an index on the canonical
station UUID. The additive index migration must roll back atomically on
failure, preserve legacy facts and replay without changes. An interrupted
national publication resumes its bound manifest and existing identities.

B-BR-MAP20-DETAILS-01: Expanded map controls group the ANP/Community selector
under the price-source label, show the ANP survey period on its own row and
place route planning in a separate full-width labeled action. Keep keyboard,
48dp targets, narrow screens, both themes and 200% text support. This does not
restore discovery-driven content in the compact bottom action panel.

BUC-SNA-OCNPJ: Enrich a pinned ANP snapshot, validate/replay it on disposable
PostGIS, publish through the existing loader in staging, and compare public
API names with the minimized source. Open the same stations in Android map,
search, community and profile flows, including legal-only and same-name cases.
Deploy tested source through the protected dev → main workflow and verify
the actual deployed revision. Local acceptance does not certify a release.

## Opening observations

Staging has 49,038 directory stations, eight non-null legal-name fields and no
trade-name fields. A public Sorriso sample returns the legal label as
`display_name` and `legal_name:null`. SNA-01 through SNA-06 source support is
already integrated; real enrichment/publication is missing. The local
OpenCNPJ `2026-10-balanced-1.2` snapshot is available. Its exact-CNPJ lookup
contains the establishment trade name and active status. Only its minimized
establishment fields will be retained; the reference repository is read-only.

## Source and minimized admission

The read-only OpenCNPJ reference is
[AlexandreZanata/OpenCNPJ](https://github.com/AlexandreZanata/OpenCNPJ).
Its local `2026-10-balanced-1.2` snapshot declares competence `2026-10` and
source snapshot `D61010`. SHA-256:
`76f36078c1e5b727af8416262309d1285e39f054514218a75a46206d5786037b`.
The offline CLI is used as a preparation tool; no runtime dependency or
remote credentials are introduced. Its MIT option is used under the
repository's MIT OR Apache-2.0 license. Receita Federal remains the data
source; the adapter's descriptor records the dated public source URL.

The current frozen ANP CSV has 45,816 full CNPJs; all matched the snapshot.
45,787 are active and 32,226 have active nonblank names before the existing
placeholder/name policy. The real Rust preparer admitted 45,696 assertions,
with 31,636 usable distinct trade names, and retained 120 quarantined rows.
Mato Grosso contributes 1,171 admitted rows and 1,092 trade names; Sorriso
has 25 admitted rows and 22 trade names. The eight Leverger alias exceptions
and other previously retained historical rows are not deleted or inferred
closed. These are edition counts, not a census of all canonical history.

The minimized name stream is 6,175,546 bytes / 45,816 rows, SHA-256
`c7212221a0b64b131ce3cb146714b7a5a0f43717a0333e02421a273e0875c865`.
It contains only schema version, full CNPJ, trade name, active status and
observation date. Original OpenCNPJ contact/partner fields are discarded
immediately in memory. Raw company responses and personal identifiers are
not committed. The national prepared manifest is
`a69e35113c41a06fd25e7bf1771c57fbfce3ea2b14f770f9733e15761eb5f863`;
its run UUID is `2b52a005-66b4-4def-8b33-22dd1d668ae1`.

## Validation

- Six Python adapter cases pass exact establishment/branch matching,
  minimization, checksum binding, missing rows, malformed metadata/status,
  duplicate/foreign CNPJs and no partial publication on failure.
- Go race tests pass frozen-name replay, changed-edition fingerprints,
  missing/corrupt inputs, traversal/symlink/oversize refusal and the sync CLI.
- Real disposable PostgreSQL/PostGIS tests pass actual Rust v3 admission,
  rollback, incomplete/foreign evidence refusal, source/correction races,
  curated-field preservation, both-name search and public CPF minimization.
  The new feed case passes optional legacy fields, allowlisted addresses,
  CPF-only suppression and unchanged money. Its first fixture used a future
  anchor; pinning the seed to the test reader clock corrected that fixture.
- Migration 156 passes failed-upgrade rollback, recovery, legacy preservation,
  idempotent replay and a selective query plan using the station UUID index.
  The national dry publication exposed two full assertion scans per station
  before this index; the migration changes no source facts.
- The real national manifest loaded/published all 45,696 admitted assertions
  in disposable PostGIS and passed independent read-only verification. Replay
  of the exact manifest completed without new identities or lost publication.
- Android: 71 focused JVM tests pass, including bounded city-name pagination,
  full-CNPJ matching, complete-cache fallback, cancellation, name search,
  public feed compatibility and canonical profile heading precedence without
  changing dated ANP prices. Debug/instrumentation APK builds and lint pass.
- All 23 selected emulator tests pass: 14 map tests and nine detail/community
  tests. Expanded controls remain reachable in both themes at 200% text.
  The physical Android debug APK was updated without clearing user data;
  the real Sorriso map shows aligned source choices, a separate survey period
  and a full-width route action.
- Go vet, sqlc generate/vet, OpenAPI lint, mobile static guards, whitespace and
  secret checks pass. No new dependency is required.

## Repeatable operation and recovery

1. Pin an OpenCNPJ snapshot and the current ANP CSV in a private directory
   outside Git. Record their SHA-256 values before exact-CNPJ export:

   ```sh
   python3 tools/station-prep/scripts/export_opencnpj_names.py \
     --binary "$OPENCNPJ_BINARY" --snapshot "$OPENCNPJ_SNAPSHOT" \
     --snapshot-sha256 "$OPENCNPJ_SHA256" --registry "$NAMES_DIR/registry.csv" \
     --output "$NAMES_DIR/name-inputs"
   ```

2. The adapter queries at most 100 CNPJs per bounded CLI export. Inspect the
   aggregate summary, descriptor and hashes, never dump raw responses. Review
   unmatched/inactive/blank names; they retain the existing legal fallback.
   Do not use fuzzy trade-name or address matching to merge establishments.
3. Prepare with the existing `prepare_registry ... --name-inputs
   "$NAMES_DIR/name-inputs/manifest.json"` contract. Run the existing
   `station-load --publish` and `station-load --verify` against disposable
   PostGIS first. Pin the manifest/streams and keep quarantine visible.
4. After protected integration and the signed staging migration, back up the
   staging DB privately, preserve the previous immutable tool release, install
   the tested executables in a new release directory, and change only the
   registry CronJob's tools path. Place the minimized inputs at
   `/state/name-inputs/manifest.json` with UID1000/private permissions. The
   supervisor auto-discovers this descriptor, copies/hash-binds it into each
   frozen edition and includes its hash in the refresh fingerprint.
5. Run a bounded Job from that CronJob and the existing host/HTTPS guard.
   An interrupted pending run resumes its frozen inputs; changing the mutable
   monthly input cannot rebind it. Missing previously configured inputs fail
   closed. Verify the completed manifest, public city pagination and exact
   CNPJ names; compare identities, points, official prices and community
   counters with the pre-import receipt. Keep approved corrections intact.
6. Refresh the pinned OpenCNPJ edition explicitly when a new reviewed monthly
   snapshot is available. The daily ANP job preserves and reuses that edition;
   it does not silently acquire new RFB datasets. Rollback keeps prior source
   assertions and uses the existing audited source/correction process. Removing
   a name file or overwriting canonical SQL is not a rollback method. The
   additive index can remain during application rollback.

Public `display_name` is the primary label and optional `legal_name` the
secondary fact. Community feed adds compatible `station_legal_name` and
`station_address` fields. Android overlays current canonical names on old
local price rows by full CNPJ, with a complete origin-scoped city cache; it
does not rewrite price IDs, values, dates or source provenance. Equal names
render once, with the known address below.
