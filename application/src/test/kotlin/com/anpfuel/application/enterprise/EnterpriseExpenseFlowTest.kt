package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.attachment.*
import com.anpfuel.application.enterprise.expense.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.expense.*
import com.anpfuel.domain.enterprise.fleet.AssetKind
import com.anpfuel.domain.enterprise.fleet.EnterpriseAssetChoice
import com.anpfuel.domain.enterprise.fleet.UsageBasis
import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun xid(n: Int) = "7e000000-0000-4000-8000-" + n.toString().padStart(12, '0')

/**
 * EMP-EXP-004 (B-BR-EMP-517..521, EDR-336..338): offline direct expenses through the real queue service and producer,
 * exact centavos, acceptance only from receipts, and the optional receipt as a separate EMP-DOC-004 capture.
 */
class EnterpriseExpenseFlowTest {
    /** The EXP-002 intake in miniature: a held expense stays RETRY/UNAVAILABLE, a refused one REJECTED, else ACCEPTED. */
    private class Server {
        val delivered = mutableListOf<String>()
        var hold = false
        var reject = false
        fun answer(op: EnterpriseOperation): OperationAcknowledgement {
            delivered += op.operationId
            ExpenseOperations.read(op)
            val result = if (hold) "RETRY" else if (reject) "REJECTED" else "ACCEPTED"
            val code = if (hold) "\"UNAVAILABLE\"" else if (reject) "\"FORBIDDEN\"" else "null"
            return OperationAcknowledgement.parse(("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}",""" +
                """"result":"$result","code":$code,"entity_revision":${if (result == "ACCEPTED") "1" else "null"},""" +
                """"received_at":${if (hold) "null" else "\"2026-11-15T18:00:00Z\""},"review_item_id":null,"warnings":[]}""").toByteArray(), op)
        }
    }

    private class Fixture(receiptsEnabled: Boolean = true) {
        val trips = TripFlowFixture()
        val server = Server()
        val a = trips.a
        val b = trips.b
        val queueService = EnterpriseQueueService(trips.source, trips.queue, { _, operations ->
            EnterpriseOperationDelivery(operations.map(server::answer))
        }, EnterpriseQueueRegistry(EnterpriseExpenseProducer()), trips.clock, { 0 })
        val store = EnterpriseExpenseStore { attempt ->
            attempt.assertCurrent()
            trips.disk.of(attempt.partition).entries.values.filter { it.operation.type == ExpenseOperations.SUBMIT }
        }
        private var ids = 300
        val service = EnterpriseExpenseService(trips.source, queueService::save, store, { ZoneId.of("America/Cuiaba") }, { trips.wall }, { xid(ids++) })
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
            entropy = { 0L }, newId = { xid(ids++) }, enabled = receiptsEnabled)
        val receipts = EnterpriseReceipts(attachments, receiptsEnabled)
        fun flow() = ExpenseFlow(trips.source, service, trips.fence, receipts) { account, partition -> queueService.dispatch(account, partition) }
        fun choice(org: EnterpriseContextSummary = a) = EnterpriseAssetChoice(org.partition.organizationId, xid(70), 3, AssetKind.VEHICLE,
            UsageBasis.DISTANCE, emptyList(), Instant.parse("2026-11-15T11:00:00Z"), false)
        fun form(paid: String = "12,50", company: String = "12,50", category: String = "TOLL", payer: ExpensePayer = ExpensePayer.EMPLOYEE) =
            ExpenseForm(category, paid, company, payer)
        fun draft(fill: Byte = 7): ReceiptDraft {
            val bytes = ByteArray(160) { fill }.also { it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[158] = 0xff.toByte(); it[159] = 0xd9.toByte() }
            return ReceiptDraft((ReceiptIntake.inspect(bytes, ReceiptSource.CAMERA) as ReceiptInspection.Accepted).original, LegibilityHint.of(emptySet()))
        }
        fun entries(p: EnterprisePartition = a.partition) = trips.disk.of(p).entries
    }

    private suspend fun opened(x: Fixture, context: EnterpriseContextSummary = x.a): ExpenseFlow =
        x.flow().also { it.open(context, x.trips.fence.epoch) }

    @Test fun `an expense is saved durably in its creation partition with the exact entered amounts`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        flow.record(x.choice(), x.form(paid = "98765,43", company = "4321,09", category = "MAINTENANCE", payer = ExpensePayer.COMPANY), null,
            x.trips.fence.epoch, flow.state.value.generation)
        val state = flow.state.value
        val view = state.expenses.single()
        assertEquals(view.fact.operationId, state.lastSaved)
        assertEquals(view.fact.expenseId, state.openExpenseId)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, view.sync)
        assertEquals(ExpenseReviewLine.NOT_SENT, view.review)
        assertEquals(listOf(9_876_543L, 432_109L), listOf(view.fact.paidCentavos, view.fact.companyCentavos))
        assertEquals(listOf("MAINTENANCE", "COMPANY", xid(70)), listOf(view.fact.category, view.fact.payer, view.fact.assetId))
        val operation = x.entries().values.single().operation
        assertEquals(listOf(x.a.partition.ownerAccount, x.a.partition.origin, x.a.partition.organizationId),
            listOf(operation.binding.ownerAccountId, operation.binding.apiOrigin, operation.binding.organizationId))
        assertEquals(x.a.membershipId, operation.membershipId)
        // The exact EXP-002 schema-1 payload: centavos texts, explicit payer, no trip, nothing else.
        assertEquals("""{"asset_id":"${xid(70)}","category":"MAINTENANCE","company_amount":"432109","paid_total":"9876543","payer":"COMPANY","trip_id":null}""",
            String(CanonicalJson.encode(operation.payload, CanonicalJson.MAX_OPERATION_DEPTH)))
        assertTrue(operation.attachments.isEmpty() && operation.softReferences.isEmpty() && operation.dependsOn.isEmpty())
        // BR-OFFL-003: organization B neither lists nor holds it.
        val other = opened(x, x.b)
        assertTrue(other.state.value.expenses.isEmpty())
        assertTrue(x.entries(x.b.partition).isEmpty())
    }

    @Test fun `a failed write or an inexact amount saves nothing and nothing reads as saved`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        x.trips.failSave = true
        flow.record(x.choice(), x.form(), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.saveFailed)
        assertNull(flow.state.value.lastSaved)
        assertTrue(flow.state.value.expenses.isEmpty())
        x.trips.failSave = false
        // Amount conversion errors: a third decimal, a grouping separator, a currency sign, a company share above the
        // paid total, a value beyond the money profile, a fuel cost or an unknown category.
        for (form in listOf(x.form(paid = "12,345"), x.form(paid = "1.234,56", company = "0"), x.form(paid = "R$ 10", company = "0"),
            x.form(paid = "12,50", company = "12,51"), x.form(paid = "100000000000,01", company = "0"), x.form(category = "FUEL"),
            x.form(category = "PEDAGIO"), x.form(company = ""))) {
            flow.record(x.choice(), form, x.draft(), x.trips.fence.epoch, flow.state.value.generation)
            assertTrue(flow.state.value.captureInvalid, form.toString())
            assertNull(flow.state.value.lastSaved)
        }
        assertTrue(x.entries().isEmpty())
        assertTrue(x.sealed.values.all { it.bytes.isEmpty() })
        // Another organization's asset choice is never saved in this partition.
        val wrong = opened(x)
        wrong.record(x.choice(x.b), x.form(), null, x.trips.fence.epoch, wrong.state.value.generation)
        assertEquals(EnterpriseAccessError.STALE, wrong.state.value.error)
        assertTrue(x.entries().isEmpty())
    }

    @Test fun `acceptance follows only the expense's own receipt and no decision is made on the device`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        flow.record(x.choice(), x.form(), null, x.trips.fence.epoch, flow.state.value.generation)
        x.server.hold = true
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val held = flow.state.value.expenses.single()
        assertNotEquals(EnterpriseSyncStatus.ACCEPTED, held.sync)
        assertEquals(ExpenseReviewLine.NOT_SENT, held.review)
        x.server.hold = false
        x.trips.later(60 * 7)
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val accepted = flow.state.value.expenses.single()
        assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.sync)
        assertEquals(ExpenseReviewLine.AWAITING_APPROVAL, accepted.review)
        val sent = x.server.delivered.size
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(sent, x.server.delivered.size)
        // A refused expense says so with its reason; it is never shown approved.
        x.server.reject = true
        flow.record(x.choice(), x.form(paid = "8", company = "8", category = "PARKING"), null, x.trips.fence.epoch, flow.state.value.generation)
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val refused = flow.state.value.expenses.first { it.fact.category == "PARKING" }
        assertEquals(ExpenseReviewLine.REFUSED, refused.review)
        assertEquals("FORBIDDEN", refused.code)
        assertTrue(flow.state.value.expenses.none { it.review == ExpenseReviewLine.AWAITING_APPROVAL && it.sync != EnterpriseSyncStatus.ACCEPTED })
    }

    @Test fun `the receipt is a separate photo of the expense and its failure keeps the expense`() = runTest {
        val x = Fixture()
        val flow = opened(x)
        flow.record(x.choice(), x.form(), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        val state = flow.state.value
        val view = state.expenses.single()
        assertEquals(view.fact.operationId, state.lastAttached)
        val photo = state.attachments.getValue(view.fact.operationId).single()
        assertEquals(AttachmentTarget(DocumentTargetKind.EXPENSE, view.fact.expenseId), photo.attachment.target)
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, photo.label)
        // The record is accepted while its photo is still unsent.
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, flow.state.value.expenses.single().sync)
        assertEquals(AttachmentLabel.SENDING, flow.state.value.attachments.getValue(view.fact.operationId).single().label)
        // A failed second write keeps the expense and says the receipt was not saved.
        x.failCapture = true
        flow.record(x.choice(), x.form(paid = "8", company = "0", category = "PARKING", payer = ExpensePayer.COMPANY), x.draft(3),
            x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(2, flow.state.value.expenses.size)
        assertTrue(flow.state.value.receiptNotSaved)
        assertNull(flow.state.value.lastAttached)
        assertNotNull(flow.state.value.lastSaved)
        // A receipt added later from the detail.
        x.failCapture = false
        val second = flow.state.value.expenses.first { it.fact.paidCentavos == 800L }
        flow.attach(second.fact.expenseId, x.draft(4), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(second.fact.operationId, flow.state.value.lastAttached)
        assertEquals(AttachmentTarget(DocumentTargetKind.EXPENSE, second.fact.expenseId),
            flow.state.value.attachments.getValue(second.fact.operationId).single().attachment.target)
    }

    @Test fun `a refused receipt keeps the expense and a receipt never names another partition's expense`() = runTest {
        val x = Fixture(receiptsEnabled = false)
        val flow = opened(x)
        flow.record(x.choice(), x.form(), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(1, flow.state.value.expenses.size)
        assertTrue(flow.state.value.receiptNotSaved)
        assertEquals(AttachmentLocalFailure.DISABLED, flow.state.value.attachmentRefusal)
        val y = Fixture()
        val inA = opened(y)
        inA.record(y.choice(), y.form(), null, y.trips.fence.epoch, inA.state.value.generation)
        val expenseOfA = inA.state.value.expenses.single().fact.expenseId
        // Cross-partition attachment: organization B's owner does not list A's expense, so it cannot take a receipt.
        val inB = opened(y, y.b)
        inB.attach(expenseOfA, y.draft(), y.trips.fence.epoch, inB.state.value.generation)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, inB.state.value.attachmentRefusal)
        inB.attach(xid(999), y.draft(), y.trips.fence.epoch, inB.state.value.generation)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, inB.state.value.attachmentRefusal)
        // The protected attachment write itself refuses an owner operation that is not queued in its own partition.
        val ownerOfA = y.entries().keys.single()
        val refused = runCatching { y.receipts.save(y.b.partition, ownerOfA, AttachmentTarget(DocumentTargetKind.EXPENSE, expenseOfA), y.draft()) }
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, (refused.exceptionOrNull() as? ReceiptSaveRefused)?.failure)
        assertTrue(y.sealed[y.b.partition]?.bytes.orEmpty().isEmpty())
    }

    @Test fun `a restart keeps the expense, its amounts and its receipt`() = runTest {
        val x = Fixture()
        val first = opened(x)
        first.record(x.choice(), x.form(paid = "45,90", company = "40"), x.draft(), x.trips.fence.epoch, first.state.value.generation)
        val before = first.state.value.expenses.single()
        first.close()
        val restarted = opened(x)
        val after = restarted.state.value.expenses.single()
        assertEquals(before.fact.expenseId, after.fact.expenseId)
        assertEquals(listOf(4_590L, 4_000L), listOf(after.fact.paidCentavos, after.fact.companyCentavos))
        assertEquals(1, restarted.state.value.attachments.getValue(after.fact.operationId).size)
        assertNull(restarted.state.value.lastSaved)
    }

    @Test fun `the producer admits only an operation the decoder reads back`() = runTest {
        val x = Fixture()
        val producer = EnterpriseExpenseProducer()
        assertEquals(ExpenseOperations.SUBMIT, producer.descriptor().type)
        val attempt = x.trips.source.attempt(x.trips.source.account(), x.a.partition)
        val tx = object : EnterpriseQueueTransaction {
            override fun record(id: String): EnterpriseLocalRecord? = null
            override fun put(record: EnterpriseLocalRecord) { fail<Unit>("an expense projects nothing") }
        }
        val foreign = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"${xid(1)}","owner_account_id":"${x.a.partition.ownerAccount}","api_origin":"${x.a.partition.origin}","org_id":"${x.a.partition.organizationId}","membership_id":"${x.a.membershipId}","enrollment_id":"${xid(4)}","type":"expense.submit","operation_schema":1,"entity":{"type":"expense","id":"${xid(2)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(),
            SyncBinding.of(x.a.partition.ownerAccount, x.a.partition.origin, x.a.partition.organizationId))
        assertThrows(ExpenseOperationException::class.java) { producer.save(tx, attempt, foreign) }
    }
}
