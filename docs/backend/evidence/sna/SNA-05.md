# SNA-05 — Station names and addresses on Android

Status: IN_PROGRESS

Bounded children: A domain/API decoder; B retained Room cache and both-name
search; C list/map/capture/detail presentation, locales and visual acceptance.

B-BR-STATION-MOBILE-05A: Keep the server display name primary and the optional
legal name verbatim as a fact. Display a secondary legal name only if it
meaningfully differs after case/whitespace normalization. Both names are
searchable; an absent legal name adds no invented fallback. Decode additive
address source/name fields for old/new servers; unknown source stays unknown.

Format the address from an explicit component allowlist, with municipality
name/valid UF as fallback. Ignore municipality codes, map coordinates, extra
fields and non-text payloads. A legacy readable raw address may supply the
street line when components are missing; it cannot be a coordinate pair.
Only a known non-ANP source can carry the approximate address label. Keep the
coordinate pair separately for explicit copy/navigation actions in the UI.

BUC-SNA-05A: Prove full and partial formatting, old wire payloads, optional
name/source fields, same-name suppression, both-name search and coordinate or
code-only refusal before changing the domain. Preserve old create callers via
defaults. No Room schema/UI behavior change in this bounded child except
consumers already using the domain address label; those surfaces are audited
in the later presentation child.

## SNA-05A checkpoint

Status: LOCAL_DONE
Validation: PASS

Tested parent 404a560a plus this child. RED reached the missing domain
fields/behavior after resolving local SDK configuration. GREEN: all 883 domain
unit tests and all eight directory codec tests pass, including synthetic
full/partial/legacy addresses, both names, whitespace/case equality, absent or
future source and non-text optional fields. Canonical coordinates remain
separate. The address-line formatter uses only explicit text components and
rejects coordinate literals and municipality codes; capture's existing domain
address-label accessor uses the same formatter. Unknown sources stay unknown.

Compilation used the installed SDK/NDK and full JDK 21, Gradle offline, with
Kotlin compilation in process. Diff and secret review pass. No dependency,
Room schema, server API, SQL, UI resource or staging change in this child.
Next: preserve the optional fields in the Room cache (current schema 32), and
implement remote/offline both-name search without searching private CPF tokens.
Then finish the presentation, eight locales, ViewModel/Compose tests and the
light/dark proof. No UI/device acceptance is claimed here.

## SNA-05B1 opening

B-BR-STATION-MOBILE-05B1. Preserve the optional legal name and address source
in the directory's scoped Room cache. Existing rows retain their addresses
and read both new fields as unknown. Keep scope, position and identity intact.

BUC-SNA-05B1. Round-trip both fields through the cache and verify migration
from schema 32 with an existing station row and a recorded page cursor.

Status: LOCAL_DONE
Validation: PASS

Room 32 → 33 adds nullable legal_name and address_source columns to the
station cache. The scoped round-trip initially failed because both facts
were discarded. All 15 cache unit tests now pass. The instrumented migration
passed on a disposable Android 35 emulator with the generated schema bundled
in the test APK. The fixture verifies the old address, scope, row position,
cursor and null defaults. The mobile static gate and secret review pass.

## SNA-05B2 opening

B-BR-STATION-MOBILE-05B2. Search the public portions of the display and legal
names. The directory must not reveal a removed CPF token through result
membership. Preserve city filters, escaped query text and cursor ordering.

BUC-SNA-05B2. Exercise both name matches and CPF exclusion on real PostGIS.
Use the same name matching in the Android map's offline search.

Status: LOCAL_DONE
Validation: PASS

The real PostGIS regression first failed on a legal-name-only query. The
directory now applies the public CPF-token rule before comparing either name.
The fixture verifies that adjacent CPF-shaped tokens and formatted tokens
cannot affect result membership. The directory read and HTTP integration
suites passed with race detection. SQLc generation and vet passed. All 26
map ViewModel tests passed, including a network failure followed by offline
legal-name search with the trade name retained as the result label.
