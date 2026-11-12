# PR202 repository-port failure contracts

Status: LOCAL_DONE
Validation: PASS

The required Quick verification job 130869611343 (run 43123498846) on
PR202's merge of source 0474dac5 and base ef96cd13 refused the organization
application package because it had no behavioral tests. Both organization
and membership application packages declare repository ports; their
production adapters already have behavioral and PostGIS coverage, but the
port consumers had no explicit failure-result contract suite.

Added external application tests using the real adapters through both normal
and explicit-transaction interfaces. Unavailable storage and a missing
transaction return no organization, employee, membership candidate or
successful write. No mocks, database, test skip, workflow, manifest or gate
change is involved. Existing adapter assertions remain intact.

Validation: GOCACHE=/tmp/enterprise-go-build go test -race
./internal/modules/organization/... ./internal/modules/membership/...
passed all six package suites. Vet, strict format, diff and secret checks
are recorded with the commit. This is scoped failure-contract evidence,
not a full Quick verification or green remote CI claim.

The prior cumulative PR description is preserved verbatim in
[the archived body](pr202-body-0474dac5.txt); the live description can now
link to canonical evidence without exceeding GitHub's body limit.
