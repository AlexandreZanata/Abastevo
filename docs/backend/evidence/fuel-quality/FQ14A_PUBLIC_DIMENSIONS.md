# FQ-14A — Public sample dimensions for Android consumption

Status: LOCAL_DONE
Validation: PASS

Parent: 8c579e4d, maintained dev. B-BR-FQ-01/03/05/07/11/12/13/16;
BUC-FQ-03/04/09. Open the Android journey with a source-separated public API
contract before wiring mobile consumers. Existing summary cells count assays;
they must never be presented as counts of judged samples.

Add a bounded station/fuel/selected-lease projection read to the public summary.
Expose versioned sample/visit/verdict/date dimensions and score=null/UNSCORED.
If no matching projection of the selected revision/methodology exists, explicitly
return unavailable dimensions, never zero samples or rows of another basis.
Historical raw facts stay available even if their derived projection has been
replaced. No request computes/writes a projection or reads private reports.
Include the new representation in ETags so a completed recomputation invalidates
the previously unavailable summary. Existing read kill switch and contracts stay
compatible; no schema/dependency, numeric score, source activation or D scope.

Acceptance: real PostGIS/HTTP before-and-after recompute, selected fuel, other
station, correction/basis isolation, unavailable source and disabled reads;
actual OpenAPI validation and affected read/HTTP race/static/SQL checks.

## Delivered and verified

Summary now carries optional additive `sample_dimensions`, with at most eight
station/fuel rows of the exact selected revision and current methodology. Counts
are separate from assay cells; no matching projection means available=false and
an empty list, never invented zero samples or a numeric score. Derived dimensions
participate in ETags; completed recomputation invalidates a pending response.
Other stations/fuels and replacement revisions cannot leak into that response.
Historical raw cells remain available when their derived row was overwritten.
No serving request computes or writes any derived data.

A new methodology must also get a new recompute intent for an unchanged source.
The prior revision-only dedupe returned an already-completed legacy job. A RED
real-queue test reproduced zero new queued jobs; versioned methodology/policy
keys now create exactly one while preserving the completed job and retry budget.
Existing import→enqueue→apply and replay acceptance remains green.

Commands from backend/, GOCACHE=/tmp/fq-completion-go-cache:

```sh
go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/http ./internal/modules/quality/adapters/read ./internal/platform/apicontract
go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/http -run 'TestSummaryDimensionsCannotCrossStationOrCorrectionBasis|TestQualityActualReadResponsesMatchOpenAPI'
go test -count=1 -race -tags=integration ./internal/modules/quality/adapters/jobs -run 'TestNewMethodologyDoesNotReuseCompletedLegacyRecompute|TestImportEnqueuesRecomputeWithPublishedRevision'
go vet -tags=integration ./internal/modules/quality/adapters/http ./internal/modules/quality/adapters/read ./internal/modules/quality/adapters/jobs
go build ./...
XDG_CACHE_HOME=/tmp/fq-completion-staticcheck GOTOOLCHAIN=go1.27.1 GOCACHE=/tmp/fq-completion-static-cache staticcheck ./internal/modules/quality/adapters/http ./internal/modules/quality/adapters/read ./internal/modules/quality/adapters/jobs
sqlc vet
sqlc generate
```

PASS: real HTTP11.471s/read7.843s/apicontract5.452s race suites; extended HTTP
cases2.757s, versioned jobs2.004s. Actual populated and no-source OpenAPI responses,
disabled routes, staleness and existing revision/cursor contracts remain green.
Build/vet/staticcheck/SQL regeneration stable, format/diff/links/secret review
PASS. No schema/dependency or runtime activation. Initial sqlc parameter spacing
was corrected before successful compilation and real SQL checks.

This closes the backend contract slice only. FQ-14 Android is next; FQ-15/16/17,
FQ-06/07 live source coverage and final protected delivery remain pending; D held.
