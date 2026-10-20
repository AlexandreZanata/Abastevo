# FQ-12 — Frozen PMQC sample-judgment methodology and independent oracle

2026-10-18 review addendum: affected acceptance claims are superseded by
[the independent review](FQ_REVIEW_20261018.md), including full sample scope,
methodology version, projection fencing and operator-only dead-letter recovery.

Status: LOCAL_DONE
Validation: PASS

Completed 2026-10-18 on maintained dev, parent c8066352 (FQ-05W8). The tested
behavior is the methodology contract and its real-data reconciliation; no
score, API, projection or runtime change was made (FQ-13 owns projections).
Delivery remains INTEGRATION_PENDING.

Before behavior: B-BR-FQ-08/09/16, BUC-FQ-05/06/09. Only per-assay source
conformity existed (`Conforme` Sim/Não), so a sample could only be judged by
the general all-conforming rule; the official Aspecto exceptions, replica
semantics, epoch limits, dimension contract and nullable score states were
not frozen, and no independent oracle existed.

## Inspected official guidance (verified local copy, 2026-10-17)

`orientacoes-analises-pmqc.docx` (ANP/SBQ, October 2026, "abertura integral",
83,667 bytes; sha256 pinned in FQ-02C/FQ-05W2 evidence context) freezes:

- The file unit is the assay (`Ensaio`); the judgment unit is the sample
  (`IdNumeric`, grouped with `Produto`). A sample is conforming when no assay
  is `Não`; any `Não` outside the Aspecto exception makes it nonconforming.
- Gasoline (RANP nº 807/2020): any `Não`, including Aspecto, is a failure.
- Ethanol (RANP nº 907/2022): a sole Aspecto `Não` fails only if the
  evaporation residue or its non-volatile material substitute is
  `Não`; absent complementaries leave the vice unconfirmed and the sample is
  treated as conforming for judgment.
- Diesel (RANP nº 968/2024, art. 16): heterogeneous product fails directly;
  particles need total contamination; turbidity needs water and total
  contamination (a first `Não` dispenses the second); absent complementaries
  leave the vice unconfirmed. Samples collected under the revoked RANP nº
  50/2013 follow the historical complementary sets by grade
  (S500: water / water and sediments; S10: water / total contamination).
- Replicas of the same `(IdNumeric, Ensaio)` must be handled explicitly;
  qualitative results and empty units must not be converted to numbers; CNPE
  content ranges change with collection date and never become fixed
  thresholds. The guidance does not carry the 968/2024 effective day.

## Frozen contract

`quality/domain/methodology.go` (`pmqc-sample-judgment-v2026-10a`):

- Replicas collapse per assay; mixed conforming/nonconforming replicas are a
  conflict and the sample stays UNRESOLVED (`replica_conflict`), never
  guessed. Unknown/INCONCLUSIVE statuses stay UNRESOLVED
  (`unknown_assay_status`).
- Fuel families come from the frozen `Fuel` enum; the Aspecto exceptions
  above are implemented verbatim, with reason codes:
  `ethanol_aspecto_complementary_conforming`,
  `diesel_aspecto_complementary_conforming`, `aspecto_vice_unconfirmed`,
  `diesel_defect_unclassified`, `diesel_rule_epoch_unverified`.
- Diesel defect classification keys on the official vocabulary
  (`Heterogêneo`, `material particulado`, results opening with `Turvo`);
  unknown wording is UNRESOLVED, not classified by guess.
- Diesel epoch envelopes: collection year ≥ 2026 uses the current rule (the
  guidance's own open-data frame), ≤ 2023 the historical rule; 2024–2025 is
  epoch-unknown and resolves only when both rules agree, otherwise
  UNRESOLVED. No effective date was invented.
- Dimensions (`ProjectFuelDimensions`) expose distinct samples, distinct
  visits (station + collection day), per-verdict counts, oldest/newest
  collection days and `UNSCORED`; `MethodologyLimitation` states that no
  numeric weights or sufficiency thresholds are frozen. Nothing converts
  qualitative results or filters samples by sparse counts.

## Independent oracle and real-data reconciliation

A standalone stdlib-Python oracle (`/tmp/fq-oracle/oracle.py`, not committed:
transient tooling) re-implements the guidance text independently and was run
first; the Go domain then reproduced it exactly on both real datasets
(env-gated test `methodology_oracle_test.go`). Frozen September numbers are
asserted in the test:

| Dataset | samples | gasoline C/NC/U | ethanol C/NC/U | S10 C/NC/U | S500 C/NC/U | reasons |
|---|---|---|---|---|---|---|
| PMQC_2026_09.csv (sha256 in FQ-05W2 evidence) | 4,164 | 1,457/129/0 | 1,049/29/1 | 1,018/39/3 | 424/15/0 | 5 cleared, 1 defect-unclassified, 3 replica conflicts |
| 2025/PMQC_2025_01.csv (annual member) | 5,549 | 1,979/51/0 | 1,560/44/0 | 1,260/108/2 | 498/47/0 | 2 unconfirmed, 2 replica conflicts |

Real findings recorded, not smoothed: 32,071 repeated (sample, assay) rows in
September; three samples with conflicting replicas (blank-result `Não` versus
measured `Sim`, e.g. `Massa Específica a 20°C` 1058347 / `Ponto de Fulgor`
1059071, 1060502) stay UNRESOLVED; sample 1062530 (`Límpido com água livre`,
`Teor de Água` Sim) is UNRESOLVED because the guidance vocabulary does not
classify that defect wording; the 12 Aspecto `Não` samples in September
resolve per the rules (6 diesel cleared, 4 confirmed, 2 gasoline failed).
No `Não` was converted into a failed sample.

Bias and limits frozen: historical archives before the open-data change are a
positive cadastre (conforming-only), so nonconformity dimensions are
structurally absent for those series; CNPE content ranges vary with the
collection date; qualitative assay values are opaque text; diesel epoch
2024–2025 stays conservative; no numeric methodology is published while
calibration cannot support it. Official findings, private community reports
and payments remain separate categories (FQ-11/09 unchanged).

## Validation

```sh
ANPFUEL_TEST_PMQC_CSV=/tmp/fq-official-20261016/PMQC_2026_09.csv go test -count=1 ./internal/modules/quality/domain/ -run TestPMQCMethodologyAgainstRealOpenData -v
ANPFUEL_TEST_PMQC_CSV=/tmp/fq-oracle/PMQC_2025_01.csv go test -count=1 ./internal/modules/quality/domain/ -run TestPMQCMethodologyAgainstRealOpenData -v
go test -count=1 ./internal/modules/quality/domain/
gofmt -l internal/modules/quality/domain
```

PASS: 21 judgment contract cases plus dimensions; both real files match the
Python oracle and the frozen counts; unit domain suite green. No dependency
addition; `git diff --check` and the scoped secret scan cover the change. The
oracle script and extracted CSV live only in /tmp and are not committed.

## Explicit next obligations

FQ-13 consumes this frozen contract for deterministic projections and
recomputation; numeric scores stay unavailable until calibration is accepted.
FQ-14–FQ-17 and the final protected dev→main batch remain; D is
NOT_AUTHORIZED.
