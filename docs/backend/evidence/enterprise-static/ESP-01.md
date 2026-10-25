# ESP-01 — Admit source profiles, rights and public fields

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Date: 2026-10-24
Branch: `codex/enterprise-static-portal`
Parent: ESP-00 `88c9639f`.

Rules: B-BR-ESP-02–09/16; BUC-ESP-01/05.
[Plan](../../../planning/ENTERPRISE_DATA_STATIC_PORTAL_PLAN.md#esp-01--admit-source-profiles-rights-and-public-fields).
[Register frozen by this task](../../../product/ENTERPRISE_SOURCE_ADMISSION.md).

## Before behavior

The ESP plan named six candidate sources but did not freeze exact observed
resources, dataset rights, public fields, native keys, clocks, coverage,
retention, correction windows or quotas. Existing FQ/RST evidence admitted parts
of the registry and enforcement/fine streams, but the portal cannot treat a page
label, an existing parser or the site footer as a dataset license.

## Delivered

[Enterprise source admission register](../../../product/ENTERPRISE_SOURCE_ADMISSION.md)
freezes S-ESP-01–06 with three separate status axes (`ingest`, `runtime`,
`public_release`), a public-field allowlist, excluded private fields, clocks,
keys/dedupe, coverage, retention, quotas, correction windows and failure policy.
It records three rights classes instead of one blanket claim:

- `RIGHTS_PENDING`: registry, prices and PMQC — no dataset license found in the
  inspected metadata; local implementation allowed, public static release blocked
  until a compatible license/permission is recorded.
- `ADMITTED` under declared CC BY attribution: enforcement and fines, still
  blocked by their own runtime/association gates.
- `NOT_ADMITTED`: the ANP retailer API; no contract/coverage/quota evidence.

No source, schedule, import, API route, static route or deployment is enabled.

## Bounded real inspection, 2026-10-24

One GET per page with a descriptive user agent, 25 s timeout and 3 MB cap; no
archive/member download and no load campaign. Pages and metadata are outside Git.

| Page | HTTP / type | Bytes | Observed update |
| --- | --- | --- | --- |
| S-ESP-01 registry | 200 `text/html` | 264,737 | 2026-10-23 09:21 |
| S-ESP-02 recent prices | 200 `text/html` | 446,356 | 2026-10-23 16:29 |
| S-ESP-02 historical series | 200 `text/html` | 328,336 | 2026-10-20 13:30 |
| S-ESP-03 enforcement | 200 `text/html` | 264,240 | 2026-10-20 12:43 |
| S-ESP-04 fines | 200 `text/html` | 265,975 | 2026-08-24 11:27 |
| S-ESP-05 PMQC | 200 `text/html` | 270,945 | 2026-10-13 17:19 |

Metadata files were downloaded individually (bounded, outside Git) and inspected:

| Metadata | Media type observed | Bytes | SHA-256 | Finding |
| --- | --- | --- | --- | --- |
| Registry | `application/pdf` | 159,866 | `774259f8…ef8c98` | 2022 Print-to-PDF; no text layer |
| Historical prices | `application/pdf` | 191,035 | `cfe58226…2b8350` | fields/cadence; no license field |
| Enforcement | DOCX | 17,715 | `04917ce1…c5066cf` | CC BY/CC0/ODbL/PDDL menu; monthly |
| Fines | `application/pdf` | 108,518 | `0b2c4fd2…c0e36b` | CC BY; annual extraction |
| PMQC | DOCX | 84,545 | `9eaed144…144e3f` | 19-variable full-open schema; no license field |

The ANP footer states `CC BY-ND 3.0` for "todo o conteúdo deste site". The
register treats that as site-content terms, not as a dataset license, and does not
infer redistribution/derivative rights from it.

## Findings that change later tasks

- **Newest weekly price filename changed shape.** The page lists
  `revendas_lpc_2026_10_11-2026_10_17.xlsx` for the newest week and
  `revendas_lpc_2026-10-04_2026-10-10.xlsx` for the previous one. The current
  `official` discovery builds only the second shape, so it cannot discover the
  newest week. ESP-02 must accept the observed variants and derive the period from
  the verified filename, not from a link label.
- **Historical series is semester ZIPs** (`ca-2026-01.zip`), not the single CSV
  described in `docs/data-sources.md`. A backfill needs bounded ZIP/member
  handling; the existing price parser still owns the CSV/XLSX content.
- **Enforcement has no live Go stream.** FQ-06's adapter is bound to a synthetic
  CSV contract, while the page offers two XLSX panels. The raw columns, complete
  history and safe station links remain unaccepted; ESP-11 stays blocked.
- **Fines have a declared CC BY license and a strong native pipeline**, but safe
  Directory station association and runtime activation remain OWED. Event-level
  publication without a station claim is the only currently defensible slice.
- **PMQC metadata now describes full conforming/non-conforming publication** and
  still declares no license. It remains optional and `RIGHTS_PENDING`.
- **Rights are the main public-release blocker** for the core price/registry
  portal. Local construction, fixtures and internal analysis may proceed; ESP-18
  must not publish those slices until the maintainer records compatible rights.

## Checks executed

- Bounded `curl` availability/type/size matrix above; `curl -L` followed the
  public dataset pages; no source archive was downloaded.
- `pdftotext`/DOCX XML extraction of the five metadata files, followed by
  license/period/field inspection. The registry PDF has no text layer; this is
  recorded as unreadable rather than guessed.
- `sha256sum`, `stat` and `file -b` on the retained metadata copies.
- Local Markdown link/path check for the new register and evidence; pre-existing
  MAP evidence links in `PROGRESS.md` remain outside this change.
- `git diff --check`; scoped secret/PII inspection (no credentials, CPF, precise
  GPS, raw source rows or private documents added).
- No backend, Rust, landing or Android runtime test was run: this phase changes
  documentation only and activates nothing.

## Limitations and recovery

- Rights for registry/prices/PMQC are unresolved; the register does not convert
  the site footer into a dataset license.
- The inspection did not download a weekly workbook, semester ZIP, enforcement
  XLSX or fine CSV, so exact column/row coverage remains owned by ESP-02/03 and
  the existing FQ evidence. The synthetic enforcement contract is not claimed as
  the live schema.
- The correction windows and byte caps are Abastevo operating policies, not ANP
  guarantees; ESP-02/06 must test them against actual validators and failures.
- Rollback is to withdraw this register revision. It enables no source and
  requires no data cleanup.
- Next: ESP-02 — close the bounded discovery/correction gaps, starting with the
  observed weekly filename variants and no-ETag content rechecks.
