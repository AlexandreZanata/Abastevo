# RVA04: transactional training revocation fence

Status: A/B database checkpoint LOCAL_DONE / INTEGRATION_PENDING. Physical-output race acceptance continues in RVA05; collection remains OFF.

## B-BR / BUC

- B-BR-RVA04-1: consent decisions and derivative admissions serialize per owner in short database transactions. A derivative requires the exact current granted revision, owner/reference, terms and future-only capture scope at commit. A read before minimization is insufficient.
- B-BR-RVA04-2: OFF records a durable monotonic revocation watermark in the same transaction. Selection immediately excludes every derivative at or below it, including stale restored consent rows; bounded batches materialize derivative tombstones. A later grant never restores old derivatives or extends their cleanup deadline.
- B-BR-RVA04-3: no network or image processing occurs while the owner lock is held. Pending physical output and post-write races remain RVA05 obligations before this parent gate closes.
- BUC: sanitizer pauses -> revoke commits -> old proposal refuses. Admission wins first -> revoke excludes it immediately -> durable bounded tombstone recovery -> tracked physical purge.

## Evidence

RED: a revoked image remained ACTIVE and an already-prepared stale derivative inserted after OFF. GREEN: per-owner advisory transaction locks now serialize guarded consent decisions and inventory admissions. Exact revision/reference/terms/capture coverage is checked again in the admission transaction. No connection is held across image processing or storage I/O.

Append-only 000064 records minimal immutable owner/revision revocation receipts, backfills existing OFF decisions and indexes active rows. Selection checks current consent plus the surviving revocation fence. Each OFF materializes at most 32 tombstones immediately; the every-minute reconciler repairs at most 32 more globally. Selection is already denied while repair remains pending. Each derivative uses its first covering revocation receipt, so later OFF/ON cycles neither extend old deadlines nor falsely use an older cycle for new images. Receipt retention/admission remains tracked by RVA12.

Real disposable PostGIS under race validates: 12 admissions vs OFF, revoke while sanitizer is paused, stale commit refusal, 65 rows -> 32 immediate tombstones with zero selectable leftovers -> 32/1/0 restart repair, stale restored grant refusal, fresh grant admits only its own revision, separate immutable deadlines across grant cycles, injected transaction rollback with no partial decision/tombstone, migration-only backfill/replay, and all existing privacy adapter integration tests. Existing consent/derivative migration proofs now freeze their historical target schemas so future dependent migrations do not invalidate the test setup.

Privacy unit/domain/HTTP/job trees, worker/ops consumers, affected vet, sqlc generation/vet, gofmt/diff/secret checks pass on the working tree based on `16ec87c`. Erasure continues to tombstone/purge every inventoried row, including batches not yet materialized. Its counts describe rows covered by the call and are not a globally unique billing/event counter.

No dependency, real-photo fixture, VPS/phone or historical soak change. RVA05 still owes crash-consistent output identity, replay repair, pending/ready state and a paused physical-write vs revocation proof. RVA07 owns bounded purge/deadline health; full parent acceptance is not inferred from this database checkpoint.
