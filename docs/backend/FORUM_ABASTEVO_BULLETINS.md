# Forum Abastevo — deterministic official bulletins

Status: **FROZEN METHODOLOGY / SOURCE PORT LOCAL_DONE** (FOR-25, 2026-11-06).
Scope: input selection and exact aggregates only. Templates, display rounding
and publication ride FOR-26/FOR-27. No bulletin is published by this document.

## Version and authority

- Methodology: `forum-official-mean-v1`.
- Input owner: Official (`official_revisions`,
  `official_station_prices`, `official_import_runs`,
  `official_current_revisions`). Forum never reads a single price row to infer
  representativeness.
- Product rule: [B-BR-FOR-15](../product/FORUM_ABASTEVO_CONTRACT.md) and
  [BUC-FOR-08](../product/FORUM_ABASTEVO_CONTRACT.md).
- Source rule: [FORUM_ABASTEVO_SOURCES.md](FORUM_ABASTEVO_SOURCES.md) — only
  accepted internal Official imports. Community and charging observations are
  excluded from the first statistical template.

## Comparison window

The first bulletin compares two **accepted complete weekly survey revisions**:

1. the current published revision of the latest survey window in
   `official_current_revisions`;
2. the current published revision of the latest earlier window whose
   `survey_end` is strictly before the latest window's `survey_start`.

Both revisions must have `status = 'published'` and a current pointer. Staging,
failed, needs-review, superseded and orphan revisions are never inputs. If fewer
than two eligible windows exist, there is no bulletin. A same-window corrected
import replaces the window's current pointer and is the revision used.

## Sample and deduplication

For one UF, one Official fuel wire value and one unit:

- take the latest observation per station in the revision, ordered by
  `collected_on DESC, id` (the existing ranking rule);
- a station contributes at most one observation, so duplicate source rows are
  counted in coverage but not summed twice;
- geography is the station's canonical `directory_stations.state`; stations with
  no canonical state are excluded from UF bulletins and never guessed;
- the same fuel and unit apply to both revisions; mixed units are refused.

The result is a **cross-sectional** comparison of two samples. It does not
measure same-station price change, national fleet behavior, ANP's published
mean, statistical confidence or future prices.

## Eligibility

A UF/fuel/unit pair is eligible only when:

- both revisions are present;
- both samples have at least **30 distinct sampled stations**;
- both sums and observation counts are positive;
- the query fuel/unit pair is in the frozen Official ranking vocabulary.

The 30-station floor is a conservative product threshold, not a statistical
significance claim. Ineligible or absent regions produce no bulletin and a typed
skip reason for the future operational record.

## Exact aggregate

For each revision and UF:

- `sum_milli_brl`: exact integer sum over the deduplicated sample;
- `observation_count`: count of deduplicated observations;
- `sampled_stations`: distinct stations in the sample (equal to the count);
- `source_observations`: raw Official rows for the fuel/unit before dedup;
- `source_stations`: distinct stations with any Official price in the revision
  and UF (coverage denominator);
- mean: exact rational `sum / count`, reduced with positive denominator. FOR-25
  stores numerator/denominator; display rounding is frozen in FOR-26;
- coverage flags: sample is sampled (`sampled = true`) and never claims national
  completeness.

Sums are read from PostgreSQL `numeric` text and must fit a positive `int64`
milli-BRL value. Overflow is a typed refusal, never a wrapped value.

## Input manifest

Every computed set carries, for both revisions: revision UUID, survey
`start`/`end`, `published_at`, source URL, source checksum and parser version;
plus methodology version, fuel, unit and query version. This manifest is the
reproducible provenance for FOR-26 templates and FOR-27 review.

## Refusals and non-goals

- No LLM, network or invented sentence in this task.
- No community or charging observation enters the first template.
- No national aggregation or unobserved-region extrapolation.
- No publication, source activation or Android surface is implied by the
  aggregate port.
