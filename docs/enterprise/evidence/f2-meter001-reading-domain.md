# EMP-METER-001 — Exact immutable meter evidence

Date: 2026-11-14.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Continue the authorized full F2 then F3 from EMP-OFFL-004 commit ea42ef9d,
backed up at origin/codex/enterprise-f2. Eight checkpoints are LOCAL_DONE;
56 audited nodes remain before this task. F2/M1/M2/C1 and F3 stay unaccepted.
The sole bounded task is the dependency-ready meter/reading pure domain,
using accepted FLEET-001 references and the existing time/exact-value kernels.
B-BR-EMP-131..136, BUC-EMP-65..68 and EDR-087..090 precede behavior.

Recommended decisions: strict decimal inputs with exact integer conversions;
separate hour/minute arguments; org-bound evidence; immutable readings plus
appended correction/void facts; supplied SUSPECT classification only; stable
observation/receipt/ID order and actual latest-valid anchors in one segment.
No routes, migrations, device work, speculative service or new dependency.
Use shared synthetic Go/Kotlin vectors with meaningful RED→GREEN and local
adversarial/property/static checks. Root coordinates one Gradle owner and
atomic commit/backup push. No whole-task PASS is claimed by this opening.

Before RED vector freeze, resolve the correction-operation ambiguity using the
recommended single logical command: fresh correction operation O2 owns new
reading R2/O2 and fact F2/O2; R1/O1 stays immutable. The two composite parts
share O2, while subsequent independent reuse is refused. This avoids inventing
two commands for one supersession. Per-meter replay does not prevent one trip
operation carrying both odometer and hour-meter observations. Existing fleet
metadata requires nonblank reasons up to280 characters without invalid Unicode
or controls. Opening evidence must be observed, never a missing placeholder.

## Conversion RED

Kotlin discovers all six new conversion/presence JUnit methods against actual
compilable stubs: five fail on missing positive behavior and the strict grammar
denial baseline already passes; zero errors/skips,0.113s (Gradle11s). The first
sandbox invocation reached no tests because the shared Gradle lock was outside
writable roots; the authorized cache-access rerun provides the actual RED.
Retain /tmp/enterprise-meter001-kotlin-values-red.log. No GREEN or whole-task
acceptance is inferred at this point.

Go actual focused RED compiles the new package and executes three top-level
methods/eleven outcomes including subcases. All eight positive exact-conversion
subcases and their parent fail, the reading/supersession method fails at missing
meter construction, and the closed-stub refusal baseline passes; elapsed0.003s.
Retain /tmp/enterprise-meter001-go-red.jsonl. Both owners agree declared-time
presence must be explicit: missing declaration is rejected, while an explicitly
supplied bounded year0001 observation follows the existing time kernel. Unset
server receipt remains invalid. No arbitrary minimum observation date is added.

The initial Go GREEN passes its three top-level/eleven outcomes0.004s after
pure implementation; /tmp/enterprise-meter001-go-initial-green.jsonl. Kotlin's
first six conversion/presence methods also pass (Gradle13s). These preliminary
results do not accept series, full shared vectors or the task. Root independently
checks the documented arithmetic constants using decimal arithmetic before
shared-oracle comparison. Final adversarial/copy/format review remains required.

## Time metadata RED and correction

Independent adversarial Go execution reproduces a valid explicit minimum
observation at0001-01-01T00:00:00Z with entry America/Cuiaba and a valid server
receipt being refused. NewReading had passed the device entry zone as the
organization reporting zone to Capture, manufacturing a year0 reporting day.
The reading owns no reporting-day snapshot. The recommended correction uses
UTC solely for existing kernel validation and retains the original entry zone
and offset, without inferring organization reporting settings. B-BR-EMP-132
now states that distinction. This is the sole failure of the nine Go
methods in that run (including the six new adversarial methods); retained log
/tmp/enterprise-meter001-go-adversarial.jsonl. The actual RED precedes the fix
and receives a shared negative-entry-zone minimum-time conformance case.

## Frozen oracle and evidence RED

The synthetic shared corpus freezes261 cases: VALUE99, METER14, OPEN_SET8,
READING54, PRESENCE11, HISTORY56 and USAGE19. SHA256
578b99a926e5591aea9bcf5ed98d6e7529301c672c51e6353d87c09f3f55147a.
Its strict README pins aliases, explicit defaults, complete maps and counts;
base units are lowercase m/s. Missing-start usage retains the actual available
end anchor. Pure open-set validation checks supplied in-service facts without
claiming persistence or reserving a meter. Corpus structure/header/unique-ID/
unique-field/count validation passes before both consumer runs.

Kotlin then executes all ten evidence/series methods against compiled stubs:
ten assertion-path failures, zero errors/skips0.154s (Gradle12s), retained in
/tmp/enterprise-meter001-kotlin-evidence-red.log. Immutable references, metadata,
ordered anchors, terminal facts and series behavior follow this actual RED.

## Final bounded peer review

The independent review finds no arithmetic, reference, supersession, immutable
snapshot or anchor defect in the GREEN main implementations. It identifies
two real evidence/API gaps to close before acceptance: Go's shared reader
itself refuses four wrong-kind/unknown-form VALUE rows, rather than exercising
a domain entry point; Kotlin's public Reading constructor silently defaults
classification to VALID despite the explicit supplied-classification contract.
Take the recommended narrow correction: add a pure entry dispatcher over the
existing exact converters in both languages and delegate all VALUE semantics
to it, with actual focused RED before validation. Remove the Kotlin model's
classification default; fixtures may still explicitly supply their documented
synthetic default. This introduces no plausibility policy, wire route, service
or dependency. Final tests/manifests follow those concrete changes.

The initial complete Go corpus run passes all261 rows, ten top-level methods/
304 test nodes0.020s. Its first fixture-opening attempt used the wrong relative
path and executed no shared rows; the corrected six-parent path loads every
case rather than skipping the fixture. Root does not count that path failure
as domain RED. Before the final switch, ParseEntry receives its own actual
nine-case focused RED: seven subcases plus their parent fail0.002s while two
closed-denial cases pass. Retain /tmp/enterprise-meter001-go-entry-red.jsonl.

The peer also identifies a concrete Kotlin defensive-copy gap: OpenSet validates
the caller list in multiple passes and takes its returned snapshot afterward.
A changing AbstractList can return valid organization A on those checks and
foreign B when copied. The bounded recommended fix snapshots once before
validation and returns only the checked snapshot. Actual RED/final GREEN must
precede checkpoint acceptance. This changes no in-service or authority policy.

The final Go release after the public-entry correction passes11 top-level/
314 test nodes under -race1.121s, including the unchanged261 shared cases,
24 deterministic order permutations and16 parallel immutable forks. New-package
statement coverage is92.8%; gofmt, pinned vet/staticcheck and scoped whitespace
checks pass. The COW regression explicitly exercises spare backing capacity;
all clock inputs are fixed. Final Go source manifest SHA256
467548ebe8dec1eaa8badcf56cfb07b46655a19f12c80293975748b80ee0592b,
at /tmp/enterprise-meter001-go-source-final.sha256. Independent read-only review
finds no remaining concrete Go source/consumer blocker.

Kotlin actual shared-entry RED discovers every261 row:133 fail before the new
entry behavior,128 closed baselines already pass, zero errors/skips0.365s.
The changing-list regression separately fails1/1 on assertions0.043s before
the snapshot correction. Its current complete meter GREEN then passes278
cases (261 shared+17 local), zero failures/errors/skips. The last affected
enterprise run and final source manifest remain pending before closure.

Initial scoped Kotlin coverage highlights the same unexecuted Nothing-return
helper compiler paths already repaired in FLEET-001. Reuse that established
exception-factory/explicit-throw refactor inside this new package; do not add
trivial generated-copy tests or change filters/thresholds. Revalidate the actual
source change and report real scoped coverage, keeping the whole historical
Kotlin gate distinct. This is a bounded post-GREEN refactor, not extra policy.

## Final accepted local checkpoint

The final Kotlin enterprise regression passes941 cases, including meter278
(261 shared+17 local), zero failures/errors/skips after the explicit-throw
refactor (Gradle14s). No test, filter or threshold is weakened. Actual scoped
meter-package coverage is2288/2386 instructions(95.8927%),184/210 branches
(87.6190%) and187/191 lines(97.9058%). This is not the historical whole-domain
module coverage acceptance. Final log
/tmp/enterprise-meter001-kotlin-enterprise-refactor-final.log. The five-file
Kotlin source manifest SHA256 is
1b7c2014b82c17755881ab046edcf72877a639e7b015b667acee01070fb7c425,
at /tmp/enterprise-meter001-kotlin-final-source-manifest.json.

Final independent Go/Kotlin main and shared-consumer reviews are clear:
explicit classifications, exact typed entry, actual checked snapshots, copied
metadata, retained terminal chains and real available usage anchors. The new
Go package is registered in the existing quick manifest. Shared corpus SHA261
remains578b99a926e5591aea9bcf5ed98d6e7529301c672c51e6353d87c09f3f55147a.
Plan consistency348/545/167/145 with zero problems, final diff/secret review
and source-manifest verification pass. No SQL/device test is implied or needed
by this pure-domain change; no migration, route, intake handler, producer,
permission, real meter/asset or consumption interval becomes active.

The combined16-file code/test/oracle/quick-manifest record is
/tmp/enterprise-meter001-source-final.json; its SHA256 is e25fe589460364f7549c9de6f1edd290889b701219d22a3129d718b6987d8fa8.
Nine checkpoints are now LOCAL_DONE / INTEGRATION_PENDING;55 audited nodes
remain. Full F2/M1/M2/C1 remains unaccepted and full F3 remains queued.

Next dependency-ready task: EMP-FUEL-001, already in the audited C1 closure.
Read-only source review confirms its owned fuel-products-v1 catalog is needed
before runtime fleet persistence; kernel cooking-gas products and CAT_A/B
fixtures cannot be used as a production catalog. Complete/validate that whole
pure fueling task before FLEET-002. Category vocabulary has no canonical enum;
record a faithful private persisted-declaration source at FLEET-002 opening,
rather than inventing category/type rules. Powered activation remains closed
until the real METER-002 source. This preserves actual dependency-first work;
it does not start or accept the whole F3 early.

Executed commands (from backend for Go, repository root for JVM):

```sh
go test -race -count=1 -coverprofile=/tmp/enterprise-meter001-go-final-v2-coverage.out ./internal/modules/fleet/domain/meter
go vet ./internal/modules/fleet/domain/meter
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-meter001-go-static-cache staticcheck ./internal/modules/fleet/domain/meter
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' --offline --no-daemon --no-parallel --max-workers=2 --console=plain
python3 docs/enterprise/tools/check_plan.py
git diff --cached --check
git diff --cached --binary | gitleaks stdin --redact --no-banner
```

Use pinned Go1.27.4/GOCACHE=/tmp/enterprise-go-build and installed JDK17 as
in preceding phase records. Scoped-only meter RED/GREEN commands select
`com.anpfuel.domain.enterprise.meter.*`. Logs and exact reviewed input manifests
above preserve intermediate failures without another aggregate acceptance run
for record-only edits. Protected dev→main integration remains pending.
