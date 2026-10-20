# FQ-05W3 — Directory-owned source identity and publication fencing

Status: LOCAL_DONE
Validation: PASS

Opened 2026-10-16; completed 2026-10-17 on maintained dev, parent 3a181143.

Before behavior: B-BR-FQ-02/03/04/05/12, BUC-FQ-01/02/03/04. No identity creation,
name/address/proximity matching, business validity inference or module SQL leaks.
Directory's identifier valid_from/valid_to are operational bookkeeping clocks,
not business establishment windows. Read at most two distinct canonical IDs for
an exact full CNPJ; one active identity on an active station may bind dated source
facts under that identifier. Multiple canonical IDs stay ambiguous, retired-only
or inactive identities require succession review, another branch is review only,
missing stays unmatched. Source observation dates remain independent.

Directory owns all source identity SQL and exposes read-only values and a
transaction guard through ports. The guard rechecks bounded expected full-CNPJ/
station pairs and locks current identifier/station rows during quality publication,
including exact replays. Normal owned retirement/reassignment/status transitions
cannot cross the guarded commit. Query execution and guard failures return no
partial result/publication/watermark. Negative links remain inactive and need no
lock to prevent public binding. Lock order is deterministic; prepared data is
re-resolved on retry, never automatically reassigned from stale cached bindings.

Measure historical/full-CNPJ and base-prefix access against synthetic retained
rows before adding any required Directory-owned index. Immediate acceptance:
pure unique/ambiguous/retired/branch/unknown/clock cases; real SQL bounded lookup,
lookup outage, no writes, upgrade/replay/index plan; retirement/status before and
during commit; publisher guard failure and replay refusal/concurrency. No source
worker activation, deployment, scores, D, GPS or iOS change.


## Implementation and acceptance

Directory owns source_identity.sql, read-only application values and
SourceIdentityReader. Full identifiers are kernel-validated; exact reads return
at most two distinct canonical IDs, base lookups only prove another branch.
Neither API carries operational validity timestamps, names or coordinates.
Quality's pure ResolveCurrentIdentity policy keeps one current canonical identity,
repeated rows for that same identity, retired/inactive identity, ambiguous
successors, missing and other-branch outcomes separate. All non-bound outcomes
remain inactive with no station assignment. Observed source date is preserved.

The Directory guard copies/deduplicates expected pairs, sorts locks, uses bounded
512-pair SQL arrays and checks active bindings/station status plus retained
canonical continuity. Real 514-binding boundary and later-chunk retirement tests
PASS. Shared locks stay inside the caller's transaction. Publisher.WithGuard
returns an immutable per-attempt publisher; every publication and replay invokes
the guard before writes. Real retirement/status writers reach a SQL lock barrier,
then complete only after commit; changed identities refuse replay/new publication
without partial editions, new identities or current-pointer drift. Pool outage
and malformed identifiers return closed redacted errors.

Actual RED: the generated exact query scanned all20,001 retained identifiers
(20,000 rows filtered,0.925ms local). Append-only Directory migration102 adds
(kind,normalized_value text_pattern_ops,station_id) INCLUDE(valid_to).
Real101→102 upgrade on retained synthetic rows and migration replay PASS.
EXPLAIN executes the exact traced sqlc query/arguments, not a simplified query;
exact and branch plans use the owned index with no identifier Seq Scan, measured
0.040ms/0.010ms locally. These are local observations, not production SLAs.

SQL generation caught an unsupported multi-array UNNEST form and nested @argument
inference; the final typed single-array ordinality joins/explicit sqlc.arg form
are verified against real PostGIS. No SQL text interpolation or cross-module
imports of Directory's generated query package.

Validation (backend/, GOCACHE=/tmp/fq-completion-go-cache):

- `go test -count=1 -race -tags=integration ./internal/modules/directory/adapters -run TestSource`: new identity, failure, transaction/lock/index cases PASS.
- Full eight-package quality `-race -tags=integration` matrix with the pinned Rust quality preparer: PASS, including publisher guard atomic replay and all schema96–102 upgrade cases. Directory integration cases also passed except the existing daily-sync case initially lacked its required ANPFUEL_TEST_PREPARER_PATH; that exact case was rerun with `/tmp/fq-native-rust/debug/prepare_registry`, together with the new chunk-boundary test, and PASS. No test was skipped/weakened.
- Full Directory unit regression initially hit the socket sandbox; rerun with local socket permission PASS. Quality domain/Directory application/domain race tests PASS.
- Affected vet/staticcheck, API/worker/migrate/ops build, sqlc vet and all-owned regeneration hash comparison, format, whitespace, links and tracked/changed/untracked secret/PII surface review PASS. Only synthetic identifiers/metadata in fixtures and query plans.

Private diagnostic logs: /tmp/fq05w3-directory-red.log, directory-green.log,
critical.log, directory-final.log and index-plan.log under the fq05w3 prefix.
Canonical behavior is in committed tests; temporary logs are not delivery artifacts.

No deployed source activation or complete native worker claim. Next: FQ-05W
file-backed verified fetch/discovery, per-attempt Directory resolution/guard
composition, serial job slots and immutable static Rust worker image. FQ-12–17
and final protected dev→main delivery remain pending; D remains NOT_AUTHORIZED.
