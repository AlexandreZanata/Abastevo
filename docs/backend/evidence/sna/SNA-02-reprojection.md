# Station fact re-projection operations

SNA-02C uses retained Directory evidence and the existing durable job queue.
It performs no source download/import or identity creation. The campaign is
an immutable operator/reason request. These commands are implemented; their
presence is not permission to execute them against staging or production.
Obtain the owner's specific approval before any live campaign or live change.

Build the operator and worker from the tested revision. Use the deployment's
normal private configuration/credentials; do not paste a DSN into a command,
log, fixture or this document. Queue a request only with accountable operator
identity and a fresh UUID recorded in the approval:

```sh
ops directory reproject-facts --campaign <uuid> --reason 'SNA-02 approved re-projection' --operator <restricted-operator-id>
ops directory facts-status --campaign <uuid> --operator <restricted-operator-id>
```

Repeating the same campaign/operator/reason converges on the existing request
and first scan job. A changed operator/reason with that UUID is refused.
The worker registers `station-facts-reproject`, with no periodic schedule or
startup enqueue. Each scan considers at most 100 source-backed existing IDs
in UUID keyset order. Its station jobs and next scan are committed together.
A station job chooses the latest eligible complete source when it executes;
station locks and edition checks prevent stale or duplicate effects. Platform
claims use SKIP LOCKED and token-fenced acknowledgement/retry.

Status is a consistent read-only snapshot of scan/station job states. Complete
requires a completed scan and no queued, leased or dead jobs in either group.
A dead job is an unresolved failure; use the normal job incident/replay process
after correcting its cause. This operational count is not a national station
or EV coverage denominator, nor proof that every station has a usable source.
Stations without complete identity-bound evidence are excluded from discovery.
New source arrivals during a campaign belong to normal publication or another
explicit campaign; a campaign is not a frozen national data snapshot.

No source assertion is edited or deleted. Old canonical display/address values
that are curated stay intact. Unknown historical legal names remain NULL;
re-projection does not reconstruct them from a current label. Reviewed location,
status, profiles, comments and observations stay outside this job. Recovery
retries retained work; it does not reset the database or mutate migration history.
