# P37-PC04 reviewed subset and shared photo contract

B-BR-PC06 / BUC-PC03. Parent e41936c, maintained dev, implementation opening.
One confirmed review has stable per-fuel command ids, original capture time,
account/key/environment scope and one capture/media session. All nonblank rows
are validated before durable publication to the local queue; no half-validated
review is dispatched. A retry reuses the frozen snapshot, never generates new
ids, resets time, silently changes owner or claims server acceptance from local
queueing. Disabled enqueue, expired media/proof, scope changes and unavailable
storage are visible failures. Blank/X rows never enter the snapshot. CNG/LPG
use their actual canonical unit. No GPS/raw OCR/signed URLs enter the outbox.

The signed existing upload API gains optional `photo_capture_id`, `station_id`
and `captured_at` as an all-or-none photo-flow envelope. A valid owner/key/station
receipt is checked before reservation, then bound to exactly one server evidence
session before any presigned URL is returned. The reverse session->receipt
association is also unique. Replays preserve expiry; reservation expiry cannot
outlive the original capture receipt. Failed competing binding grants no URL.
Legacy media/manual contracts retain their existing narrow behavior.

Signed observation intake gains optional `photo_capture_id`. When supplied,
ready owner evidence must resolve to that receipt's unique session and original
capture time/station. Multiple selected fuels can reference that one object;
foreign owner/key/station/session, changed capture time and expired proof fail.
A capture/product/condition tuple may publish only one immutable observation;
replay uses the original id. Shared references are an explicit authorized lane;
legacy objects retain one-observation binding. Ready alone does not override
expiry. Reference metadata is removed with operational evidence and attribution
is unlinked by existing rights erasure; no new GPS or original archive exists.

The worker signs each mutation with the exact fresh server nonce. It negotiates
SHA-256/size/MIME, PUTs only to the bounded HTTPS presigned target without auth
headers or redirects, completes, waits for READY and submits canonical money,
fuel and condition JSON. A pending media job stays retryable, not acknowledged.
Store owner receipt/status instead of deleting acknowledged local commands;
server RECEIVED remains pending validation and is distinct from VALIDATED.
Cancellation or cleanup of one row cannot delete media used by another row.

Immediate acceptance: domain/intake/parser/auth failures; original time/deadline
and owner scope; stable retries/double taps/partial failures; transaction and
restart recovery; real PostGIS migration/concurrency/idempotency; shared object
owner/session/fuel isolation; ordinary expiry/erasure and ready-media failure.
Source completion never certifies currently unavailable staging profile/media
prerequisites. No historical corpus prices are sent to a live backend.

Status: IMPLEMENTING
