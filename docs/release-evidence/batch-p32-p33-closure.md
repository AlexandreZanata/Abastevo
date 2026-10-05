# Batch closure record — P26 (landing) + P32/P33 (profile) construction

Date: 2026-10-05. Scope: all mandatory next construction phases per the
roadmap dependency graph, Android-prioritized. iOS archived; G09
UNCERTIFIED; no deployment/tag/pilot authorized by this record.

## Integrated during this batch

- **P26 landing guides — INTEGRATED:** T01–T05 LOCAL_DONE on
  `codex/phase-26-landing-guides` (commits `6556a99`, `a84f5cb`,
  `c304400`, `33cc11f`, `0af287e`), merged to `main` as `938fc1f`
  (PR #117). Validation at merge time: landing build/check PASS,
  `npm test` 24/24, gate-selection 15/15, quick-verify ok (10s),
  diff-check + secret scan PASS.

## Code-ready, merge pending (LOCAL_DONE, INTEGRATION_PENDING)

- **P32 Android profile journeys** on `codex/phase-32-station-profile-app`
  (base P31 `1299e9c`): `bd732bf` (T01 public profile/badge),
  `85f010c` (T02 claim export-sign-import), `46da74b` (T03
  management/invitations/contest), `acb28db` (T04 offline/restart code
  acceptance + phase exit). Full domain/application/data/app suites
  0-fail, `:app:assembleDebug` PASS per task.
- **P33 fraud/ops/candidate** on `codex/phase-33-profile-acceptance`
  (base P32 `acb28db`): `d5f0eb0` (T01 adversarial containment +
  publish-window Busy/409 fix + harness race fix, 10/10 `-race` runs),
  `590613c` (T02 authority lifecycle lock), `cbddfc4` (T03 pinned
  candidate + `profile-review.md` runbook).

## Test backend `https://teste.abastevo.com.br`

- TLS unverified at every probe this batch (Fortinet middlebox CA
  `FG6H0FTB23902129`, curl exit 60, openssl `unable to get local
  issuer certificate`). No `-k`/trust-all bypass applied anywhere.
- Live contract/integrated-flow exercise stays UNVERIFIED; P34–P38
  Android/VPS code checkpoints already exist on their own branches
  (LOCAL_DONE, not merged). No live success inferred.

## Owed at conjunto closure (not executed here)

1. End manual/device batch (file-URI expiry, low-end memory, TalkBack/
   font scale, old-API fallback, provider callbacks, novice journeys,
   storage attach proof, performance budgets) — P38-T02 union matrix.
2. Required CI on current heads (`Quick verification` + backend
   fast/integration + Android signal).
3. Cumulative PR(s) reconciling `main` with the P32/P33 lineage (which
   diverged from pre-landing P31), guarded merge preserving commits,
   verified branch deletion.
4. Merged-SHA wiki sync (WIKI_PENDING until then).
5. P09/G09 real-production certification, then P10 public pilot. P11
  optional.

## Notes

- A parallel session merged PR #117 while this batch ran; P26 work was
  preserved (no re-creation, no overwrite). Main checkout was left
  untouched after that observation.
- Repository rename noticed: remote suggests `AlexandreZanata/abastevo`
  (pushes via the current remote name still succeed). Remote rename is
  owner-authorized work, not done here.
