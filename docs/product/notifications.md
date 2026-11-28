# Notifications: in-app inbox, account events and local alerts

The bell in the app header opens **Notifications**, one list of everything the
app told the person, newest first. Items come from two places that never mix
their data:

- **Account notifications** are created by the backend for a signed account
  (someone replied to your forum topic, your community price was validated or
  rejected). They live in the server inbox and follow the account.
- **Local alerts** are created on the device from data the device already has
  (a fuel got cheaper in your city after a new ANP week, a tracked vehicle
  price dropped, a new ANP week arrived). They are never sent to the server.

Push delivery through a provider (OneSignal, see
[notification content](notification-content.md)) stays future work; this
contract does not need any push SDK.

## B-BR-NOTIF-01 Notifications screen

- The header bell opens Notifications on every screen that shows it; Settings
  stays reachable from Perfil and from the screen's settings action.
- One list, newest first: account notifications (signed accounts only) and
  local alerts. Each row has an icon per kind, a title, one or two lines of
  body, the relative time and an unread dot.
- "Mark all as read" marks every listed item read; opening an item marks it
  read and opens its target (forum topic, My contributions, Explorar).
- Signed out, the screen shows local alerts and a sign-in hint for account
  notifications. Offline, it shows local alerts and the last account page it
  read, marked as not refreshed. Empty: an empty state with a link to
  notification settings.

## B-BR-NOTIF-02 Account notification inbox (backend)

- Table `account_notifications`: one row per `(account_id, event_key)`, so a
  repeated or retried event never notifies twice. Rows expire 30 days after
  creation; reads skip expired rows and writes purge a bounded number of them.
- Producers insert in the same transaction as the event they describe,
  through an explicit port of the notifications module. A producer never
  notifies the actor about their own action.
- Only active accounts receive rows; account erasure deletes the inbox in
  batches and deleting an account row cascades to it.
- v1 kinds and payloads (JSON object ≤ 2 KiB, no email, no GPS, no tokens;
  author names are the public forum alias already visible on the topic, empty
  when the reply has none):

| Kind | Producer | Payload |
| --- | --- | --- |
| `forum.reply` | the first publication of a reply on a topic of the account: comments are premoderated, so this happens when an operator approves it (`ReviewCommentTx`), only while the topic (and the parent, for an answer) is published; re-approval after an edit stays silent; parent-comment authors are not notified | `topic_id`, `topic_title` (≤ 160), `comment_id`, `author_name` (≤ 80) |
| `contribution.validated` | a community observation attributed to a contributor bound to the account reaches a VALIDATED decision (`RecordDecisionTx`, used by the worker and by moderators) | `observation_id`, `station_id`, `station_name` (≤ 160), `fuel` (wire product, e.g. `ETHANOL`) |
| `contribution.rejected` | the same observation reaches a REJECTED decision, including a later moderator invalidation (so one observation may produce both notices) | same as validated, plus `reason`: `invalid-station`, `invalid-supersedes`, `evidence-owner-mismatch`, `evidence-timeout`, `evidence-reused`, `contributor-blocked` or `moderation-invalid` |

- Unknown kinds are skipped by the app, so new kinds can ship server-first.

## B-BR-NOTIF-03 Account notification API

- `POST /v1/notifications/list`, a private session read like the ledger record
  pages: `{family_id, access_token, limit (1..50, default 30), before?}` →
  `{items: [{notification_id, kind, created_at, read, payload}], next_before,
  unread}`. `before` is the sealed `next_before` cursor (bound to the account,
  valid for 15 minutes); `next_before` is null on the last page.
- `POST /v1/notifications/read`, a signed write (device key bound to the
  account): `{family_id, access_token, operation_id, notification_ids (1..50)}`
  or `{..., all: true}` → `{marked}`, the number of the caller's live
  notifications the request covers (all read afterwards), so a replay answers
  the same. Ids of other accounts are ignored, never revealed.
- Both are `Cache-Control: no-store`. Any session or proof failure returns the
  generic `401 notifications.auth-required`; a malformed body, limit or cursor
  returns `400 notifications.invalid`; a store failure returns 503.

## B-BR-NOTIF-04 Account notifications on the device

- While signed in, a periodic background job (at most every 3 hours, network
  required, battery not low) reads the first page. For each unread item it has
  not shown before it posts an Android notification in the "Account updates"
  channel: at most three per run, then one summary. Shown ids are remembered on
  the device (bounded to the last 200).
- No Android notification is posted without the OS notification permission;
  the inbox still shows every item.

## B-BR-NOTIF-05 Local alerts

- Created on the device after a successful ANP weekly import, from local data
  only:
  - `local.city_average_drop`: a fuel's weekly average in the selected city
    fell from the previous imported week (one alert per city and week, listing
    the fuels that fell, largest drop first);
  - `local.vehicle_price_drop`: the existing per-vehicle alert (UC-014);
  - `local.new_week`: a newer ANP survey week was imported.
- Every local alert is kept in a device inbox (last 100, 30 days) and listed on
  the Notifications screen. An Android notification is posted only with the OS
  permission, in the existing price-alert channel.
- Read state of local alerts is device-local.

Code: backend module `notifications` (`backend/internal/modules/notifications`),
Android `ui/notifications`, local inbox in `data/local/notifications`.
