# Abastevo Data

## 1. Purpose

Serve reproducible admitted datasets, methodology and analysis to an organization through scoped product access while preserving public basic access and excluding private company records.

## 2. Scope

F8 includes catalog, regional series, watchlists, comparisons, reports, API keys and metering. The owner admits attributed ANP-derived services under EDR-394. Each deployed Data source digest still requires a recorded review. Private receipt, employee, agreement and track data are excluded from market products.

## 3. Entities and ownership

A regional dataset version pins the enterprise export snapshot, official
survey revision and method version. The catalog checks the latest recorded
commercial review for the exact registry and price digests. A later review
can hold a previously approved dataset. Each catalog revision retains the
state and minimum sample size. The registry pin is S-ESP-01 and the
official price pin is S-ESP-02.

The organization owns its API keys, usage, watchlists and report requests. Key secrets are protected and only safe metadata is redisplayed. A dataset is not the organization's private operational database.

## 4. Business rules

- **BR-DATA-001 — Recorded admission.** EDR-394 permits paid facilitation with attributed ANP open data. The current exact-digest review must admit each input. An objection triggers withdrawal and removal under the [source procedure](../source-admission.md#f8-commercial-rights-decision-emp-gov-016).
- **BR-DATA-005 — Service value and attribution.** Charge for analysis, tools and access. No raw ANP files, bulk replicas, licensing of source data, exclusivity, ownership claims or ANP endorsement. Keep official and community data separate. Show `Fonte: ANP`, the source link and the survey dates or source edition in API metadata, reports and exports. Link the original data for free through ANP. Retain the source URL, collection time and SHA-256 per snapshot.
- **BR-DATA-002 — Exclude private inputs.** Company, employee, document, agreement and location records never enter a market dataset by default (BR-SRC-005).
- **BR-DATA-003 — Reproducible basis.** Series, comparisons and reports include origin, period, coverage and methodology revision (BR-TRUTH-004).
- **BR-DATA-004 — Current key scope.** Issue, rotation, expiry, revocation and quotas preserve the organization boundary and server authorization.

## 5. Use cases

**UC-DATA-01 — Query an admitted series.** A currently authorized key requests an enabled dataset and regional period. The service checks rights admission, key scope and quota, then returns version-bound paginated data and basis. Missing input remains explicit.

**UC-DATA-02 — Rotate a key.** A fresh permitted organization actor issues an approved replacement. The recorded overlap ends at its stated time and the old key is refused afterwards. Audit contains no secret.

**UC-DATA-03 — Request a comparison.** Through the company reports API, a
member holding `rpt.export.run` and `data.usage.read` requests
`data.market_comparison.json`. The parameters name one fuel and two exact
prepared inputs. Each input is an object with `dataset` and `revision` under
`baseline` or `reference`. The normal private export queue delivers a JSON
file with the source attribution, method, survey period and station coverage.
The service checks current plan and source rights at request, generation and
on both download steps. Delivery also requires the original requester’s
current company proof.

## 6. States and transitions

Keys follow active, rotating, revoked and expired states in [the state catalog](../state-model.md). The scoped company API issues and rotates keys after fresh proof, an organization-wide `data.key.manage` grant and the current Data capability. Revocation uses the same current proof and permission after the capability ends. Mutations bind a client operation ID to their command, and only the committed first issue or rotation response includes the bearer. Listing uses company- and member-bound cursors. The normal key proof holds the current operator switch, plan, active organization and key row through consumption. Revocation applies to the next request. Report files use the existing export lifecycle and private expiry. Rights-pending admission remains a hold on the affected dataset regardless of subscription state.

## 7. Roles and permissions

The [Data permissions](../permissions.md) separate key management, consumption and report requests. Key scope cannot widen the organization's admitted capability. Staff account status and personal Premium give no Data key-management rights. Full private organization exports follow the distinct reporting contract.

## 8. Offline, concurrency, idempotency and corrections

API requests, key changes and report generation require server confirmation. Stable query basis and cursors prevent page mixing. Usage and retry identity are explicit. Notification subscriptions and report links recheck current recipient scope.

The company usage view keeps a shared entitlement lock through the selected
plan's allowance, usage and switch reads. A view that waits for a subscription
change uses the committed plan and the server instant after the wait.
Source review writes also serialize with readers. Each recorded decision for
a source digest must be the next consecutive revision, because readers take
the highest revision as the current decision. API results and their quota
receipts, watch notices, comparison checks and prepared-series installation
retain the current decision through their database transaction. A reader
waiting on withdrawal reevaluates the pinned revisions after the decision
commits.

## 9. Events, notifications and integrations

Reuse enterpriseexport source preparation and typed admitted read ports. Usage and report jobs use the existing queue. External source updates never rewrite a stored method snapshot. A member with current Data scope and the organization-wide key management
permission can watch a fuel in an admitted regional dataset. Creation pins
an exact prepared revision with a publishable sample. The queued worker
checks every 24 hours and compares the station means of the newer admitted
revision with the latest survey end. A correction of an earlier week never
replaces the latest survey. It checks current plan and source admission
before publishing. When the last observed revision loses admission, the watch
restarts from the newer admitted revision without a notice.
A changed mean produces one in-app notice for the current permitted member,
with the public source, survey end date and direction. Historical survey dates
remain visible, and rights withdrawal, unavailable samples, lost access and
retries do not produce duplicate or inferred changes.

## 10. Data, formulas and examples

A response states product and units, region, observation period, source coverage, method version and missing-data reasons.
A market comparison returns the exact rational difference of the admitted
station means in milli-BRL per unit. For example, a difference of one-sixth
is encoded as `"1/6"`. Geographic comparisons use the same period, and
temporal comparisons use a later non-overlapping period in the same
region. The format returns `INSUFFICIENT_COVERAGE` for a suppressed cohort or
`INCOMPARABLE_BASIS` when the two supported observations use different
measurement bases. No unsupported aggregation fills gaps or mixes price conditions. An aggregation threshold needs a recorded review before use. No private row becomes market-safe merely by removing its name.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-DATA-01 | PANEL | Manage authorized keys | Permitted organization actor | Safe metadata, issue, rotate and revoke | Fresh authority, one-time secret and visible expiry |
| SCR-DATA-02 | PANEL | Inspect usage and methodology | Permitted member | Quota period, datasets and coverage | No rights-pending sale claim, scoped totals and five shared states |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP17](../backlog/ep17-data.md) owns tasks and source gates. VAL-36 tests private-data exclusion across API, jobs and exports. Exercise revoked or foreign key, pagination scope, quota races, unavailable source and rights-pending dataset before activating any Data surface.

The PostGIS privacy fixture uses the runtime grants and the production registry
and official price readers. It builds the core artifact before and after
submitting and independently approving 500 synthetic private fuelings. A
linked receipt and an admitted recorded track are present for the second
build. The test compares the manifest and all three core files byte for byte
with the same build clock and unchanged public-source evidence. A separate API
fixture covers a foreign key, below-threshold suppression and revoked rights.
Deployment requires the recorded exact-digest approvals and the M8 gate.

### Preparing an approved regional revision

After migration 000294, the migration owner applies
`infra/db/data-rights-role.sql` with `-v data_rights_role=<dedicated-login>`.
Use that login's DSN from protected operator configuration for rights recording.
The approval is tied to the verified core export and the exact official
survey revision. For S-ESP-01, the catalog pins the verified
`stations.jsonl` projection digest. For S-ESP-02, it pins the ANP artifact
checksum of the selected official revision.

Set `SNAPSHOT_DIR` and `OFFICIAL_REVISION` to the reviewed values. Choose
stable per-intent UUIDs `REGISTRY_OPERATION` and `PRICE_OPERATION`, and
set `REGISTRY_REVIEW` and `PRICE_REVIEW` to the next review revision for
each digest. A digest with no prior decision starts at 1.

```sh
manifest="$SNAPSHOT_DIR/manifest.json"
snapshot_sha256=$(jq -er '.content_sha256' "$manifest")
registry_digest=$(jq -er '.files[] | select(.name == "stations.jsonl") | .sha256' "$manifest")
price_digest=$(jq -er --arg revision "$OFFICIAL_REVISION" \
  '.sources.official[] | select(.revision_id == $revision) | .source_checksum' "$manifest")

ops enterprise data rights record --source S-ESP-01 --digest "$registry_digest" \
  --revision "$REGISTRY_REVIEW" --status APPROVED --decision-ref EDR-394 \
  --operation-id "$REGISTRY_OPERATION" --license-ref Decreto-8777-2016 \
  --snapshot-dir "$SNAPSHOT_DIR" --snapshot-sha256 "$snapshot_sha256"
ops enterprise data rights record --source S-ESP-02 --digest "$price_digest" \
  --revision "$PRICE_REVIEW" --status APPROVED --decision-ref EDR-394 \
  --operation-id "$PRICE_OPERATION" --license-ref Decreto-8777-2016 \
  --snapshot-dir "$SNAPSHOT_DIR" --snapshot-sha256 "$snapshot_sha256" \
  --official-revision "$OFFICIAL_REVISION"
```

Keep the operation IDs for retries. PostgreSQL records the login, decision,
digest and time in the immutable review. A withdrawal uses a fresh operation
ID, the exact currently approved digest, its next revision and a withdrawal
decision reference, with `--status WITHDRAWN` and no approval-only flags.

Under the recorded redistribution approvals, register the exact catalog
revision and the plan's dataset scope. The companion rate policy records the
approved request window and maximum for the same plan revision. The plan
catalog's API call limit applies to the current subscription term. No
production policies, plan admissions or datasets ship with the migrations.

Run `ops enterprise data prepare --dataset <key> --revision <n>
--snapshot-dir <path>` with both enterprise and billing enabled. Use a
dedicated ordinary database login with schema usage, SELECT on the source
review and regional catalog tables, and SELECT/INSERT on
`enterprise_regional_series` and `enterprise_regional_series_samples`.
The API login only needs to read those projections. The preparer reads a
pinned core enterprise export, verifies its manifest and file digests and
publishes the aggregate in one transaction. A matching retry converges on its
stored digest.

Consumers send the scoped bearer in the Authorization header and the exact
catalog revisions to the regional series API. Request IDs bind the selected
page. Rate usage carries across key rotation. A repeated matching request
does not add another subscription-term call. The response describes the
official survey period, inputs, condition and sample coverage. Unavailable or
withdrawn sources prevent the call from committing. An admitted period with no
publishable fuel sample returns `INSUFFICIENT_DATA`.
