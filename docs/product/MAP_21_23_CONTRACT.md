# MAP-21–23 implementation contract

Scope: complete only community operational reports, native route planning and
explicit foreground guidance. MAP-24–33 and production certification are outside
this batch. The preceding real-data minimum map is local acceptance only.

## MAP-21

B-BR-MAP21-01: operational events are dated contributor observations (`WORKED`,
`DID_NOT_WORK`, `IN_USE`, `ACCESS_RESTRICTED`), never a live feed or a guarantee.
No confidence score, paid trust, fixed expiry or photo requirement is added.
A null event preserves existing price/experience reports.

B-BR-MAP21-02: SITE events require a real charging place. CONNECTOR events
reference an existing connection-group UUID under that place, matching its
connector code. EQUIPMENT targets use a source reference only when that group
reports exactly one unit; unknown/multiple quantities do not identify a physical
unit. The UI describes this reference limitation. No proximity identity or
invented equipment IDs are accepted. The event instant is RFC3339, cannot be in
the future according to the injected server clock, and is separate from the
price survey/report day.

B-BR-MAP21-03: submit/correct/withdraw bind a stable operation UUID to owner,
canonical payload and resulting revision. Identical lost-response retries
converge; changed bodies, wrong owners, stale revisions and attempts to revive a
withdrawn report refuse. Existing hidden/moderated state cannot be cleared by
editing. Quota checks and mutation/receipt publication share the transaction.

B-BR-MAP21-04: persist before network; reject a full existing 50-command queue
without evicting prior work. Serialize enqueue/drain and preserve FIFO when a
command is unavailable/conflicted. Validate owner before/after network, partition
by owner/origin, never store access tokens in queued commands, and never paint a
late private result into another account. Retry via the existing WorkManager/
connectivity conventions with explicit parked errors and owner-scoped discard.
A local discard does not withdraw a report already accepted by the server.

BUC-MAP21: signed free user -> offline operation -> restart/reconnect -> same
operation accepted once -> edit -> lost reply/retry -> public dated history ->
withdraw; cross-account, replay, moderation, storage and timestamp failures.

## MAP-22

B-BR-MAP22-01: origin/destination/stops resolve through manual map/search,
selected admitted station entrance/reference or explicit current fix. Route
geometry, maneuvers, distance and duration come from the pinned native core.
Never substitute straight-line distances or claim live traffic/alternatives
that the core did not return. Unknown entrances are explicitly approximate.

B-BR-MAP22-02: cancellation/generation fences obsolete builds/searches. Native
missing-region/neighbor results produce required-map/download state. Avoidance
options are limited to actual SDK capabilities. Browsing/planning works without
GPS; missing data never becomes a drivable success.

BUC-MAP22: offline route with stops -> preview -> change/cancel; missing maps,
no route, restricted roads/entrances, origin=destination, lifecycle and stale
callbacks. Native fixture execution is distinct from fake-port unit evidence.

## MAP-23

B-BR-MAP23-01: explicit start/stop controls a location foreground service,
notification stop action and native follow/progress/reroute. Permission denied/
revoked, stale/poor GPS, missing maps, process death and OS start restrictions
surface recoverable states. Browsing never starts continuous GPS.

B-BR-MAP23-02: voice uses local Android TTS only, honors audio focus and rejects
network-only voices; guidance remains usable with textual maneuvers if no local
engine/voice is available. Calls/focus loss stop speech appropriately.

B-BR-MAP23-03: no trace recording, telemetry or public upload. Recovery metadata
is private, minimized and owner-bound; recovery requires explicit resume and
never starts tracking at boot. Logout/account switch stops owner guidance.

BUC-MAP23: emulator replay -> progress/maneuver -> wrong turn/reroute -> GPS loss
-> recovery -> stop from app/notification -> location/audio/native resources
released; no TTS, permission and lifecycle failures tested independently.

MAP-23 technical boundary (2026-10-26): car guidance accepts only finite fixes
with reported accuracy >0 and <=50 m, monotonic age 0..15 seconds and positive UTC
sample time. Poor/stale fixes never advance native progress or speech. Guidance
requires precise foreground location, enabled GPS and visible notifications
(API33 notification grant and enabled channel); planning remains permission-free.
Rerouting beyond 60 seconds surfaces a recoverable failure instead of pretending
success. Session/owner changes synchronously fence new fixes and stop resources.
SDK TTS auto-initialization is removed; the host initializes local voice only after
explicit service start. Native track recording is disabled before feeding fixes;
no track save or trace serialization is introduced. Recovery saves only the private
owner-bound planning request, never the sampled stream, and requires explicit resume.
