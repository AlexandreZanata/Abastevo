# Forum source investigation, rights and collection

Status: RESEARCH / SOURCES NOT ENABLED. Checked 2026-11-05.
This record distinguishes observed official pages, unproven machine endpoints
and permission decisions. A successful HTTP response is not a license or SLA.
No source owner was contacted and no account/provider was configured.

## Candidate register

| Candidate | Observed official access | Rights and attribution | Recommended initial disposition |
|---|---|---|---|
| ANP | [News listing](https://www.gov.br/anp/pt-br/canais_atendimento/imprensa/noticias-comunicados), HTML; no RSS link found in the inspected listing | Footer links CC BY-ND 3.0; item-specific exceptions still need checking; identify ANP, original URL/date and license | Priority candidate. Manual metadata review first; machine feed/API is UNVERIFIED. No auto-excerpt/image authorization |
| Senatran | [Correct news listing](https://www.gov.br/transportes/pt-br/assuntos/transito/noticias-senatran), HTML; the initially suggested `conteudo-Senatran/noticias` path could not be fetched | Inspected footer CC BY-ND 3.0; attribution and per-item rights required | Use this canonical listing for research; RSS/API UNVERIFIED, collection disabled |
| Ministry of Transport | [News](https://www.gov.br/transportes/pt-br/assuntos/noticias), HTML; extracted listing depends on page rendering | Inspected footer CC BY-ND 3.0; no blanket summary/image approval | Candidate for road/mobility metadata; HTML adapter only after access/terms review, no headless-browser service in MVP |
| DNIT | [News](https://www.gov.br/dnit/pt-br/assuntos/noticias), readable HTML | Inspected footer CC BY-ND 3.0; assess photograph ownership separately | Candidate for road notices. Avoid duplicate Ministry/agency syndication; RSS/API UNVERIFIED |
| Detran-MT | [Official news URL](https://www.detran.mt.gov.br/noticias) timed out during research | Terms, format and redistribution not verified | BLOCKED candidate, not a fallback scraper. No assertion of reliability or permissions |
| Agência Brasil / EBC | [Official feed directory](https://agenciabrasil.ebc.com.br/feed/) links [latest RSS](https://agenciabrasil.ebc.com.br/rss/ultimasnoticias/feed.xml) and [economy RSS](https://agenciabrasil.ebc.com.br/rss/economia/feed.xml) | [Reproduction policy](https://agenciabrasil.ebc.com.br/sobre) permits journalistic use with attribution, asks commercial reproduction to contact licensing and warns about partner content | Best confirmed RSS discovery, but commercial Abastevo fit remains REVIEW_REQUIRED; all images off; exclude partner material |
| Motor1 Brasil | [Automotive RSS endpoint](https://motor1.uol.com.br/rss/news/all/) answered the research fetcher with RSS content type, which that fetcher cannot render | Redistribution/summary/image permission UNVERIFIED; RSS itself does not grant it | Useful specialist candidate; disabled until a documented permission/terms decision. No promised free republishing |
| Abastevo accepted Official datasets | Existing immutable Official revisions plus [ANP price series](https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/precos-revenda-e-de-distribuicao-combustiveis/serie-historica-do-levantamento-de-precos); use the accepted internal import manifest as the actual input | Dataset license/provenance remains the existing importer responsibility; generated prose is original Abastevo methodology output | Most controllable low-cost content source in FOR-25–FOR-27; eligibility still depends on accepted inputs and coverage |

Frequency is not a publisher SLA: no feed cadence or continuous uptime was
measured. Proposed **our collection budget** is one conditional fetch/source/hour,
maximum four approved sources, one source in flight, <=100 entries/pass. Revisit
only with demand, permission and measured VPS budget. Senatran/Ministry may
overlap; start with one rather than duplicate requests. Editorial-only release
can run with zero external automatic sources.

The web research renderer identified RSS MIME for EBC latest and Motor1 but
could not parse their bodies; EBC economy timed out in that renderer. Treat
runtime XML parser compatibility, current redirect chains, ETag behavior and
timestamp completeness as FOR-10/FOR-11 validation obligations. Do not report a
successful import, usable API or sustained availability from this investigation.

## Publication permissions, not just collection permissions

The inspected government footers point to
[CC BY-ND 3.0](https://creativecommons.org/licenses/by-nd/3.0/), which permits
redistribution subject to attribution and prohibits sharing adapted material.
Therefore this plan does not treat a trimmed/copied/paraphrased summary as
automatically authorized. Distinguish factual original commentary from an
adaptation, document the permitted display mode and escalate uncertain reuse to
the maintainer's rights review. A site's general footer also does not establish
that every credited third-party photograph belongs to the site.

For each source, record official host/path, owner, access format, permitted
title mode (`EXACT`, `ORIGINAL`, `NONE`), excerpt mode (`LICENSED_EXCERPT`,
`ORIGINAL_EDITORIAL`, `NONE`), media permission (baseline `NONE`), attribution,
license/terms URL/hash, review date/expiry, item exceptions and whether automatic
publication is separately allowed. Default rights review expiry: 90 days;
expiry or terms change pauses publication and creates review work.

An article may be linked manually with an original Abastevo introduction only
after the relevant item decision. Full article storage, feed HTML mirroring,
automatic translation and machine paraphrases are excluded. An excerpt-free
mode may show only approved title/source/date/link; it must not synthesize a
publisher summary. Media stays off even when a feed contains `enclosure` or
OpenGraph image URLs. Future images require licensed acquisition, limits,
private-storage/privacy review and a separate task.

## Bounded pipeline in the existing worker

1. Scheduler enqueues `forum-source-fetch-v1` per source/hour, with existing
   dedupe and a run lease. Disabled/unapproved/expired source is a no-op with a
   typed reason. Job payload is source ID + version, never arbitrary fetch URL.
2. Adapter loads reviewed source configuration; sends conditional GET using last
   **successful** ETag/Last-Modified. 304 updates success metadata without
   generating duplicates. Do not advance validators after partial failure.
3. Safe HTTP client validates URL and resolved IPs at every connection/redirect;
   HTTPS:443 only, exact approved host/path (normalize encoded/dot path segments before checking),
   no credentials, cookie jar, inherited
   proxy or cross-origin authentication. Reject loopback/private/link-local/
   multicast/reserved IPv4 and IPv6 including mapped forms, cloud metadata,
   userinfo, downgrade, DNS rebinding and redirects to unapproved hosts. Pin the
   validated address in DialContext while preserving TLS SNI/verification.
4. Maximum 2 redirects, 5s connect, 10s fetch, 30s whole pass; body <=2 MiB
   after decompression, per-entry <=16 KiB metadata, <=100 entries. Refuse excess
   rather than silently treating truncation as a complete successful feed.
   XML parser accepts RSS/Atom only in first adapter, rejects DOCTYPE/ENTITY and
   unbounded nesting (>32), never fetches external entities. Evaluate existing
   dependencies before adding a parser; Go stdlib is the first choice.
5. Normalize GUID/canonical URL, source date/timezone and Unicode text. Do not
   strip meaningful URL queries or assume timezone when absent. Unknown/future
   date (>5 minutes beyond clock), empty title, incompatible rights or malformed
   source leaves an unpublishable candidate/reason. Raw HTML is neither returned
   nor stored; derive plain permitted metadata, never render it in the app.
6. Deduplicate transactionally with source/GUID and canonical URL constraints.
   Existing same input is a no-op. Source edit changes revision and requeues
   editorial review; a source deletion is review work, not arbitrary immediate
   deletion of related discussion. Preserve local original publication ordering.
7. Classify with versioned source/category map plus normalized pt-BR keyword
   rules: fuel names → FUELS; recarga/vehicle electrification → ELECTRIC;
   Contran/Senatran rules → TRAFFIC_RULES; roads/interdictions → ROADS_MOBILITY.
   Resolve multiple matches by explicit priority; ambiguous/off-topic goes to
   review. Keep classifier version and reason codes, not a fictitious confidence
   probability. No LLM call or arbitrary content execution.
8. Commit draft, safe change record and run checkpoint together. Publication is
   a separate admin action with expected revision and current rights check.
   Initial auto-publication remains OFF. A later narrowly approved deterministic
   source/template can opt in after corruption/rights/replay tests.
9. Bounded retries: existing job mechanism, at most three attempts per period;
   respect capped Retry-After for 429/503, exponential backoff and next scheduled
   pass after exhaustion. Maintain last good public content; stale-source status
   is operational, never a fake “fresh” date in the feed.

Forum jobs must remain in the ordinary dispatcher, with one bounded source job
at a time and bounded database statement time. Do not join or slow the dedicated
evidence/privacy safety lanes. Worker cancellation releases/reclaims leases;
success/timeout/crash must not duplicate publication. No crawling arbitrary URLs,
image fan-out, browser automation farm or HTTP work inside a DB transaction.

## Current legal and operational review before social launch

Review [copyright law](https://www.planalto.gov.br/ccivil_03/leis/l9610.htm),
[LGPD](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm),
[Marco Civil](https://www.planalto.gov.br/ccivil_03/_ato2011-2014/2014/lei/l12965.htm)
and current implementation obligations. The original text of article 19 is not
an adequate standalone rule for operating the Forum. The
[ANPD's 2026 guidance](https://www.gov.br/anpd/pt-br/assuntos/marco-civil-da-internet)
discusses platform handling of reports, reasoned decisions and contestation;
the [STF process for RE 1037396](https://portal.stf.jus.br/processos/detalhe.asp?incidente=5160549)
and RE 1057258 must be checked in their then-current final form at launch.
This is an engineering review gate, not a claim of legal certification or that
every large-platform obligation applies to this small application.

FOR-16 must document operator ownership, an existing support/appeal contact,
handling of urgent legal/safety notices, copyright requests, decision reasons,
author/reporter feedback, privacy export/erasure, applicable retention and
access-log obligations. Extend existing terms/privacy/community rules; do not
invent a blanket “wait for a court order” policy or automatically hide every
reported post. Reporting and response must work before user publication is
enabled. Report status/appeals can use existing support plus private receipts;
push notifications and an independent appeals platform are not MVP dependencies.

## Evidence limits and revalidation

Source research establishes candidates and observed conditions only. It does
not prove permission for this commercial use, parser behavior, capacity,
correctness of external claims or continued availability. Recheck terms,
canonical paths, representative permitted item fixtures, date attribution,
robots/access restrictions and deterministic parsing in FOR-10. If no external
source passes, deliver original editorial mode and record NEWS_SOURCE_BLOCKED;
do not quietly activate an unreviewed alternative or label news ingestion done.
