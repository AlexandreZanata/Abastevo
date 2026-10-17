# FQ-05W — Trusted native PMQC runtime composition

Status: IN_PROGRESS / INTEGRATION_PENDING. Opened 2026-10-16.

Persistence prerequisite: [FQ-05W1](FQ05W1_MEMBER_PROVENANCE.md), LOCAL_DONE.
[FQ-05W2 archive/process](FQ05W2_ARCHIVE_PROCESS.md) is LOCAL_DONE; historical date-time compatibility is now LOCAL_DONE in [FQ-02C](FQ02C_SOURCE_CLOCKS.md). [FQ-05W3 Directory identity/fence](FQ05W3_DIRECTORY_FENCE.md) is LOCAL_DONE. Fetch/worker/image composition below remains IN_PROGRESS.

Before behavior: B-BR-FQ-01/02/03/04/05/07/12/13/16, BUC-FQ-01/02/03/04.
Compose the real pinned Rust preparer, bounded official fetch/archive handling,
Directory-owned read-only identity, existing durable jobs and atomic publication.
Default native intake/discovery stays off until the scoped staging acceptance;
public reads, signed report rights and other source/safety schedules remain separate.

Verified source observation: the 2025 annual ZIP contains twelve monthly CSV/JSON
pairs, not a single annual CSV. It is 24,513,334 compressed bytes, SHA256
c25d8dcf0c93560085572c49327a077202c6ac33d24c87303c4bb34b60d383c2.
Monthly CSV members range from 8,016,475 to 27,921,699 bytes. Preserve a monthly
edition per CSV and freeze parent URL/hash/bytes plus exact member/hash/bytes.
Identical archive hashes never collapse different monthly members as aliases.
No source/source-period completeness claim from partial archive processing.

Bounded processing: at most 32 safe, unique archive entries, 100 MiB compressed,
1 GiB declared aggregate, 30 MiB selected CSV, existing 500,000-row/100 MiB prepared
contract. Parse only the selected CSV with CRC/declared-size verification; unused
JSON is not an imported format. Reject traversal, encryption, symlinks, duplicate
members, unknown names, mismatched period/source and unsupported native headers.
Use file-backed fetches and private per-attempt workspaces, trusted bundled static
Rust binary, explicit timeout/cancellation and complete cleanup. No raw source,
quarantine text or subprocess output in job errors/logs.

Directory clocks: operational identifier insertion timestamps are not business
establishment dates. A unique current full-CNPJ canonical identity may bind a
dated official record under the same full identifier; source observation supplies
its date, never a guessed business window. Multiple canonical identities stay
ambiguous, retired-only identities require succession review, different branches
only route to review, and missing identity never creates a station. Recheck/lock
active identity inside publication; preparation/replay cannot cross retirement.

Historical publication must not regress stream current pointer or completed
watermark. Frozen zero-padded year/month labels order the existing stream contract;
corrections of the current edition still advance its revision. Frozen source
period claims accompany PMQC as well as events. Publication/replay remain atomic.

Native source work has serial execution slots separate from existing safety
loops; a disabled flag excludes its queued work without charging attempts.
Discovery consumes bounded verified inventory, rejects changed/hash-mismatched
fetches and records source/member scope. No new public operator/business endpoint,
GPS, payment, purchase documents, iOS work or unrelated registry activation.

Immediate acceptance: member alias regression, late historical current/watermark
regression, archive traversal/bomb/CRC/truncation/shape, real Rust process outage/
timeout, fetched-file integrity/cleanup, source allowlist/redirect/private-address
negative cases, actual Directory ambiguity/retirement/concurrency, SQL migration
empty/upgrade/replay, native API/worker source composition and indexed load.
