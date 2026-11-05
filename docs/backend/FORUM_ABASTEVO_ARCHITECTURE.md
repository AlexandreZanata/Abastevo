# Forum Abastevo architecture and proposed API

Status: PROPOSED / NOT IMPLEMENTED. Baseline: GitHub main
`3fc77acc6d3dc85819cbc9d00cadaec056448d0a`, reconciled 2026-11-05.
Behavior owner: [product contract](../product/FORUM_ABASTEVO_CONTRACT.md).
Implementation order: [FOR-00–FOR-29](../planning/FORUM_ABASTEVO_PLAN.md).
No schema, OpenAPI or executable code is changed by this planning package.

## Architecture choice and alternatives

Use a new `backend/internal/modules/forum/{domain,application,adapters}` module
in the existing monolith, composed by `backend/cmd/api/main.go` and
`backend/cmd/worker/main.go`. Pure domain owns post/comment state and text rules;
application owns ports/use cases; adapters own pgx/sqlc, HTTP, source parsing and
jobs. No new process, database, provider, broker or independent admin site.

- Extending `community`: smallest folder change but mixes price observations,
  consensus/trust and station projection with editorial/news lifecycles. News
  changes would affect the critical price module. Reject for this baseline.
- Separate `forum`: adds a bounded domain and tables, isolates imports and
  failures, and reuses concrete platform/account/moderation ports. Chosen.
- Shared generic discussion engine: could eventually serve multiple domains,
  but feedback has station/fuel targets, 280-scalar text, VALID/INVALID votes and
  business attribution. A generalized migration now adds risk without a second
  compatible proven consumer. Defer; do not move existing comments or votes.

Compose cross-module dependencies through narrow functions/interfaces in the
composition roots, following current feedback wiring. Never import another
module's adapters into Forum domain/application. A tiny existing transport helper
may be reused; do not build a new generic platform as a prerequisite.

Data flow: Android → Forum HTTP → application ports → Forum SQL. Signed writes
add existing account session + identity proof/binding and idempotency adapters.
Reports join existing Moderation cases. Existing admin → Forum review adapter →
same Forum transaction + admin audit. Existing worker/jobs → bounded source
adapter or deterministic bulletin builder → draft → explicit review → public
projection. External collection never runs inside a public request.

## Evidence and reuse constraints

- `community/application/feed.go`, `community/adapters/read/feed.go` and
  `db/queries/community/feed.sql` implement the station price feed, not Forum.
- `feedback/application/comments.go`, `feedback/domain/feedback.go` and
  `db/migrations/000024_feedback_comments.sql`: station/fuel FK, one reply level,
  owner CAS and 280-scalar limit are evidence for patterns, not reusable rows.
- `identity/adapters/auth/auth.go` verifies signed method/authority/path/query,
  body digest and single-use nonce. `identity/adapters/idempotency.go` wraps
  reservation/business/outcome in one pgx transaction; its owner is a contributor
  UUID and its replay TTL is seven days (`identity/domain/identity.go`). It is
  **not** an arbitrary account idempotency store.
- `moderation/domain/case.go` has no `FORUM_POST`/`FORUM_COMMENT` target yet.
  `moderation/adapters/actions.go` records actions and enqueues downstream intent;
  `cmd/ops/moderation.go` is the restricted operator path. Extend validation,
  database checks and dispatch together; adding an enum alone is insufficient.
- `admin/adapters/http/http.go`, `admin/application/service.go`,
  `admin/adapters/store.go`, `admin-web/src/{api,app}.ts` provide session, CSRF,
  Origin/CIDR fencing, idempotent transaction and audit patterns. Reuse these for
  Forum editorial/review controls. `staff/domain/catalog.go` explicitly says
  roles/badges confer no permission; do not turn `community_moderator` into an
  unimplemented authorization shortcut.
- `platform/jobs/{queue,schedule,dispatch}.go` and worker composition provide
  durable retries, leases and deduplicated recurring schedules. Preserve the
  existing safety dispatcher isolation and limits.
- `account/application/erasure.go`, `account/adapters/erasure.go` and actual
  `cmd/worker/main.go` compose account cleanup using bounded callbacks. Add a
  Forum port/callback and completion dependency; price-contributor erasure alone
  cannot clean account-authored Forum data.
- `official/application/queries.go` and `db/queries/official/{revisions,ranking}.sql`
  expose revision-aware data and exact prices, not a weekly state bulletin API.
  Build a bounded aggregate read owned by Official if the required aggregate is
  absent; Forum must not infer statistical representativeness from a price row.
- `alerts/application/ports.go` and `subscription/application/ports.go` exist on
  this main. Alerts are optional later integration; subscription is not a gate
  for free Forum. Neither proves an operational Forum notification provider.

All paths in the preceding list are relative to `backend/internal/modules/`
unless prefixed by `db/`, `platform/`, `cmd/` or `admin-web/` (backend directories
except `admin-web/`, which is repository-root). The execution plan provides
clickable full repository-relative links for the primary evidence.

## Proposed schema (design only)

Allocate migration numbers from actual latest main **at implementation time**;
the reconciled main reaches migration 000156. Do not reserve a fixed next number.
Use timestamptz UTC, UUID IDs generated with existing facilities, checked bounded
text/enums, parameterized SQL and explicit transactions. No executable DDL here.

### FOR-03: editorial foundation

1. **`forum_sources`**: UUID PK; unique stable `code`; display name; source kind
   (`ABASTEVO`, `GOVERNMENT`, `PUBLISHER`, `DATASET`); exact HTTPS origin/path
   allowlist; optional fetch URL/format; collection/publication flags default
   false; `rights_status` (`PENDING`, `APPROVED`, `DENIED`); separate allowed
   title/excerpt/image modes; terms URL, terms SHA-256, checked time, review
   expiry, policy version, attribution text and administrator audit reference.
   Store approved public configuration only, never credentials or unrestricted
   arbitrary URL patterns. One Abastevo source is a migration seed; no fake news.
2. **`forum_posts`**: UUID PK; kind/category; nullable `author_account_id` FK
   accounts ON DELETE RESTRICT, required for non-removed TOPIC; `source_id` FK required
   for every non-TOPIC; title, excerpt, plain body; canonical HTTPS URL; source
   external ID/hash; source-published/source-updated timestamps; local
   created/updated/first-published timestamps; immutable `first_publication_seq`
   on first publication; `publication_state` (`DRAFT`, `PENDING_REVIEW`,
   `PUBLISHED`, `REJECTED`, `REMOVED`); independent `visibility` (`VISIBLE`,
   `HIDDEN`); revision >=1; nullable coarse UF/IBGE/station/fuel; nullable
   author operation UUID/logical-body hash; rights snapshot/version; nullable
   bulletin key/provenance and `supersedes_post_id` self-FK. Kind-specific checks
   reject false authorship/source combinations. Image is absent in baseline.
3. **`forum_feed_state`**: singleton checked PK; monotonic bigint `revision`.
   Lock it when inserting/publishing/editing/hiding/removing a public post or
   changing public source metadata; bump in the same transaction. Initial
   publication gets that revision as `first_publication_seq`. This small row
   provides committed publication order and cheap cache invalidation without
   copying source publication dates into ordering. Do not bump for comments:
   feed summaries omit mutable comment counts. Benchmark lock wait before
   considering sharded epochs; it is a conscious low-write-volume tradeoff.
4. **`forum_changes`**: UUID PK; target kind/UUID; revision; action; actor kind
   and nullable account FK/admin audit reference; policy version; time; safe
   reason code; prior/new state. Unique target/revision/action key. Append-only transition
   metadata audit (privacy redaction/expiry still apply); never store historical bodies, aliases, raw report details,
   tokens, request dumps or URLs with secrets. Public detail exposes only latest
   revision/update/correction metadata. Admin audit remains authoritative for
   administrator actions; this records domain transitions, not a second admin
   identity or session system.

Indices: visible published feed `(first_publication_seq DESC, id DESC)` partial
on PUBLISHED/VISIBLE; same with category prefix; source external ID partial
unique `(source_id, external_id)` and canonical URL hash partial unique
`(source_id, canonical_url_hash)`; owned `(author_account_id, created_at, id)`;
unique `(author_account_id, create_operation_id)` for TOPIC; unique bulletin
key when present. Index additions for UF/municipality are FOR-24 after EXPLAIN,
not every possible filter combination. Public queries always join current
source rights/publication eligibility, not a stale cached permission bit.
TOPIC/comment public queries also require a currently active author account;
account deletion/suspension hides them immediately before batch scrubbing.
NEWS detail contains only its permitted original introduction and source link,
never a fetched article body. Body is nullable for link-only NEWS/BULLETIN
representations; kind-specific shape is frozen in OpenAPI, not guessed by clients.

### FOR-11–FOR-13: collection

5. **`forum_ingestion_runs`**: UUID PK; source FK; unique `(source_id, period,
   parser_version)`; lease owner/expiry; state (`RUNNING`, `SUCCEEDED`, `FAILED`);
   start/finish; status/typed failure code; counts; conditional ETag/Last-Modified
   and checkpoint. Metadata only; no raw article/page storage or response dump.
   Keep source last-success validators only after complete durable processing;
   partial/failed pass must not advance the checkpoint. Reclaim expired RUNNING
   work; bounded rows and cleanup share existing worker infrastructure.

Deduplication records remain on posts even after editorial rejection/removal
so the next poll does not resurrect them. GUID match and canonical URL match
must resolve to the same row; divergent matches enter review without merging.
Canonicalization removes only source-reviewed tracking parameters, not all query
parameters. Hash title/date is a review hint, never the unique content identity.
No cross-publisher syndication merge without explicit editorial association.

### FOR-14–FOR-20: social persistence

6. **`forum_comments`**: UUID PK, post FK RESTRICT, `author_account_id` FK RESTRICT
   (nullable only on scrubbed removed rows),
   nullable parent comment FK, depth 0/1, per-post immutable `sequence`, text,
   revision, PENDING_REVIEW/PUBLISHED/REJECTED/REMOVED, visibility, timestamps,
   `create_operation_id` UUID/logical hash. Unique `(author_account_id, create_operation_id)` and
   `(post_id, sequence)`; composite parent/post relationship plus locked parent
   validation enforces same post and depth. Visible comment and reply indices
   `(post_id, sequence, id)` / `(parent_id, sequence, id)` partial on live visible
   published rows. Post row has a monotonic `next_comment_sequence`, assigned
   under lock and committed with insert. Lock order: live account, post, parent,
   quota/operation rows; tests enforce consistent order across deletion/moderation.
7. **`forum_report_receipts`**: UUID PK; reporter account FK, typed target UUID,
   case FK to existing moderation cases; operation UUID/hash; time/reason code.
   Unique `(reporter_account_id,target_type,target_id)`. Private detail stays in
   the restricted existing case path, never in feed data. A typed dispatch or
   explicit post/comment nullable FK pair with exactly-one CHECK is required to
   enforce target existence; a bare polymorphic UUID without integrity is not
   sufficient. Reconcile deleted reporter references according to account
   privacy policy; never disclose them to authors.

Reuse existing identity rate buckets and attempt rows. The actual
`identity/adapters/quota.go` exposes `Limiter.CheckTx`; define separate
hour/day operation keys under versioned Forum policies, use both in the same
transaction, and hash the account quota subject with the existing privacy-key
pattern rather than storing raw IDs in global buckets. Replays do not consume
quota again. Add transactional account locks for live counts. No
`forum_categories`, `forum_post_topics`, `forum_reports` case queue,
`forum_reactions` or `forum_subscriptions` tables in this baseline. A fixed enum
and one category eliminate two unnecessary lookup/join tables. Reactions and
subscriptions need later behavior decisions before schema.

### Integrity, retention and migrations

Run writes under pgx transactions; account deletion status and binding must be
checked/locked inside the committing transaction, not only at HTTP entry. Post
removal serializes with comment creation; parent hide/erase cannot be bypassed
by a concurrent reply. CAS increments revision only once. Counters/admission
roll back on failed insert/audit/job; failure after commit is a replay.

Use existing seven-day identity replay for proof-owner requests, with sanitized
logical command bytes excluding rotating credentials and nonces. Include
operation UUID inside the signed JSON so header changes cannot fork intent.
Verify the exact transmitted signed bytes before constructing this logical body;
never normalize bytes before proof verification. The existing account-bound
`SignedOwner` composition in `cmd/api/main.go` (currently composed for
subscription) is a pattern to adapt through Forum ports, not a paid gate.
Persist account/operation uniqueness plus body hash on create beyond that TTL.
Replay rechecks active account/current resource visibility and returns only
`id`, `revision`, `state`, `replayed`; never stored text or permission. For
edit/remove/report, identity runner covers seven days; after that, stale
expected revisions conflict and repeated removal converges without restoring
text. Cross-key create duplication is prevented by the account unique key.

After erasure, live-author constraints permit only removed, scrubbed rows with
null author references. Audit actor/reporter account references must also be
unlinked or transformed under the existing reviewed erasure policy; append-only
event metadata does not justify retaining a permanent personal identifier.

Deletion scrubs text immediately. Metadata audit proposed operational horizon:
90 days; ingestion run rows: 30 days; unreviewed rejected candidates: 30 days
then retain a bounded dedupe tombstone. These are proposed service defaults,
subject to existing legal-hold/backup policy review in FOR-01/FOR-16, not statutory
deadlines. At 50,000 pending candidates or 1 GiB Forum relation size, stop new
collection and alert before increasing budgets. Published editorial retention
is editorially managed; do not silently delete active discussions to save space.
Purge in <=100-row batches with max runtime, metric and restart cursor.

Migrations: foundation tables/indexes → importer metadata → social tables and
moderation target checks → regional/FTS indices after measurement. Each is
append-only, with empty-DB, upgrade-from-supported-schema, repeat migrate,
constraint-negative, rollback/failure and recovery tests on real PostGIS. No
destructive down migration; rollback uses switches/previous compatible binary.

## Proposed REST command contract

All following routes are **future**, not working endpoints. Preserve `/v1`
and existing `httpapi.WriteJSON/WriteError`, bounded decode, static machine codes
and request IDs. Public error shape follows `platform/httpapi/respond.go`.

### Anonymous editorial reads (FOR-05)

- `GET /v1/forum/categories`: `{items:[{code:"FUELS"}], taxonomy_version:1}`.
- `GET /v1/forum/posts?category=FUELS&kind=NEWS&limit=20&cursor=...`: `{items:[],
  next_cursor:null, snapshot:"opaque", fetched_at:"RFC3339"}`. Default limit 20,
  maximum 50; unknown/duplicate filters and invalid enums refuse 400.
- `GET /v1/forum/posts/{post_id}`: `{post:{...}}`, 404 if not public.

Summary fields: `id`, `kind`, `category`, `title`, nullable `excerpt`, nullable
`source:{name,url,license_url,attribution}`, nullable `public_alias`, nullable
`source_published_at`, `first_published_at`, `updated_at`, `revision`, nullable
`region:{uf,municipality_ibge}`, nullable `fuel_product`, nullable `station_id`.
Detail adds `body_text`, `comments_enabled`, `correction` and nullable structured
`bulletin`; never author account ID, `is_owner`, report state or tokens. NEWS
source/date are mandatory; TOPIC source/date are null and alias is public only
while author remains active. Abastevo-owned editorial attribution is explicit.

Fixture examples must be labeled synthetic and live only in future tests. This
plan intentionally publishes no invented article, date, price or benchmark.

### Social commands (FOR-17–FOR-23)

- `GET /v1/forum/posts/{id}/comments?limit=20&cursor=...`: top-level visible
  comments only; `{items:[comment],next_cursor:null}`. Reply count may be returned
  in this online/no-store response; never fan out every reply with the post.
- `GET /v1/forum/comments/{id}/replies?limit=20&cursor=...`: one bounded reply
  page under a currently visible parent/post.
- `POST /v1/forum/posts/{id}/comments`: create with `text` and optional
  `parent_id` (null or top-level same-post ID), common signed/session envelope.
- `POST /v1/forum/comments/{id}/edit`: `text`, `expected_revision`, common
  envelope. `POST /v1/forum/comments/{id}/remove`: expected revision + envelope.
- `POST /v1/forum/posts`: `title`, `body_text`, `category`, optional coarse
  region/station/fuel, common envelope; TOPIC kind is server-selected.
- `POST /v1/forum/posts/{id}/edit`: editable topic fields and expected revision.
  `POST /v1/forum/posts/{id}/remove`: expected revision + envelope. Users cannot
  edit NEWS/EDITORIAL/BULLETIN even by supplying a matching alias.
- `POST /v1/forum/reports`: `target_type` (`POST`/`COMMENT`), `target_id`,
  reason enum, optional detail, common envelope. Successful receipt 202.
- `POST /v1/forum/owner/read`: typed target/id and session envelope; proof
  required, `no-store`; returns only owned current content/state/revision.
  Needed for pending-edit/delete controls without contaminating public caches.
  Private text never enters URLs. No new owner-list endpoint in this baseline.

Common command JSON: `family_id`, `access_token`, `operation_id` UUID plus typed
business fields. HTTP `Idempotency-Key` must equal operation_id when supplied;
operation_id is mandatory and the signed body is authoritative. Reuse the
existing Signature/Signature-Created/Signature-Expires/Signature-Keyid/
Signature-Nonce headers and body digest, existing signer and account binding.
Fresh proof required on retry. Reject extra fields, body-supplied author/kind/
moderation status and incompatible session/signing key. Do not copy feedback's
session-only authorization as if it satisfied signed-write policy.

The `edit`/`remove` command style matches the existing feedback API and carries
the established JSON session/proof envelope. Avoid duplicate PATCH/DELETE aliases
and an invented bearer-token scheme. Owner read is POST for credential-safe
body transport; it remains a read and creates no domain event.

Create returns 201 `{id,revision,state:"PENDING_REVIEW",replayed:false}`; edit
returns 200 same shape; remove returns 200 `{id,state:"REMOVED",replayed:false}`.
Replay preserves logical receipt/current safe state. Max HTTP body 32 KiB for
post commands, 8 KiB for comment/report/owner commands; public detail <=32 KiB,
feed <=64 KiB; transport caps checked before expensive DB work. Unicode scalar
limits still apply independently of byte limits.

### Existing administrator extension

Mount under existing `AdminMiddleware()` only when enabled: `GET
/v1/admin/forum/posts?state=...`, `POST /v1/admin/forum/posts` (draft),
`POST /v1/admin/forum/posts/{id}/edit`, `POST .../{id}/review`, and `POST
/v1/admin/forum/comments/{id}/review`. Review body: expected revision, action
`PUBLISH|REJECT|HIDE|RESTORE`, safe reason and operation UUID. Source decisions:
`GET /v1/admin/forum/sources`, `POST /v1/admin/forum/sources/{id}/review`.
Reuse admin session/CSRF/Origin/CIDR/rate/body/idempotency/audit facilities;
no public moderation mutation route. Existing moderation CLI dispatch must
recognize Forum cases and converge on the same domain transitions. A privileged
account or role assignment without an admin session does not enter this lane.

### Pagination, filters and cache

Chronological feed order is immutable `(first_publication_seq DESC,id DESC)`;
source publication date is displayed separately. First request reads committed
feed revision S; all pages constrain sequence <=S and seek below last tuple.
Existing HMAC cursor seals scope (`forum-posts-v1`), normalized filters, S,
last tuple and policy version using `platform/httpapi/cursor.go` (15-minute TTL).
This prevents late commits/backdated news from entering an earlier snapshot.
Hide/remove always win; gaps are allowed, previously deleted content is not
resurrected to make a snapshot look complete. Edits do not reorder a post.
Comments use a per-post committed sequence snapshot and ascending keyset order.
Every mutation uses a bounded lock/statement timeout; define and test a common
lock order across idempotency/admission, account, source, post/comment and feed
epoch before FOR-03/FOR-14. Acquire the global feed epoch last and never acquire
a post/account lock after it. Source-wide review/erasure uses bounded targets,
not a transaction holding every post plus the global epoch. After transaction
retry, repeat live authorization and preserve the same logical operation ID.
Tampered/expired/filter-swapped cursors get 400 `forum.cursor-invalid` or
`forum.cursor-expired`; Android offers refresh, not an infinite retry.

Category/kind in MVP; FOR-24 adds UF/IBGE/fuel/station subject filters and text
query up to 80 scalars/8 tokens. `sort=recent` is the only baseline ordering.
Relevance ranking is deferred: no opaque “trending” score. PostgreSQL Portuguese
FTS in FOR-24 searches title + original body/excerpt with generated tsvector,
GIN and parameterized plainto_tsquery; combine with filters then the same recent
order. Measure EXPLAIN/latency before adding. Empty search matches normal feed;
invalid/too-long query refuses. No Elasticsearch or extension dependency.

Forum public paths inherit the existing edge request/body/rate protections;
FOR-05 verifies they cannot bypass current anonymous-read limits. Avoid a new
per-read database quota query. Social admission uses transactional hour/day
checks in addition to that edge budget.

Initial all routes use `Cache-Control: no-store`, matching existing social
transport. FOR-09 may enable editorial public-only `no-cache, max-age=0,
must-revalidate` plus ETag after conditional-read/rights-hide tests; never
positive shared TTL without an explicit removal-delay decision. Build ETag from
normalized query/cursor and current feed revision; revalidate visibility/rights
before 304. Public routes ignore cookies/session for projection; private/admin/
commands/comments always no-store. Android cache is application-managed as
specified in the product contract, separately from HTTP cache semantics.

### Error and compatibility behavior

400 malformed JSON/UTF-8/UUID/filter/cursor, 401 absent/expired/replayed proof or
session, 403 active proof but suspended/unbound write authority, 404 absent/
foreign/private target, 409 stale revision/changed operation body, 413 body cap,
415 wrong media type, 429 quota with Retry-After, 503 disabled/unavailable store.
Use `forum.*` machine codes and static messages; dependency errors never expose
SQL, session, source body or network topology. Disabled public read gate yields
503 `forum.unavailable`; absent admin configuration keeps admin routes 404.

OpenAPI additions belong to `contracts/openapi/v1.yaml`, with negative fixtures
and `backend/internal/platform/apicontract` checks in the same implementation
commit. Nullable fields stay explicit. Clients tolerate new optional fields and
unknown future kinds as an unsupported-content row; server input enum remains
closed. No change to existing price/account/feedback routes is required.

## Primary technical references

The proposed SQL design follows PostgreSQL documentation for
[row locks and deadlock ordering](https://www.postgresql.org/docs/18/explicit-locking.html)
and [built-in Full Text Search](https://www.postgresql.org/docs/18/textsearch.html).
These explain mechanisms; actual query plans and concurrency tests on the
repository-pinned PostGIS version are still required before acceptance.
