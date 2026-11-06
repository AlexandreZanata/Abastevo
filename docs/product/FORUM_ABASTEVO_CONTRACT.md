# Forum Abastevo product contract

Status: PROPOSED / NOT IMPLEMENTED. Planning date: 2026-11-05.
Owner: maintainer. Scope: Android and the existing Go backend only.

The [execution plan](../planning/FORUM_ABASTEVO_PLAN.md) owns task order;
the [architecture](../backend/FORUM_ABASTEVO_ARCHITECTURE.md) owns proposed
persistence and wire contracts. Existing product/privacy rules prevail outside
this explicitly bounded addition. No deployment or source activation is implied.

## Product decision and delivery boundaries

Forum is a reading and discussion destination reached from Community. Community
continues to show its city, contribution action and existing price activity.
Forum is not a price contribution, rating, source of station confidence or paid
benefit. Reading and all social participation remain free.

The useful first release is **editorial reading**: entry card, chronological
feed, categories, article detail, original source link, attribution and bounded
offline editorial cache. It can contain original Abastevo editorial text and
individually approved licensed links. It does not require a news crawler.

The complete planned baseline adds, in order: approved-source news collection;
comments and one-level replies with owner controls and moderation; user topics
and bounded search; deterministic data bulletins. Reactions, follows,
notifications, personalized ranking, attachments and public deep links are
deferred extensions. Their absence is not a reason to delay editorial reading.

## Business rules

- **B-BR-FOR-01 — Separation.** Add “Fórum Abastevo” immediately after the city
  selector and before the price contribution card. Preserve existing bottom-bar
  destinations, price filters, station routes and contribution behavior.
- **B-BR-FOR-02 — Provenance.** A post has exactly one kind: `EDITORIAL`, `NEWS`,
  `TOPIC` or `BULLETIN`. Source identity, source publication time, Forum first
  publication time and update time are distinct. An external news item without
  a trustworthy publication time remains in review. Never substitute crawl time.
- **B-BR-FOR-03 — Rights.** Publication requires a recorded rights decision for
  title, excerpt and any image separately. Feed availability is not permission.
  No copied full article, automatic paraphrasing, hotlinked image or image upload
  in this baseline. An original editorial introduction must not be represented
  as a verbatim publisher summary. Unknown rights keep the candidate unpublished.
- **B-BR-FOR-04 — Reading.** Anonymous clients receive only currently published,
  visible post projections and visible comments. Draft, pending, rejected,
  hidden, removed and foreign-owner private records are indistinguishable from
  missing resources. Public output never includes account UUIDs, addresses,
  session credentials, key fingerprints, reporter identity or precise GPS.
- **B-BR-FOR-05 — Authorship.** Every social write and private owner read requires
  a valid account session, existing signed-request proof and live binding of the
  signing contributor key to that account. Derive author server-side. Suspended,
  deleted, expired, revoked and unbound identities refuse. A public staff badge,
  Premium status or payment grants no editing or moderator permission.
- **B-BR-FOR-06 — Text.** Plain UTF-8 text only; reject malformed encoding/NUL
  and blank values after trimming. Proposed limits: title 1–120 Unicode scalar
  values; topic/editorial body 1–4,000; editorial/news introduction 1–400;
  comment/reply 1–280. Introduction is optional when rights permit no excerpt;
  the scalar bound applies when present. Preserve meaningful line breaks. Do not interpret HTML,
  Markdown, scripts, mentions, remote embeds or automatic link previews.
- **B-BR-FOR-07 — Categories and regions.** One code-owned primary category per
  post: `FUELS`, `ELECTRIC`, `TRAFFIC_RULES`, `VEHICLES_TECH`, `ROADS_MOBILITY`,
  `DRIVER_ECONOMY`, `ABASTEVO_DATA`. Localize labels in the client. Optional UF,
  IBGE municipality code, Directory station UUID and existing fuel wire enum
  describe subject matter, not the author's whereabouts. Validate municipality
  against UF and station against Directory; no GPS collection is needed.
- **B-BR-FOR-08 — Moderation.** Editorial/news start as `DRAFT`; submissions by
  users start as `PENDING_REVIEW`. Only an authorized operator publishes them.
  Review outcome never follows automatically from report count. A report opens
  or joins an existing case, leaving a legitimate post visible until a reviewed
  decision. Hide/restore is audited; removal and account erasure cannot be undone
  by restore. Report text is not copied into public responses or logs.
- **B-BR-FOR-09 — Owner controls.** Owners can edit/remove their live topics and
  comments. Expected revision is mandatory for edits. An edited published topic
  or comment returns to review and is temporarily absent from public reads;
  previous text is not served as a fallback. Flat top-level comments plus one
  reply level only; no reply to a reply or cross-post parent. A hidden parent
  suppresses its reply subtree from public reads.
- **B-BR-FOR-10 — Replay and concurrency.** A client generates an operation UUID
  before each intent; retries keep that UUID and logical body, refresh session
  and create a fresh proof nonce. At most one create per account/operation UUID
  survives even across devices/keys and after the normal replay window. A
  different logical body conflicts. Edit/remove/report outcomes and database
  quota admission are atomic with their writes; unknown network outcomes do not
  cause a new intent automatically.
- **B-BR-FOR-11 — Bounded admission.** Initial proposed server policies: topics
  3/hour, 10/day and 20 live/account; comments/replies 20/hour, 100/day and 100
  live/account across Forum; reports 10/day/account, one per account/target.
  Enforce both time windows, durable live budgets and body caps transactionally,
  including concurrent different keys. Return 429 and `Retry-After`; do not
  weaken existing account/global flood protections. These are Forum-only policy
  defaults to freeze in FOR-01, not claims about current runtime constants.
- **B-BR-FOR-12 — External links.** Only HTTPS source links, without userinfo,
  fragments or unsafe schemes, are actionable. News links must match the
  approved source's exact host and allowed path scope. User text links remain
  non-clickable in this baseline. Opening a source is explicit in the system
  browser, with the hostname visible; never use an in-app HTML renderer.
- **B-BR-FOR-13 — Retention.** Owner removal immediately excludes content from
  reads and clears body/excerpt/alias-bearing revision data; keep only bounded
  non-content tombstone and audit metadata. Account erasure immediately blocks
  writes and hides authored Forum rows, then scrubs references/text in restartable
  batches through existing account recovery. Privacy export includes owned
  Forum records. Restore replay must reapply erasure before public traffic.
  Backups, legal holds and abuse evidence follow the reviewed existing policy;
  no arbitrary new legal retention period is asserted by this plan.
- **B-BR-FOR-14 — Offline and freshness.** Only bounded public editorial/news/
  bulletin text is cached; never owner drafts, session credentials or report
  details. Show cache age and offline status. Expire local text after 24h;
  revalidate on return/network recovery. A live 404/hidden/deleted response
  invalidates the cached copy immediately. Social content and writes require
  network; pending writes are not a new background outbox in this baseline.
- **B-BR-FOR-15 — Bulletin truth.** Deterministic templates use accepted immutable
  source revisions and exact integer/decimal quantities. Display dataset IDs,
  periods, geography, fuel, units, sample counts, coverage, methodology version
  and correction status. Official and community facts never become one average.
  No zero denominator, missing-source substitute, estimated electric opening or
  extrapolation to unobserved drivers/stations. Statistical bullets do not
  change authoritative Official/Directory/Charging records.
- **B-BR-FOR-16 — Operability.** Independent default-off read, write, collection
  and auto-publication switches. Disabling a source stops collection and queues
  review of its visible records. Source outage never deletes unrelated posts or
  blocks prices/maps/health. No LLM, paid API, service, Redis, search cluster,
  websocket or constant polling is required.

## Use cases and observable acceptance

1. **BUC-FOR-01 Discover/read.** Visitor taps the card, sees real published
   content or an honest empty state, reads a post, opens its original source,
   then returns to the previous Community city/filter/scroll state. Forum works
   without selecting a city. Price contribution remains independently usable.
2. **BUC-FOR-02 Editorial publication.** Existing privileged administrator creates
   a draft with source/rights metadata, reviews a preview and explicitly
   publishes a matching revision. Missing rights or stale revision refuses.
3. **BUC-FOR-03 Collect/review news.** Periodic worker fetches an allowlisted
   authorized source, validates bounded metadata and converges on one candidate
   across retries. Unknown category/date/rights or suspicious change enters
   review, not the public feed. Operator can reject, publish or correct it.
4. **BUC-FOR-04 Discuss.** Active signed account submits one comment/reply;
   receipt shows “Awaiting review”. Moderator-approved content becomes readable.
   Visitor attempting to write enters the existing free sign-in journey;
   returning preserves a memory-only draft and never auto-submits it.
5. **BUC-FOR-05 Correct/remove.** Author loads a separate private owner projection,
   edits with current revision or removes. Foreign user gets 404; simultaneous
   edits yield one winner and a conflict. Pending/removed text does not leak.
6. **BUC-FOR-06 Report/moderate.** Signed account reports `SPAM`, `HARASSMENT`,
   `OFF_TOPIC`, `MISINFORMATION`, `RIGHTS` or `OTHER`, optionally up to 200 scalars
   of private detail. Duplicate reports converge. Existing admin tools show
   case/target, reason, actor and audit; hide/reject/restore are explicit actions.
7. **BUC-FOR-07 Open topic.** Signed account submits title/body/category and
   optional coarse region. A safe pending receipt replaces optimistic public
   insertion. Owner can review their submission using the private projection.
8. **BUC-FOR-08 Publish bulletin.** Completed accepted datasets trigger one
   reviewed template output per deterministic key. Same input is a no-op;
   corrected input creates a linked correction. Missing coverage produces no
   bulletin and an operational reason, never fabricated text.
9. **BUC-FOR-09 Exercise rights.** Existing export/deletion flow includes Forum.
   Partial cleanup/restart/restore cannot resurrect content or acknowledge
   completion before Forum cleanup has succeeded.

## Launch decisions

Editorial reading can ship after FOR-09 with content ownership and operational
review proven. News collection can remain disabled indefinitely without
pretending it is delivered. Social phases require real operator coverage,
published terms/community policy, abuse handling and privacy tests before their
write switch can be enabled. All moderation is pre-publication initially; its
cost is operator time and delayed discussion, to be measured before any change.

No source is approved by this document. No device, capacity, runtime or legal
acceptance is inferred from a plan. Optional features require their own scoped
contract and tests; they are not silently included in “implement Forum”.
