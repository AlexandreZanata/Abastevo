# P09-T04 functional multiplatform planning evidence

Date: 2026-09-30. Branch: codex/phase-09-production-validation; base c51fa03; [PR #14](https://github.com/AlexandreZanata/brazil-fuel-prices/pull/14). Scope: policy/docs and deferred G09 record validation; no new app/account/social/media feature implementation.

## Requirements and boundaries

User confirmed 280-character comments/replies and free signup by email access code, Google and Apple. ADR-014 defers real G09 until functional G18, while protected G09-LOCAL integration opens app development. ADR-015 and the source/official compatibility audit define KMP/Swift work. P12–P18 add 31 implementation tasks, preserve P10/P11 anchors, and separate free accounts from later billing. Stars/percentages, privacy, media budgets/expiry, mock-location limitations and platform evidence have explicit owning tasks; all future gates remain unpassed.

Issues #11/#12 cover this correction/planning batch; #13 is the deferred release tracker and stays open. Publishing authorization covers this phase's records, PR, guarded merge and owned-page wiki snapshot; no real deployment/tag/release.

## Scoped verification

- RED: new fixture/mutant campaign against the old G09 checker accepted the unsupported certified mutant, failing as intended. It exposed the old checker's ignored input path.
- GREEN: `bash scripts/tests/test-g09.sh` accepts deferred release and refuses unsupported certification, missing app prerequisite, missing legal condition, missing integration prerequisite and missing record. Fixtures use mktemp/trap and never mutate canonical sign-off.
- `bash -n scripts/check-g09.sh scripts/tests/test-g09.sh`: PASS.
- Focused documentation check: 111 unique task IDs, complete P12–P18 field templates, existing task dependencies and relative links in all seven new canonical documents: PASS. Validator source and output are retained in /tmp/anpfuel-mobile-plan/ for this local session.
- `bash scripts/tests/test-issues.sh`: 23 passed, 0 failed; reconciliation remains idempotent and does not close/reopen historical issues.
- `bash scripts/scan-secrets.sh` and `git diff --check`: PASS. New documentation reviewed for secrets/PII/GPS/photo data; none added.
- Local/remote Quick verification is the final integration gate on the eventual committed head, recorded in PR metadata. This document does not preclaim that result or its own commit SHA.

The existing product correction evidence belongs to 8c5e795 and its recorded source fingerprint. Documentation changes do not rerun the full PostGIS/storage/load/restore campaign; final phase quick still covers the merged code diff. macOS/Xcode, native provider/device flows and revised media enforcement remain future implementation acceptance, not outcomes of this planning task.
