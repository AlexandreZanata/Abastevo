# Optional AI-training image consent amendment

State: PROPOSED_USER_REQUESTED / NOT_IMPLEMENTED, 2026-10-07. Owner: P37-PC05 in the [photo contribution plan](../planning/PHOTO_PRICE_CONTRIBUTION_PLAN.md). The user chose 24-hour storage for non-consenters and permanent training-image storage for consenters. This document defines the proposed exception; it does not alter current runtime retention or approve a legal notice for release.

## Purpose and retained-copy boundary

Use explicitly authorized fuel-price image regions and necessary corrected labels to improve recognition of station fuel prices. Training consent is separate from permission to process an image for the contribution itself. No sale, public gallery, unrelated model use or automatic third-party model upload is authorized by this scope.

All operational original/quarantine/sanitized/thumbnail/local-outbox copies still use the [existing 24-hour policy](LOCAL_MEDIA_LOCATION_POLICY.md): server first receipt plus 24h, local capture plus at most 24h, no reset through crop/retry. Consent does not make these copies permanent. Non-consenters have no training copy. Retain only a separately authorized, sanitized/minimized private training derivative with **no automatic TTL** while training remains a valid purpose and consent remains applicable. In product terms this is permanent storage, not an irrevocable right to retain personal data forever.

Revocation and erasure must be practical: the [ANPD's rights guidance](https://www.gov.br/anpd/pt-br/assuntos/titular-de-dados/direito-dos-titulares) describes withdrawal of consent and erasure rights. The [LGPD](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709compilado.htm) requires a defined purpose, necessary processing and transparent information, including duration and controller details. This design uses those sources to specify controls; final legal adequacy and the public controller/contact/processor notice remain a release obligation.

## B-BR-AT01–AT06

- **AT01 — Free choice:** unchecked by default; contribution remains available without training consent. Do not bundle it with mandatory service terms, give a trust/price/reputation bonus or interpret silence/old app versions as agreement.
- **AT02 — Prove scope:** store purpose/version, authenticated owner reference, server consent timestamp, active/revoked state and permission applicable to the individual image. A local boolean or consent at initial account signup alone is insufficient. Keep a minimal audit receipt separate from training pixels; retention of receipt/ownership links requires its own justified policy, not an unlimited identity trace.
- **AT03 — Promotion:** before operational expiry, a restricted worker checks valid per-image permission and current consent, minimizes to the fuel-price region, strips EXIF/GPS and removes identifying image content (watermarks, faces, plates, contact details). Removing EXIF alone cannot remove text burned into pixels. Uncertain sanitization prevents promotion. No original-photo archive as a fallback. An expired/deleted source cannot be restored just to train.
- **AT04 — Private collection:** separate restricted storage namespace/bucket and authorization purpose, no public URL/CDN or account-photo browsing. Least-privilege operators/workers, encryption, inventory and purpose-limited label access; prohibit contributor coordinates in labels. Avoid unnecessary cross-user image fingerprinting. Existing evaluation originals have no retroactive training consent.
- **AT05 — Revocation/deletion:** immediately exclude withdrawn images/labels from future promotion/export/training selection; remove derived dataset copies, exports, object versions and affected backups through a tested deletion ledger and restore filter. Define and measure a finite deletion SLA before enabling collection. Re-check revocation at worker commit/export/train boundaries; serialize or revalidate racing promotion. Reject stale offline consent commands and make repeated requests idempotent. Account erasure includes the collection's ownership-linked assets.
- **AT06 — Model governance:** track dataset membership and model provenance to respond to withdrawal. Do not promise that deleting source images automatically removes every influence from an already trained model. Define retraining/unlearning or other appropriate response and truthful user disclosure before any model-training release. Exporting to a third-party trainer is a new reviewed purpose/processor decision, not implied permission.

## Proposed UX copy (Portuguese, draft)

Optional unchecked checkbox beside the final review, never required to enable submit:

> Autorizo o uso das imagens de preços que eu enviar e dos preços corrigidos por mim para treinar e melhorar a IA de reconhecimento do Abastevo. As imagens autorizadas poderão ser guardadas por tempo indeterminado em uma coleção privada de treinamento. Esta autorização é opcional e posso revogá-la em Perfil → Privacidade.

Supporting notice:

> Sem esta autorização, suas fotos serão usadas apenas para processar e verificar a contribuição, com prazo máximo de 24 horas nas cópias do serviço. As cópias operacionais seguem esse prazo mesmo quando você autoriza treinamento. O cache do aplicativo também expira; se o aparelho estiver desligado, a limpeza física ocorre quando ele voltar a executar o aplicativo.

Actions: `Ler os termos de uso das imagens`, `Revogar autorização` and `Solicitar exclusão das imagens de treinamento`. Explain the actual deletion SLA and previously trained-model limitation in the detailed notice once implemented. Preserve the selected per-image decision in the durable review; a later user grant never reuses older images silently. A saved preference can be convenient, but the review must clearly show which images are authorized and permit opting out of that submission.

The detailed public terms must identify the controller/contact, purpose, image/label categories, operational and training durations, recipients/operators, security/access boundaries, rights channel, withdrawal/deletion procedure/SLA and actual model-use limitations. These owner-specific details are not invented here. Publish versioned Portuguese terms and link them from review/Profile before activating collection.

## Test-only controller/contact decision

On 2026-10-07 the owner requested a placeholder email until production.
Use `Abastevo — test environment` and `privacidade@abastevo.example.invalid`
in restricted test terms/configuration. This reserved `.invalid` address is
not a functioning rights channel. Production activation must require actual
controller/contact details and versioned terms; a placeholder cannot satisfy
that release gate. Local consent/deletion controls can be exercised with the
test configuration without claiming public legal readiness.

## Required source changes and proof

First freeze domain permission and deletion semantics; then backend consent receipt/current-state ports and minimal append-only schema, signed owner-only grant/revoke/list/delete APIs with OpenAPI changes, promotion worker/storage policy, privacy notice/export/erasure, finally Android unchecked checkbox/settings/review snapshot. Reuse account/proof/jobs/private S3 and Go/PostGIS conventions. No standalone speculative training platform is needed.

Immediate critical tests: default/absent/unsupported/stale consent denies collection; cross-account IDOR/signature/replay; double grant/revoke; queue after revoke; grant→revocation→promotion races; copy/export/train racing deletion; storage/job failure; expiry before copy; account erasure; all object versions/backups/restore; visible watermark/face sanitization failure; orphan derivative reconciliation. Ordinary evidence still expires at 24h with consent ON, OFF or worker failure. Collection outage never breaks a valid contribution. Use synthetic photos for Git tests and private isolated storage for actual deletion evidence.

Update `LOCAL_MEDIA_LOCATION_POLICY.md`, backend privacy notice, terms, OpenAPI and P37-T03 acceptance in the same implementation slice. Until that checkpoint, the exception stays proposed and collection disabled. Release requires real private-storage provisioning/deletion/restore evidence; a plan, checkbox or passing mock is insufficient.
