package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.attachment.*
import com.anpfuel.application.enterprise.fueling.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.trip.TripCapture
import com.anpfuel.application.enterprise.trip.TripMeterChoice
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.ReferenceMeter
import com.anpfuel.domain.enterprise.fleet.ReferenceMeterKind
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import com.anpfuel.domain.enterprise.fueling.*
import com.anpfuel.domain.enterprise.meter.MeterEntryForm
import com.anpfuel.domain.enterprise.meter.MeterKind
import com.anpfuel.domain.enterprise.meter.ReadingFlag
import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun fid(n: Int) = "7f000000-0000-4000-8000-" + n.toString().padStart(12, '0')

/**
 * EMP-FUEL-011 (B-BR-EMP-455..459, EDR-294..297): offline fuelings through the real queue service and producer,
 * acceptance only from receipts, and the optional receipt as a separate EMP-DOC-004 capture.
 */
class EnterpriseFuelingFlowTest {
    /** The FUEL-002 intake in miniature: a held fueling stays RETRY/UNAVAILABLE, anything else is ACCEPTED. */
    private class Server {
        val delivered = mutableListOf<String>()
        var hold = false
        var unavailable = false
        fun answer(op: EnterpriseOperation): OperationAcknowledgement {
            delivered += op.operationId
            FuelingOperations.read(op)
            val result = if (hold) "RETRY" else "ACCEPTED"
            return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
                """"result":"$result","code":${if (hold) "\"UNAVAILABLE\"" else "null"},"entity_revision":${if (hold) "null" else "1"},""" +
                """"received_at":${if (hold) "null" else "\"2026-11-15T18:00:00Z\""},"review_item_id":null,"warnings":[]}""").toByteArray(), op)
        }
    }

    private class Fixture(receiptsEnabled: Boolean = true, val withEmployee: Boolean = true) {
        val trips = TripFlowFixture()
        val server = Server()
        val a = if (withEmployee) trips.a else EnterpriseContextSummary(trips.a.partition, trips.a.membershipId, "Synthetic A", null)
        val b = trips.b
        val queueService = EnterpriseQueueService(trips.source, trips.queue, { _, operations ->
            if (server.unavailable) throw EnterpriseQueueDeliveryFailure()
            EnterpriseOperationDelivery(operations.map(server::answer))
        }, EnterpriseQueueRegistry(EnterpriseFuelingProducer()), trips.clock, { 0 })
        val store = EnterpriseFuelingStore { attempt ->
            attempt.assertCurrent()
            trips.disk.of(attempt.partition).entries.values.filter { it.operation.type == FuelingOperations.TYPE }
        }
        private var ids = 300
        val service = EnterpriseFuelingService(trips.source, queueService::save, store, { ZoneId.of("America/Cuiaba") }, { trips.wall }, { fid(ids++) })
        class Sealed { var book: AttachmentBook? = null; val bytes = linkedMapOf<String, ByteArray>() }
        val sealed = mutableMapOf<EnterprisePartition, Sealed>()
        var failCapture = false
        private fun of(p: EnterprisePartition) = sealed.getOrPut(p) { Sealed() }
        val attachmentStore = object : EnterpriseAttachmentStore {
            override suspend fun capture(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment, bytes: ByteArray) {
                attempt.assertCurrent()
                if (attachment.ownerOperationId !in trips.disk.of(attempt.partition).entries) throw AttachmentLocalException(AttachmentLocalFailure.UNKNOWN_OPERATION)
                val p = of(attempt.partition)
                val next = (p.book ?: AttachmentBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)).withCapture(attachment)
                if (failCapture) throw EnterprisePersistenceUnavailable()
                p.book = next; p.bytes[attachment.documentId] = bytes.copyOf()
            }
            override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseAttachmentSnapshot {
                val p = of(attempt.partition)
                val entries = trips.disk.of(attempt.partition).entries
                return EnterpriseAttachmentSnapshot(p.book, p.book?.items.orEmpty().mapNotNull { i -> entries[i.ownerOperationId]?.let { i.ownerOperationId to it } }.toMap())
            }
            override suspend fun bytes(attempt: EnterpriseQueueAttempt, documentId: String): ByteArray? = of(attempt.partition).bytes[documentId]?.copyOf()
            override suspend fun transition(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment): Boolean {
                val p = of(attempt.partition); p.book = p.book?.replace(expected, next) ?: return false; return true
            }
        }
        val transport = object : EnterpriseAttachmentTransport {
            override suspend fun open(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment): AttachmentSessionResult = throw EnterpriseQueueDeliveryFailure()
            override suspend fun upload(attempt: EnterpriseQueueAttempt, documentId: String, grant: AttachmentUploadGrant, bytes: ByteArray): AttachmentTransferResult =
                throw EnterpriseQueueDeliveryFailure()
        }
        val attachments = EnterpriseAttachmentService(trips.source, attachmentStore, transport, trips.clock, { Instant.parse("2026-11-15T12:00:00Z") },
            entropy = { 0L }, newId = { fid(ids++) }, enabled = receiptsEnabled)
        val receipts = EnterpriseReceipts(attachments, receiptsEnabled)
        fun flow() = FuelingFlow(trips.source, service, trips.fence, receipts) { account, partition -> queueService.dispatch(account, partition) }
        val meter = ReferenceMeter(fid(60), ReferenceMeterKind.ODOMETER, null, fid(61), Instant.parse("2026-11-01T00:00:00Z"))
        fun choice(org: EnterpriseContextSummary = a) = EnterpriseAssetChoice(org.partition.organizationId, fid(70), 3, AssetKind.VEHICLE,
            UsageBasis.DISTANCE, listOf(meter), Instant.parse("2026-11-15T11:00:00Z"), false)
        fun form(quantity: String = "150", paid: String = "970,00", price: String? = "6,490", reading: TripCapture = km("184600"), discount: String? = null) =
            FuelingForm(FuelProduct.DIESEL_S10, quantity, paid, discount, price, FillLevel.FULL, Payer.COMPANY, PaymentMethod.FUEL_CARD, reading, "Posto sintético")
        fun km(value: String) = TripCapture.Reading(TripMeterChoice(fid(60), fid(61), MeterKind.ODOMETER, null), MeterEntryForm.KM, value)
        fun draft(fill: Byte = 7): ReceiptDraft {
            val bytes = ByteArray(160) { fill }.also { it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[158] = 0xff.toByte(); it[159] = 0xd9.toByte() }
            return ReceiptDraft((ReceiptIntake.inspect(bytes, ReceiptSource.CAMERA) as ReceiptInspection.Accepted).original, LegibilityHint.of(emptySet()))
        }
        fun entries(p: EnterprisePartition = a.partition) = trips.disk.of(p).entries
    }

    private suspend fun opened(x: Fixture, context: EnterpriseContextSummary = x.a): FuelingFlow =
        x.flow().also { it.open(context, x.trips.fence.epoch) }

    @Test fun `a fueling is saved durably in its creation partition with the exact entered values`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        flow.record(x.choice(), x.form(), null, x.trips.fence.epoch, flow.state.value.generation)
        val state = flow.state.value
        val view = state.fuelings.single()
        assertEquals(view.fact.operationId, state.lastSaved)
        assertEquals(view.fact.fuelingId, state.openFuelingId)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.sync)
        assertEquals(FuelingReviewLine.NOT_SENT, view.review)
        assertEquals(listOf(150_000L, 97_000L, 6_490L), listOf(view.fact.quantity.amount, view.fact.amounts.paid, view.fact.amounts.unitPrice))
        assertNull(view.fact.amounts.discount)
        assertEquals(x.a.employeeId, view.fact.responsibleEmployeeId)
        assertEquals(fid(70), view.fact.assetId)
        assertEquals("Posto sintético", view.fact.placeName)
        assertEquals(184_600_000L, (view.fact.evidence as FuelingEvidence.Reading).value.units)
        val operation = x.entries().values.single().operation
        assertEquals(listOf(x.a.partition.ownerAccount, x.a.partition.origin, x.a.partition.organizationId),
            listOf(operation.binding.ownerAccountId, operation.binding.apiOrigin, operation.binding.organizationId))
        assertEquals(x.a.membershipId, operation.membershipId)
        // BR-OFFL-003: organization B neither lists nor holds it.
        val other = opened(x, x.b)
        assertTrue(other.state.value.fuelings.isEmpty())
        assertTrue(x.entries(x.b.partition).isEmpty())
    }

    @Test fun `a failed write, an unknown responsible or an inexact value saves nothing`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        x.trips.failSave = true
        flow.record(x.choice(), x.form(), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.saveFailed)
        assertNull(flow.state.value.lastSaved)
        x.trips.failSave = false
        flow.record(x.choice(), x.form(quantity = "150,0001"), null, x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.captureInvalid)
        flow.record(x.choice(), x.form(reading = TripCapture.Absent("  ")), null, x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.captureInvalid)
        assertTrue(x.entries().isEmpty())
        assertTrue(x.sealed.values.all { it.bytes.isEmpty() })
        val noEmployee = Fixture(withEmployee = false)
        val other = opened(noEmployee)
        other.record(noEmployee.choice(), noEmployee.form(), null, noEmployee.trips.fence.epoch, other.state.value.generation)
        assertTrue(other.state.value.responsibleUnknown)
        assertTrue(noEmployee.entries().isEmpty())
        // Another organization's asset choice is never saved in this partition.
        val wrong = opened(x)
        wrong.record(x.choice(x.b), x.form(), null, x.trips.fence.epoch, wrong.state.value.generation)
        assertEquals(EnterpriseAccessError.STALE, wrong.state.value.error)
        assertTrue(x.entries().isEmpty())
    }

    @Test fun `acceptance follows only the fueling's own receipt`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        flow.record(x.choice(), x.form(), null, x.trips.fence.epoch, flow.state.value.generation)
        x.server.hold = true
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val held = flow.state.value.fuelings.single()
        assertNotEquals(EnterpriseSyncStatus.ACCEPTED, held.sync)
        assertEquals(FuelingReviewLine.NOT_SENT, held.review)
        x.server.hold = false
        x.trips.later(60 * 7)
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val accepted = flow.state.value.fuelings.single()
        assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.sync)
        assertEquals(FuelingReviewLine.AWAITING_APPROVAL, accepted.review)
        val sent = x.server.delivered.size
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(sent, x.server.delivered.size)
    }

    @Test fun `the receipt is a separate photo of the fueling and its failure keeps the fueling`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        val draft = x.draft()
        flow.record(x.choice(), x.form(), draft, x.trips.fence.epoch, flow.state.value.generation)
        val state = flow.state.value
        val view = state.fuelings.single()
        assertEquals(view.fact.operationId, state.lastAttached)
        val photo = state.attachments.getValue(view.fact.operationId).single()
        assertEquals(AttachmentTarget(DocumentTargetKind.FUELING, view.fact.fuelingId), photo.attachment.target)
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, photo.label)
        // The record is accepted while its photo is still unsent.
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, flow.state.value.fuelings.single().sync)
        assertEquals(AttachmentLabel.SENDING, flow.state.value.attachments.getValue(view.fact.operationId).single().label)
        // A failed second write keeps the fueling and says the receipt was not saved.
        x.failCapture = true
        flow.record(x.choice(), x.form(paid = "500"), x.draft(3), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(2, flow.state.value.fuelings.size)
        assertTrue(flow.state.value.receiptNotSaved)
        assertNull(flow.state.value.lastAttached)
        assertNotNull(flow.state.value.lastSaved)
        // A receipt added later from the detail.
        x.failCapture = false
        val second = flow.state.value.fuelings.first { it.fact.amounts.paid == 50_000L }
        flow.attach(second.fact.fuelingId, x.draft(4), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(second.fact.operationId, flow.state.value.lastAttached)
        assertEquals(AttachmentTarget(DocumentTargetKind.FUELING, second.fact.fuelingId),
            flow.state.value.attachments.getValue(second.fact.operationId).single().attachment.target)
    }

    @Test fun `a refused receipt keeps the fueling and an unknown fueling takes no receipt`() = runTest {
        val x = Fixture(receiptsEnabled = false)
        val flow = opened(x)
        flow.record(x.choice(), x.form(), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(1, flow.state.value.fuelings.size)
        assertTrue(flow.state.value.receiptNotSaved)
        assertEquals(AttachmentLocalFailure.DISABLED, flow.state.value.attachmentRefusal)
        val y = Fixture()
        val other = opened(y)
        other.attach(fid(999), y.draft(), y.trips.fence.epoch, other.state.value.generation)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, other.state.value.attachmentRefusal)
        assertTrue(y.sealed.values.all { it.bytes.isEmpty() })
    }

    @Test fun `a restart keeps the fueling, its amounts and its receipt`() = runTest {
        val x = Fixture()
        val first = opened(x)
        first.record(x.choice(), x.form(discount = "5"), x.draft(), x.trips.fence.epoch, first.state.value.generation)
        val before = first.state.value.fuelings.single()
        first.close()
        val restarted = opened(x)
        val after = restarted.state.value.fuelings.single()
        assertEquals(before.fact.fuelingId, after.fact.fuelingId)
        assertEquals(listOf(150_000L, 97_000L, 500L), listOf(after.fact.quantity.amount, after.fact.amounts.paid, after.fact.amounts.discount))
        assertEquals(1, restarted.state.value.attachments.getValue(after.fact.operationId).size)
        assertNull(restarted.state.value.lastSaved)
    }

    @Test fun `warnings inform and never block`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        // 150 L x R$ 6.490 = R$ 973.50 against R$ 970.00 paid: ARITHMETIC_MISMATCH, still saved.
        assertTrue(flow.warnings(x.form()).arithmeticMismatch)
        assertFalse(flow.warnings(x.form(paid = "973,50")).arithmeticMismatch)
        assertFalse(flow.warnings(x.form(paid = "968,50", discount = "5")).arithmeticMismatch)
        assertFalse(flow.warnings(x.form(price = null)).arithmeticMismatch)
        assertTrue(flow.warnings(x.form()).reading.isEmpty())
        flow.record(x.choice(), x.form(), null, x.trips.fence.epoch, flow.state.value.generation)
        x.trips.later(60)
        // A lower reading than this device's previous fueling of the same meter is warned, never refused.
        assertEquals(listOf(ReadingFlag.READING_BELOW_PREVIOUS), flow.warnings(x.form(reading = x.km("184500"))).reading)
        assertTrue(flow.warnings(x.form(reading = TripCapture.Absent("Painel apagado"))).reading.isEmpty())
        flow.record(x.choice(), x.form(reading = x.km("184500")), null, x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(2, flow.state.value.fuelings.size)
    }

    @Test fun `the producer admits only an operation the decoder reads back`() = runTest {
        val x = Fixture()
        val producer = EnterpriseFuelingProducer()
        assertEquals(FuelingOperations.TYPE, producer.descriptor().type)
        val attempt = x.trips.source.attempt(x.trips.source.account(), x.a.partition)
        val tx = object : EnterpriseQueueTransaction {
            override fun record(id: String): EnterpriseLocalRecord? = null
            override fun put(record: EnterpriseLocalRecord) { fail<Unit>("a fueling projects nothing") }
        }
        val foreign = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"${fid(1)}","owner_account_id":"${x.a.partition.ownerAccount}","api_origin":"${x.a.partition.origin}","org_id":"${x.a.partition.organizationId}","membership_id":"${x.a.membershipId}","enrollment_id":"${fid(4)}","type":"fueling.submit","operation_schema":1,"entity":{"type":"fueling","id":"${fid(2)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(),
            SyncBinding.of(x.a.partition.ownerAccount, x.a.partition.origin, x.a.partition.organizationId))
        assertThrows(FuelingOperationException::class.java) { producer.save(tx, attempt, foreign) }
    }
}
