# Community safety contract — report and block users

Owner: maintainer. Scope: Android and the existing Go backend.

Google Play's [user-generated content policy](https://support.google.com/googleplay/android-developer/answer/9876937)
asks apps with public UGC for in-app reporting of content **and users** and
for **blocking users**. Content reporting already exists for forum comments
and replies (B-BR-FOR-08, BUC-FOR-06), station comments (B-BR-F07) and
charging observations. This contract adds the missing parts: a report action
on forum topics in the app, reporting a user, and blocking a user. It extends
the [Forum contract](../forum/contract.md) and the
[community moderation policy](moderation.md); their rules prevail
outside this addition. Organization data under
[ADR-025](../../decisions/025-private-abastevo-empresas-organization-boundary.md) is
out of scope.

## Business rules

- **B-BR-SAFE-01 — Scope.** The rules apply to public UGC written by
  Abastevo accounts: forum topics (post kind `TOPIC`), forum comments and
  replies, and station comments and replies. Editorial, news and bulletin
  posts, official data and charging observations are not user-attributed
  surfaces here. Staff badges, Premium status or payment give no immunity
  and no extra power.
- **B-BR-SAFE-02 — Content-referenced user.** A client names a user only
  through a content reference: `content_kind` (`FORUM_POST`, `FORUM_COMMENT`,
  `STATION_COMMENT`) plus the public content UUID. The server derives the
  author account. Report and block accept only content that public reads
  serve now (published and visible topic or comment; live, not hidden
  station comment). Missing, pending, hidden, removed and non-topic posts
  refuse like missing resources (404). Naming the actor's own content
  refuses with `safety.self-target` (400). Responses never contain account
  UUIDs, contributor keys, addresses or any identifier that public reads
  do not already show.
- **B-BR-SAFE-03 — Authority.** Every safety write and private read needs an
  active account session, the existing signed request proof and a live key
  binding (B-BR-FOR-05), re-checked inside the write transaction.
  Suspended, deleted, expired and unbound identities refuse. Visitors are
  sent to the free sign-in journey; the action never runs implicitly
  after sign-in.
- **B-BR-SAFE-04 — Report a user.** Reasons: `SPAM`, `HARASSMENT`, `HATE`,
  `IMPERSONATION`, `ILLEGAL`, `OTHER`; optional private detail of at most
  200 Unicode scalar values, plain text. One report per reporter and
  reported account: a repeat returns the first receipt (`replayed`).
  New user reports are bounded to 10 per account per rolling 24 hours
  (429 + `Retry-After`). A report opens or joins the single open moderation
  case on target `COMMUNITY_ACCOUNT` (target = the reported account,
  operator-only). The case detail is fixed text naming the content
  reference; reporter identity and free text stay in the report row. A
  report never hides content, suspends an account or changes trust by
  itself. Operators review it through the audited `ops moderation act`
  (`REVIEW`, `RESOLVE`, `DISMISS`) and act on the content through the
  existing content cases.
- **B-BR-SAFE-05 — Block a user.** Blocking is stored server-side per
  account, so it follows the blocker to every signed-in device and
  survives reinstall. It hides the blocked account's forum topics,
  comments and replies and station comments and replies from the blocker
  only. Blocking the same account again returns the existing block
  (`replayed`). At most 500 active blocks per account (409
  `safety.block-limit`) and 50 new blocks per account per 24-hour window,
  admitted in the block transaction (429 + `Retry-After`). The blocked account is not notified, gets no signal and
  keeps participating publicly; other readers see no change. A block never
  feeds moderation, trust, ranking, reactions or payment.
- **B-BR-SAFE-06 — Hidden content.** Public reads stay anonymous and
  cacheable and carry no author identifier. The signed-in app asks which
  of at most 100 content references it is showing were written by accounts
  it blocked; the server answers with that subset only, never the author
  or which block matched. Unknown references are ignored. The app removes
  hidden items from the forum feed, conversation lists, best-answer
  previews and station comment lists, and an opened topic by a blocked
  author shows a neutral notice instead of its text. The app may skip the
  question while the account has no blocks. A failed answer, or one slower
  than 5 seconds, shows the content (fail open) and the next load asks
  again. Accepted inference: the
  blocker can tell that hidden items share their author with blocked
  content; this is inherent to blocking and bounded by the block limits.
- **B-BR-SAFE-07 — Block list.** The owner lists their blocks: a label
  (the public alias shown on the referenced content when the block was
  made, possibly empty), the source kind and the block date. Removing a
  block takes effect immediately; a foreign or unknown block refuses like a
  missing resource. No content text is stored with a block.
- **B-BR-SAFE-08 — Privacy and retention.** Blocks and user reports are
  private: never in public reads, the live stream, metrics labels or logs.
  Account erasure deletes every block the account made or received and
  scrubs the free-text detail of reports it filed, in bounded restartable
  batches before erasure completes. Reports filed by others about an erased
  account stay as moderation evidence.
- **B-BR-SAFE-09 — Terms.** The Terms of Use list the objectionable content
  and behavior that is forbidden, say that content and users can be
  reported and users blocked in the app, and say what moderation may do.

## Use cases and observable acceptance

1. **BUC-SAFE-01 Report a topic.** A signed account opens a forum topic it
   did not write, chooses *Report topic*, picks a reason and optional
   detail, and sees the received notice. The topic stays visible until
   review (B-BR-FOR-08). A visitor is sent to sign in.
2. **BUC-SAFE-02 Report a user.** From a forum topic, comment or reply, or
   a station comment, a signed account chooses *Report user*, picks a
   reason and sees the received notice. Reporting the same user again
   shows the same receipt. Own content shows no report or block action.
3. **BUC-SAFE-03 Block a user.** From the same places, a signed account
   chooses *Block user* and confirms. That author's topics, comments,
   replies and station comments disappear for the blocker at once and stay
   hidden after refresh, on the forum feed, in conversations and on station
   pages. Other accounts still see them.
4. **BUC-SAFE-04 Manage blocks.** The account opens *Blocked users* from its
   account screen, sees each block's label and date, and unblocks; the
   author's content is visible again on the next load.
5. **BUC-SAFE-05 Operator review.** A user report appears in the moderation
   queue as a `COMMUNITY_ACCOUNT` case with reason and content reference;
   duplicate reports join the open case. The operator reviews it and acts
   on the referenced content through its own case.

## Wire contract

All routes are `POST`, JSON, `Cache-Control: no-store`, signed like Forum
social writes (session envelope `family_id` + `access_token` in the body,
request proof headers on the exact body). Errors use the platform error
envelope with codes `safety.bad-request` (400), `safety.self-target` (400),
`safety.auth-required` (401), `safety.not-found` (404),
`safety.block-limit` (409), `safety.rate-limited` (429 + `Retry-After`) and
`safety.unavailable` (503).

| Route | Body (besides the session envelope) | 200 response |
|---|---|---|
| `/v1/community/user-reports` | `content_kind`, `content_id`, `reason`, optional `detail` | `{"receipt":{"report_id","reason","created_at","replayed"}}` |
| `/v1/community/blocks` | `content_kind`, `content_id` | `{"block":{"block_id","label","source_kind","created_at"},"replayed"}` |
| `/v1/community/blocks/list` | — | `{"items":[{"block_id","label","source_kind","created_at"}]}` (newest first, at most 500) |
| `/v1/community/blocks/{block_id}/remove` | — | `{"removed":true}` |
| `/v1/community/blocks/hidden` | `items`: 1–100 of `{"content_kind","content_id"}` | `{"hidden":[{"content_kind","content_id"}]}` |

Reporting a forum topic itself uses the existing
`POST /v1/forum/posts/{post_id}/report`.
