# EMP-FUEL-001 — Exact pure supply event and owned products

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Continue full F2 then F3 from tested/pushed EMP-METER-001 commit05c83376 on
owned codex/enterprise-f2. Nine checkpoints are LOCAL_DONE / INTEGRATION_PENDING;
55 audited nodes remain. This task was already in the audited cross-phase C1
closure. Its actual PLAT-009/FLEET-001 dependencies are done; METER-001 scoped
references are now also available. Runtime fleet persistence needs the real
fuel-products-v1 catalog; public cooking-gas/absent additive/electricity or
synthetic CAT_A/B descriptors cannot replace that owner. Complete the whole
bounded pure FUEL-001, not only a catalog, before dependent runtime work.
Full F3 remains queued beyond the required C1/F2 dependency closure.

B-BR-EMP-137..143, BUC-EMP-69..72 and EDR-091..094 precede behavior. Recommended
decisions: nine fixed products; exact required candidate fields and explicitly
declared fill/payer/payment; meter reference or missing reason; independent
establishment/contracted supplier and a separate DOC issuer read projection;
category/unit-safe widened purchased totals. Add only declaration validation
in the existing time kernel so an offline candidate requires no fake server
receipt. Preserve Capture behavior with meaningful affected regressions.

One bounded task, shared strict Go/Kotlin vectors and actual RED→GREEN. Root
owns docs/commit/backup push. The Kotlin owner holds the sole Gradle lease;
root retains exclusive emulator ADB if later needed, but this pure task needs
no device/database/migration/route or producer. Synthetic-only receipt/legal,
provider, authorization, actual business persistence/review and F2/M1/M2/C1
and full F3 gates remain open. No whole-task PASS is claimed at opening.

## Profile decisions before oracle freeze

Use the existing L/M3/KWH unit tokens, storage ML/MILLI_M3/WH and family
ETHANOL/GASOLINE/DIESEL/NATURAL_GAS/ELECTRIC/NONE. NONE is the absence of a
fuel family for ARLA_32, not a consumable family. Original source-price text
retains the accepted opaque/verbatim/no-cap profile. Establishment NOT_FOUND
names and known issuer labels are nonblank/well-formed/control-free; no numeric
name ceiling is specified, and the280-character reason/notes rule is not
silently applied to names.

DOC supplies no stable issuer identity yet. Its independent transient view
therefore carries source DocumentRef, positive source revision and issuer label;
it never invents a SupplierRef or copies issuer truth onto Candidate. Pure
source values do not prove an actual document link or read authority. Optional
absence remains absence. The required candidate is ready-field validation only,
not a draft checklist or new workflow.

## Declaration/catalog/candidate RED

The Go declaration-only entry first executes three focused methods against a
compiled stub; all three fail0.002s, with no skips. After extraction, the full
accepted time package plus new declaration cases passes under -race: six
top-level/105 test nodes1.026s. Existing receipt/reporting behavior is retained.
Logs /tmp/enterprise-fuel001-go-declaration-{red,green}.jsonl.

The owned-catalog stub run also executes all nine known-product subcases:
nine fail along with their parent0.002s while the closed unknown-product
baseline passes. The descriptor implementation follows that actual RED.
Three new candidate/roles/totals positive methods then fail on typed constructor
stubs without a compile failure. No full-candidate GREEN is claimed yet.
Retain /tmp/enterprise-fuel001-go-{catalog,candidate}-red.jsonl.

Both owners freeze the minimal pure failure vocabulary INVALID, SCOPE and
PRODUCT_UNIT_MISMATCH. Incompatible/missing grouping inputs are INVALID;
empty per-product input returns no groups, never an invented compatible unit.
No new business sum ceiling, payment matrix or origin/supplier equivalence.

## Actual source dependency corrections

Record FUEL-001 → completed METER-001 because the implementation now reuses
its typed ReadingRef/Missing rather than inventing a second reading model.
Record FLEET-002 → FUEL-001 because the real owned product catalog is necessary
for its accepted catalog-backed asset/tank validation. These source-backed
edges introduce no cycle, additional node, phase acceptance or new behavior;
the audited64-node closure already contains both. Regenerate the canonical
plan indexes from those task fields. Keep the full F3 queued while consuming
its explicitly pulled-forward C1 prerequisites.

## Frozen shared profile and boundary review

The final oracle freezes200 cases: CATALOG1, PRODUCT15, ENUM38, QUANTITY27,
CANDIDATE91, ROLES15 and TOTAL13. SHA256
bca4934b4a21935cd1a1728c087fadef5d0935fa39f596b261f844464685395a.
Every row/header/namespace/unique ID/field map/UTF-8/count/hash is explicit.
The earlier201-row draft contained an invalid ENUM kind selector as a supposed
domain error. Move that negative to strict loader grammar validation, preserving
actual unknown field-token tests and adding the focused loader regression.
The temporary Go generic token dispatcher is removed before publication;
its diagnostic RED/GREEN logs are historical, not accepted business behavior.
The final Go200-row consumer/grammar regression passes. No production parser
is introduced solely to interpret test selectors or DOC revision text.

The peer also catches an actual Kotlin unit-entry mismatch: decoding FuelUnit
before comparing it to a known product maps unknown/empty/lowercase units to
INVALID. B-BR-EMP-139 and the shared profile require PRODUCT_UNIT_MISMATCH
for a differing supplied unit. Capture focused RED before comparing the raw
unit against the owned descriptor. No product aliases or normalization are added.

The final Go affected union passes -race across all four packages: fueling14
methods/250 nodes1.080s (91.3% statements), orgtime6/105 nodes,1.024s (100%), fleet10/
143 nodes,1.029s (98%) and meter11/314 nodes,1.151s (92.8%). Total41 top-level/812 nodes,
zero failures/skips. All200 shared fueling cases and the relocated private
loader negative execute. Gofmt, vet and staticcheck pass all four scopes;
independent Go source/consumer review is clear. Final eleven-file manifest:
/tmp/enterprise-fuel001-go-source-final.sha256, SHA256
a394b69d2506fdf7f1d86b563d80f269aee31be52a0785cd5dc0ac0f9c1e7ee7.
Log /tmp/enterprise-fuel001-go-final.jsonl and coverage
/tmp/enterprise-fuel001-go-final-coverage.out retain the exact final inputs.

Kotlin's actual unknown-unit RED fails1/1 on assertions0.075s, zero errors/skips
(Gradle11s), before the raw-unit comparison correction. Its first complete
318-case run executes315 passes and three failures, all in the shared reader's
absent DOC-issuer marker: the model correctly returns no projection, but the
adapter writes empty hex instead of the oracle's explicit '?'. Fix the reader
without changing the expected maps or main behavior. A separate malformed
fixture UTF-8 decoder negative must fail at the grammar boundary, escaping
rather than being converted into a successful expected domain INVALID result.
No whole-task PASS is claimed before final Kotlin execution/review.

## Final local acceptance

Final Kotlin enterprise regression passes1158 cases, zero failures/errors/
skips (Gradle13s). It includes Fuel214 (200 shared+13 local+one strict grammar
method) and Time104 (101 prior+three declaration-only cases). The grammar
method preserves unknown-selector and malformed UTF-8 rejection; original
business-negative maps stay intact. The absent-issuer marker and unknown-unit
bugs are corrected. Final main/consumer peer review is clear.

New fueling-package scoped coverage is2047/2130 instructions(96.1033%),
125/136 branches(91.9118%) and145/148 lines(97.973%). No filter, threshold,
required check or test is weakened. This does not certify historical whole-
module coverage. Final seven-file Kotlin manifest SHA256
dce9cb56f0f17b8d226573570bceba8a40f0b83c761d8af518f3802a81cbb24b
at /tmp/enterprise-fuel001-kotlin-final-source-manifest.json. Retain
/tmp/enterprise-fuel001-kotlin-enterprise-final.log.

Combined18-file code/test/oracle/quick-manifest SHA256 is 63645a49132592e21347466ddb362015a63a6b768e6d90f02e5ef0dadf22a698,
record /tmp/enterprise-fuel001-source-final.json. Actual input hashes match;
plan consistency348 tasks/545 rules/167 use cases/145 screens and zero problems,
final diff whitespace, staged secret scan and gitleaks pass. No dependencies,
SQL, route, intake handler, app screen or runtime producer are added. Owned
fuel-products-v1 now supplies the real Fleet ProductCatalog. Current runtime
operation registry remains empty. Source/legal/provider/staging/real-data/
release gates remain separate; no full F2/M1/M2/C1 or F3 acceptance is inferred.

Ten audited checkpoints are LOCAL_DONE / INTEGRATION_PENDING;54 nodes remain.
Next bounded task: EMP-FLEET-002 actual scoped assets/tanks/allocations/commands.
Use private persisted explicitly declared categories, actual org unit admission,
current authorization and immutable command receipts. Preserve employees/team
unit counts and keep powered activation dependent on real METER-002 source,
with no fake meter or self-assignment provider. The cumulative protected dev→
main PR remains the final delivery path; no construction CI polling/merge.

Final commands, backend cwd for Go and root for JVM, with installed/pinned
Go1.27.4, Go1.27.1 staticcheck and JDK17:

```sh
go test -race -count=1 -coverprofile=/tmp/enterprise-fuel001-go-final-coverage.out -covermode=atomic -json ./internal/modules/fueling/domain ./internal/modules/orgtime/domain ./internal/modules/fleet/domain ./internal/modules/fleet/domain/meter
go vet ./internal/modules/fueling/domain ./internal/modules/orgtime/domain ./internal/modules/fleet/domain ./internal/modules/fleet/domain/meter
GOTOOLCHAIN=go1.27.1 STATICCHECK_CACHE=/tmp/enterprise-fuel001-go-static-cache /home/iiii/go/bin/staticcheck ./internal/modules/fueling/domain ./internal/modules/orgtime/domain ./internal/modules/fleet/domain ./internal/modules/fleet/domain/meter
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline --no-daemon --no-parallel --max-workers=2 --console=plain
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

GOCACHE=/tmp/enterprise-go-build; Go/static logs have the final-jsonl/vet/static
names above. No unchanged aggregate suite is rerun for evidence-only closure.
The next useful task records the actual atomic commit/backup result.
