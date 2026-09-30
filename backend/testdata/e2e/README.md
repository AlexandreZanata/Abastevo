# E2E rehearsal fixtures (P09-T01)

Local disposable release rehearsal for the backend MVP (BUC-001…008).

- Candidate: `4ce5aaf` (P08 merge). The P09 branch product tree must match
  the candidate for `backend/`, `contracts/`, `infra/` and `scripts/`;
  docs-only deltas are allowed.
- `rehearsal.json` pins the synthetic stations, public-read scenarios,
  denied paths and source-separation invariants (B-BR-001/002/008/011).
- Signed-write flows (BUC-002…007) run in the existing real-PostGIS
  integration suites; this rehearsal adds the cross-module API smoke:
  directory + official + community projection served together with
  source separation intact.
- No production data, keys, photos or PII. Stations use the `c000…`
  namespace and are scrubbed after the run. Loopback only.
- Deployed staging (30-minute load, R2, TLS, provisioned host) is
  explicitly out of scope here; see `docs/release-evidence/p09-t01-rehearsal.md`.
