# Forum Abastevo — Community Policies, Terms and Operator Coverage

Authority: [ADR-019](../../adr/019-android-vps-first-commercial-pilot.md), B-BR-FOR-04–14, BUC-FOR-04/05/06/09; FOR-16.
Status: OPERATIONAL_POLICY / RELEASE_GATE. No live readiness claim without prior operator sign-off.

---

## 1. Community Guidelines & Reviewed Terms

Forum Abastevo provides a civil, transparent, and focused space for Brazilian drivers and consumers to discuss fuel prices, fuel quality, station reliability, and automotive news.

1. **Relevance:** Commentary must relate strictly to the subject post (fuel prices, station services, automotive regulations, editorial news, and data bulletins). Off-topic discussions, partisan political agitation, commercial advertisements, and spam are prohibited.
2. **Civility & Prohibited Content:** Any defamatory, libelous, threatening, harassing, discriminatory, sexually explicit, or fraudulent text is strictly forbidden. Posting personal data (PII) of attendants, station staff, or fellow users violates both platform policy and LGPD rules.
3. **Format & Length Boundaries:**
   - Body text must contain between 1 and 280 Unicode scalar values.
   - Text is trimmed of leading and trailing whitespace; empty bodies or bodies containing only whitespace are rejected with HTTP 400.
   - Content must be valid UTF-8 without control or NUL characters.
4. **Hierarchical & Thread Boundaries:**
   - Discussions support flat top-level comments (`depth = 0`) and a single reply level (`depth = 1`).
   - Replies to replies are forbidden (`ErrInvalidCommentDepth`).
   - Cross-post parenting is rejected (`ErrCrossPostParent`).
   - Hidden or tombstoned parent comments suppress their entire reply sub-tree from public feeds (`ErrParentCommentNotVisible`).
5. **Rate Quotas & Operating Budgets:**
   - Per-account creation limits: maximum 20 comments per rolling hour and 100 comments per rolling day.
   - Durable live budget: maximum 100 live comments (`PENDING_REVIEW` or `PUBLISHED`) per account across the entire Forum. Exceeding this budget requires authors to tombstone old comments before submitting new ones (`ErrCommentLiveQuotaExceeded`).

---

## 2. Premoderation Disclosure & Workflow

Per B-BR-FOR-04 and B-BR-FOR-09, Forum Abastevo operates on an approved-by-default or premoderated review model:

1. **Initial State (`PENDING_REVIEW`):**
   - Every submitted comment or reply enters persistence in `PENDING_REVIEW` state.
   - Pending comments are strictly isolated from anonymous public readers (`ListCommentsByPost` and `ListRepliesByParent` return only `PUBLISHED` rows).
   - Only the authentic author via signed private owner endpoints (`CommentReceiptDTO` / `OwnerCommentResponseDTO`) can read their own pending text. Foreign authors receive indistinguishable 404 responses.
2. **Operator Approval & Rejection:**
   - An authorized operator or automated content classifier inspects the candidate queue.
   - Approved comments advance to `PUBLISHED` with `published_at` recorded and are indexed for public reading.
   - Rejected comments transition to `REMOVED` with `visibility = 'HIDDEN'` and text scrubbed; an audited record is preserved in `forum_changes`.
3. **CAS Edits & Tombstone Removals:**
   - Authors may edit comments using Compare-And-Swap (`expected_revision`). Every edit increments `revision` and returns the comment to `PENDING_REVIEW`.
   - Authors may tombstone comments using `remove`. Removal irrevocably clears `body_text = ''`, sets `publication_state = 'REMOVED'`, `visibility = 'HIDDEN'`, and increments revision. Tombstoned comments are terminal and cannot be re-edited or re-published.

---

## 3. Legal-Notice Contact & Judicial Takedown

In compliance with the Brazilian Civil Rights Framework for the Internet (**Marco Civil da Internet**, Lei nº 12.965/2014, art. 19):

1. **Official Contact Channels:**
   - Judicial notifications, extrajudicial notices, and copyright infringement claims must be directed to:
     - **Email:** `juridico@abastevo.com.br` / `privacidade@abastevo.com.br`
     - **Postal Address:** Abastevo Soluções Tecnológicas Ltda., São Paulo/SP, Brasil.
2. **Response Deadlines & Takedown:**
   - Specific court orders (*ordem judicial específica*) identifying the exact URL or comment ID are triaged within 24 hours.
   - Upon valid court order or verified infringement notice, operators immediately execute an audited `tombstone` action, wiping body text and hiding visibility.
   - Every takedown action logs an immutable change row in `forum_changes` (`action = 'REMOVE'`, `actor_kind = 'ADMIN'`, with policy version and reason code).

---

## 4. Author & Reporter Decision and Appeal Handling

1. **Author Notification & Visibility:**
   - When an authored comment is rejected or removed by moderation, private owner read reflects the `REMOVED` state.
   - The private receipt displays removal status without exposing internal moderator notes or operator credentials.
2. **Structured Reporting:**
   - Readers may report comments via `POST /v1/forum/comments/{id}/report` selecting standard categories: `SPAM`, `HARASSMENT`, `DEFAMATION`, `OFF_TOPIC`, or `ILLEGAL`.
   - Each report opens a prioritized case in `moderation_cases` with a unique report receipt.
3. **Appeals Workflow:**
   - Authors who believe their commentary was incorrectly rejected may submit an appeal via `suporte@abastevo.com.br` quoting the `operation_id` or `comment_id`.
   - Appeals are assigned to a senior operator distinct from the initial reviewer and resolved within 5 business days.
   - If an appeal is upheld, the comment is re-reviewed and may be re-published; if denied, the rejection remains final.

---

## 5. LGPD Compliance & Account Erasure Integration

Under the Brazilian General Data Protection Law (**LGPD**, Lei nº 13.709/2018, arts. 18 and 19):

1. **Right of Access & Portability (`ExportAccount`):**
   - Authors may request a complete export of their forum activity at any time.
   - `ExportAccount` returns a deterministic JSON archive (`forum-export-v1`) containing all comments authored by the requesting account ID.
   - **Privacy Isolation:** The export strictly redacts any other users' private comments, votes, or unapproved commentary.
2. **Right to Erasure (`EraseAccountBatch`):**
   - When an account deletion is initiated, the platform coordinates batched erasure across all modules via `accountapp.ErasureRecovery`.
   - Forum erasure processes up to 100 comments per batch:
     - All verbatim body text is wiped (`body_text = ''`, `body_sha256 = ''`).
     - State transitions to `publication_state = 'REMOVED'`, `visibility = 'HIDDEN'`.
     - An audit change record is stored in `forum_changes` (`actor_kind = 'SYSTEM'`, `reason_code = 'erasure-scrub'`) without retaining sensitive content.
   - **Never Completion Before All Modules Succeed:** Account deletion claims remain in progress (`ErrErasurePending`) until `forumStore.EraseAccountBatch` confirms zero remaining unscrubbed rows.
   - **No Resurrection:** Restored database backups cannot resurrect deleted accounts or wiped comment bodies; late writers with deleted credentials fail closed at the transaction guard.

---

## 6. Operator Responsibility & Release Gate

1. **Operator Liability:**
   - The platform operates as an Internet application provider (*provedor de aplicações de internet*).
   - Under Marco Civil da Internet (art. 19), platform premoderation and community moderation tools represent good-faith efforts to maintain platform integrity and do not constitute universal editorial authorship of third-party user comments.
2. **Release Checklist (Mandatory Gate):**
   - [x] Pure domain constraints and length/depth bounds validated.
   - [x] Database schema and append-only migrations verified.
   - [x] Transactional CAS and tombstone removal tested under concurrency.
   - [x] Idempotency hashing and replay isolation verified.
   - [x] Export portability service and deterministic byte serialization validated.
   - [x] Batched erasure and `accountapp.ErasureRecovery` integration proven with real PostgreSQL tests.
   - [x] Reviewed terms, legal contact, and operator liability documented.
   - [ ] Operational legal counsel formal sign-off prior to commercial production deployment (P09/G09).
