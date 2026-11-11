# F0 remaining decisions for M0

Status: **M0 LOCAL_DONE / INTEGRATION_PENDING**, 2026-11-11.
[ADR-025](../../adr/025-private-abastevo-empresas-organization-boundary.md)
settles the private organization boundary. On 2026-11-10 the maintainer
approved the company panel at `/empresas/`, Android partitioning by
account, server origin and organization, and private C1 receipts with
real-receipt retention subject to legal review. The
[product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) records
the approved scope. See the
[M0 exit criteria](../14-roadmap.md#3-entry-exit-and-activation-per-milestone).

## Private receipts

EMP-GOV-002 · OQ-04 · CX-05 · CX-07 · EDR-014 · LV-11

**Accepted for C1 design and synthetic validation:** a separately scoped
private Empresas document. One authorized member attaches a fueling receipt
to a record in their organization. The server keeps it in the private
organization namespace, checks the document permission on every access and
audits content reads. A short-lived member-bound capability is the only
delivery mechanism. Keep receipt reception and human review separate from
fiscal status. C1 can start with synthetic fixtures and manual review.

**Pending legal review:** retention and real-receipt activation. Ask legal and fiscal
reviewers to distinguish an operational receipt from a document supporting
statutory bookkeeping. The five-year figure in the plan is a hypothesis
for review. Confirm applicability, the clock start, justified extensions,
holds and whether originals are required. The CTN ties retention of the
covered records to the tax claims arising from their transactions. Treat
that provision as an input to the category-specific review.
[CTN, arts. 173, 174 and 195](https://www2.camara.leg.br/legin/fed/lei/1960-1969/lei-5172-25-outubro-1966-358971-normaatualizada-pl.html)

Record the approved purpose, category, retention period and trigger,
access roles, erasure and backup handling before enabling real receipts.
Community photos and claim scans continue under their current rules. Stage
D purchase evidence, fiscal retrieval, long-term retention and sale
activation keep their own decisions. Acceptance must cover foreign or
expired capabilities, audited reads, deleted originals and derived copies,
and expiration while cleanup is unavailable.
[Privacy design](../11-privacy-security.md#101-retention-schedule-defaults-every-period-needs-legal-validation)

## Company web surface

EMP-GOV-003 · OQ-05 · EDR-006 · EDR-019

**Accepted company panel:** serve a standalone static TypeScript panel at
`/empresas/` on the configured API origin. Use the existing admin build
and testing pattern as precedent. Keep a distinct site build, account
session store and middleware. Use a dedicated host-only, Secure,
HttpOnly, SameSite=Strict cookie with the `__Host-` prefix and `Path=/`.
Use exact-origin and CSRF checks for mutations, and fresh account
authentication for session issuance. The server resolves current
organization authority for private routes under `/v1/orgs/{org_id}/`.

The cookie path spans the host. Prove that only the dedicated business
middleware accepts it, and that the business session cannot authorize any
admin route. Keep the privileged admin origin and its network fence
separate from the business web. Cache static assets independently from
private responses, which must use `no-store`. Test foreign origins, CSRF,
expired sessions, staff-cookie confusion, path-scope switching and
private response caching before activation. Postos stays behind its
separate gate.
[Admin HTTP regression tests](../../../backend/internal/modules/admin/adapters/http/http_test.go)

## Legal review package

EMP-GOV-005 · OQ-02 · OQ-14

Status: **PACKAGE READY / REVIEW PENDING**. The owner designates privacy,
employment and fiscal reviewers.
For each question, record the reviewed feature and data category, the
applicable source, the permitted purpose and controls, unresolved risks,
the reviewer, and the decision date. Use the full inventory in the
[privacy plan](../11-privacy-security.md#104-points-that-need-legal-validation).

- **LV-01, LV-03, LV-05 and LV-06.** For work-trip location and the
  operational trip record, review the lawful basis, necessity, expected
  data recipients, visible recording indicator and employee notice.
  Confirm permitted wording and exports given the separation from time
  recording and payroll. Ask whether a documented impact or balancing
  assessment is required before field use. [ANPD legitimate-interest guidance](https://www.gov.br/anpd/pt-br/assuntos/noticias/anpd-lanca-guia-orientativo-sobre-legitimo-interesse) · [STF record of ADI 5322](https://portal.stf.jus.br/noticias/verNoticiaDetalhe.asp?idConteudo=510120)
- **LV-02, LV-08, LV-09 and LV-10.** Map controller and operator duties
  by purpose, including employee rights requests, platform security
  records, incident handoff and sensitive content in uploaded material.
  Record the responsible party and escalation contact for each flow.
  [LGPD, arts. 5, 6, 18, 37–39 and 46–48](https://www2.camara.leg.br/legin/fed/lei/2018/lei-13709-14-agosto-2018-787077-normaatualizada-pl.html) · [ANPD incident regulation](https://bibliotecadigital.mj.gov.br/bitstream/1/12879/2/RES_ANPD_2024_15.html)
- **LV-04, LV-07 and LV-11.** Confirm retention by data category and
  inventory the storage, backups, support access and subprocessors by
  region. Record the permitted transfers, if any, and the required
  contract terms before enabling real data. [ANPD international transfer regulation](https://www.gov.br/anpd/pt-br/acesso-a-informacao/institucional/atos-normativos/regulamentacoes_anpd/resolucao-cd-anpd-no-19-de-23-de-agosto-de-2024)
- **LV-12 and LV-13.** Keep Postos evidence and Data redistribution in
  their owning reviews. Carry forward the existing Stage D and source
  rights gates.

Expected outputs are a role and processing inventory, a retention and
transfer decision, approved notices and processing terms, and a documented
go/no-go for each real-data capability. The package is prepared for review.
Real-data pilot and tracking authorization still require recorded findings
and the applicable project gates.

For the receipt decision, the review record must name the allowed document
categories and purposes, any required original, the retention period and
starting event per category, and the hold, erasure and restore behavior.
Record region and processor restrictions, an identified reviewer and date,
the approved version of the policy, and an explicit activation decision.
Receipt intake stays restricted to synthetic fixtures until those findings
are recorded and the real-data gates pass.

## Android local protection

EMP-GOV-008 · OQ-06

**Accepted partition:** scope enterprise local data and queued operations by
`(account, server origin, organization)`. Store the account and origin
with the organization scope and use the scope active at operation
creation for retries. Protect private records and files with
Keystore-backed keys per partition. Exclude the store from backup and
device transfer. The implementation must include database journals,
temporary attachments and derived caches in the protection boundary.

Lock the partition and remove session authority at logout. Another account
or origin must never reopen it. Apply server revocation immediately to
network access and handle local cleanup on contact. Record and test the
offline validity and device-clock behavior before relying on expiration
for key disposal. A disconnected device cannot receive a revocation.
Check process death, restore on another device, wrong account/origin/org,
logout with unsent work, clock changes and failed cleanup. The existing
Keystore wrappers and backup rules are precedents. Verify whether they
cover the planned enterprise store before selecting an encryption
implementation.
[Keystore device tests](../../../data/src/androidTest/kotlin/com/anpfuel/data/local/auth/KeystoreAccountStorageDeviceTest.kt) · [Backup rules](../../../app/src/main/res/xml/backup_rules.xml) · [Data extraction rules](../../../app/src/main/res/xml/data_extraction_rules.xml)

## Roadmap sequencing

EMP-GOV-012 · OQ-03 · CX-09

**Accepted on 2026-11-10:** the maintainer confirmed that local F1
construction starts only after M0 closes and the current
[ADR-019](../../adr/019-android-vps-integration-first.md) construction batch
is integrated through protected `main`. The
[repository roadmap](../../../ROADMAP.md#phase-order) records that sequence.
Verify the integration against the batch evidence and the merged revision
before starting F1. A local checkpoint or an unrelated merged PR cannot
satisfy that condition. Real-data use requires G09, the applicable pilot
gates and legal review.

## Documentation validation

All 21 canonical backlogs and all ten formerly missing design documents
are present. The original 348 task IDs, titles, phases, cuts, sizes,
priorities and dependencies match the original skeleton. All 87 incomplete
traceability rows now reference defined contracts and acceptance outcomes.

The full checker passes with 348 tasks, 545 business rules, 167 use cases
and 145 screens. The generated-index regression has three passing tests.
Two consecutive generations produce byte-identical Markdown. Links,
headings, new-file whitespace and secret surfaces pass the scoped review.

[The F0 local checkpoint](f0-local-checkpoint.md) records commands,
integration evidence and retained obligations. M0 is locally complete.
Publication of this enterprise batch remains pending.

The current main and dev bases are synchronized at `d8bdc642`. The
P38, catalog and station-profile source checkpoints and integration PR177
are ancestors of that base. GitHub confirms PR177 merged, all four
recorded checks on its head succeeded, and main has strict
`Quick verification` protection with admin enforcement.

This satisfies the recorded source-integration condition for local F1.
The existing P38 device acceptance and G09/legal/real-data obligations
retain their own evidence and gates.

## Remaining governance work

- Review staff responsibilities, managed credentials and delivery channels
  alongside F1 planning. Preserve the separate staff session and use the
  invitation path for C1. EMP-GOV-004, EMP-GOV-013 and EMP-GOV-014.
- Plan the synthetic tracking spike before deciding map and routing needs.
  Reserve device coverage and review the location questions before a
  real-data tracking test. EMP-GOV-006 and EMP-GOV-007.
- Use the [discovery protocol](f0-discovery-protocol.md) when interviews
  are separately authorized. Use the results to inform the organization
  billing decision. Review fiscal consultation after the legal package.
  EMP-GOV-009, EMP-GOV-010 and EMP-GOV-011.
- Schedule a distinct Stage D scope decision before F7 and a redistribution
  rights review before the Data product can sell public-source data.
  EMP-GOV-015 and EMP-GOV-016.
