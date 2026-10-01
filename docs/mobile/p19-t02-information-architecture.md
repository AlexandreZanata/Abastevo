# P19-T02 — Frozen community information architecture and prototype

Status: LOCAL_DONE (contract slice) on `codex/phase-19-product-identity-foundation`. Issue: #72. Binds B-BR-C01–C06 / BUC-C01–C05 (`docs/product/COMMUNITY_EXPERIENCE.md`) to existing contracts. No app/backend code changed in this task.

## Frozen navigation (prototype map, not implemented screens)

Three tabs + one persistent action, per product spec:

- **Explorar:** city + fuel selector → station list first (recent community price, observation time, condition badge, support/state) → station detail → route action. Search, distance/price filters and sort explicit. Map optional only after P20 provider/privacy audit — not in this slice.
- **Comunidade:** local station activity, contributions, discussions (structured records, no endless photo gallery). Favorites kept local; follows/alerts only per P23 contracts.
- **Perfil:** free account, contribution status/history, rights/privacy, settings. Legacy expert tools (vehicles, tank calc, ANP history, analysis) stay reachable under clearly named entries — none removed.
- **Atualizar preço:** persistent labeled primary action (not a tab, not onboarding gate) → capture → review/edit → submit → status.

Guest explores first (BUC-C01); registration is required only for comment/reply/rate/vote (BUC-C03). Camera/location are requested when useful (BUC-C02/C04), never up front. Registration interruptions restore the draft without duplication (existing outbox/idempotency preserved).

## Frozen source/condition hierarchy (grounded in `BackendPriceGroup`)

- Primary price is the community current-price projection (fuel, exact BRL thousandths, unit, conditionKind, provenance, timestamp). Until its consumer exists, community stays null/UNKNOWN — never an ANP substitution (`BackendPriceGroup.create` rejects non-null community; model comment: UNKNOWN, never ANP substitution).
- ANP appears only as a dated secondary reference (`BackendOfficialSection`: source, amountMilliBrl, currency BRL, collectedOn, surveyWeek range, revisionId). Missing community coverage says so explicitly; stale/disputed/unknown states follow server rules (B-BR-C01).
- Condition labels come from `conditionKind` verbatim; no unconditional cheapest-price ranking for loyalty/app-only prices (B-BR-C01/C06).
- Social signals stay separate: personal stars, 280-char comment/reply agreement, price confidence (B-BR-C04). No votes means no percentage.

## Frozen state matrix (presentation must implement in P20+)

Fresh / stale / disputed / unknown / no-coverage / offline-cached / expired-media / illegible-photo / wrong-OCR / condition-ambiguous / rejected / timeout-retry / rate-limited / moderation-removed. Every state has an explicit empty/error copy; invented coverage, mock members and estimated-as-real prices are forbidden.

## Usability validation — explicitly pending

Novice-driver comprehension testing (guest discovery, permission timing, source/condition hierarchy, proposed navigation) is **NOT RUN** in this slice: no participants recruited, no sessions held, no observations to record. Protocol (sample: novice drivers + maintainers; tasks: first price lookup, source/recency identification, photo contribution; pass thresholds + remediation) is defined here so P19-T04/P24 can execute it. A maintainer-approved prototype review may serve as early evidence; it is not a substitute for novice testing. This gap is recorded, not waived, and blocks any claim of validated UX.

## Validation

- `git diff --check` clean (docs-only slice).
- Scoped secret review: no secrets/PII/GPS/photo content.
- Preserved contracts: `./gradlew :domain:test --no-daemon` (record outcome in commit evidence).
