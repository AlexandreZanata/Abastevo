# Community-first compatibility and validation policy

Authority: [ADR-021](../adr/021-community-first-contribution.md), B-BR-CF01–CF07; implementation tracked by PRM15–PRM21. No live activation claim.

## Capture compatibility — PRM15-T02

New normal receipts use `photo-capture-community-v2`, never the developer preview policy. Requests omit location by default; optional supplied metadata is checked for forgery without evaluating distance or physical travel. Require a current active canonical station, signed owner/key, account/quota/idempotency and existing media controls. Camera window remains two minutes; submission/operational original-media deadline remains twenty-four hours. No background GPS or deadline renewal.

Already issued `photo-capture-v1` receipts retain their immutable policy, binding and deadlines and remain consumable until expiration. Replaying their idempotency key returns the original receipt; never reinterpret or renew it. `photo-capture-ui-test-v1` retains its explicit debug deadline and isolation. Unknown policy refuses use. No schema rewrite of applied migrations.

Deploy schema allowing v2 before the compatible backend, then the new Android consumer. New Android expects v2 for fresh normal capture; an old backend/v1 fresh response returns a recoverable unsupported-policy error, without falling back to developer mode or fabricating a fix. Old apps that decode only v1 need an update for fresh capture; their valid existing drafts remain supported. New backend receives optional old-client location metadata without granting proximity trust. Backend rollback must retain v2 receipt-consumption support or drain valid v2 drafts before reverting; never delete drafts or extend deadlines.

Owner/key/station/origin/evidence changes and malformed/forged optional claims still refuse. Authentication, private media integrity, bounded intake and all-copy expiry are not location rules.
