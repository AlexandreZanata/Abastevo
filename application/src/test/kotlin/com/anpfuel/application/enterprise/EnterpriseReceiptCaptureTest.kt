package com.anpfuel.application.enterprise

import com.anpfuel.application.enterprise.attachment.*
import com.anpfuel.application.enterprise.queue.*
import com.anpfuel.application.enterprise.trip.*
import com.anpfuel.domain.enterprise.*
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.document.DocumentTargetKind
import com.anpfuel.domain.enterprise.sync.*
import com.anpfuel.domain.enterprise.trip.*
import java.time.Instant
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

private fun rid(n: Int) = "7d0c0000-0000-4000-8000-" + n.toString().padStart(12, '0')

private fun receiptJpeg(size: Int = 160, fill: Byte = 11): ByteArray = ByteArray(size) { fill }.also {
    it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[size - 2] = 0xff.toByte(); it[size - 1] = 0xd9.toByte()
}
private val PNG = byteArrayOf(0x89.toByte(), 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a) + ByteArray(40)

/** A platform stand-in: references map to bytes; reads and discards are recorded. */
private class FakeOriginals : ReceiptOriginals {
    val files = linkedMapOf<String, ByteArray>()
    val discarded = mutableListOf<String>()
    val reads = mutableListOf<String>()
    var sample: LumaSample? = LumaSample(3, 3, 960, 1280, byteArrayOf(30, (230).toByte(), 30, (230).toByte(), 30, (230).toByte(), 30, (230).toByte(), 30))
    var failRead = false
    var inspectSample: suspend () -> Unit = {}
    private var serial = 0
    override suspend fun cameraTarget(): String = "content://synthetic/camera-${serial++}".also { files[it] = ByteArray(0) }
    override suspend fun read(reference: String): ByteArray? {
        reads += reference
        if (failRead) throw java.io.IOException("synthetic read failure")
        return files[reference]?.copyOf()?.takeIf { it.isNotEmpty() }
    }
    override suspend fun discard(reference: String) { discarded += reference; files.remove(reference) }
    override suspend fun sample(bytes: ByteArray): LumaSample? {
        inspectSample()
        return sample
    }
}

/** EMP-DOC-004 capture sheet state (B-BR-EMP-450..453): nothing is saved here, refusals never become a draft. */
class EnterpriseReceiptCaptureControllerTest {
    @Test
    fun aDeniedCameraSavesNothingAndKeepsImportAvailable() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        controller.cameraDenied(controller.beginSelection())
        assertTrue(controller.state.value.cameraDenied)
        assertNull(controller.state.value.draft)
        assertTrue(originals.reads.isEmpty())
        // Import stays available after a denial and clears the denial message.
        originals.files["content://picker/1"] = receiptJpeg()
        controller.imported(controller.beginSelection(), "content://picker/1")
        assertFalse(controller.state.value.cameraDenied)
        assertEquals(ReceiptSource.IMPORT, controller.state.value.draft?.original?.source)
    }

    @Test
    fun aCameraPhotoIsReadOnceItsPrivateFileDeletedAndTheHintAttached() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val request = controller.beginSelection()
        val target = requireNotNull(controller.cameraTarget(request))
        originals.files[target] = receiptJpeg()
        controller.cameraResult(request, target, taken = true)
        val draft = requireNotNull(controller.state.value.draft)
        assertEquals(ReceiptSource.CAMERA, draft.original.source)
        assertArrayEquals(receiptJpeg(), draft.original.bytes())
        assertEquals(ReceiptLegibility.assess(originals.sample), draft.hint)
        assertEquals(listOf(target), originals.discarded)
        assertNull(controller.state.value.refusal)
    }

    @Test
    fun aCancelledCameraDeletesItsFileAndSavesNothing() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val request = controller.beginSelection()
        val target = requireNotNull(controller.cameraTarget(request))
        controller.cameraResult(request, target, taken = false)
        assertTrue(controller.state.value.cancelled)
        assertNull(controller.state.value.draft)
        assertEquals(listOf(target), originals.discarded)
        assertTrue(originals.reads.isEmpty())
    }

    @Test
    fun missingUnreadableAndUnsupportedFilesNeverBecomeADraft() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val request = controller.beginSelection()
        val target = requireNotNull(controller.cameraTarget(request))
        controller.cameraResult(request, target, taken = true)
        assertEquals(ReceiptRefusal.MISSING, controller.state.value.refusal)
        assertNull(controller.state.value.draft)
        assertEquals(listOf(target), originals.discarded)
        controller.imported(controller.beginSelection(), "content://picker/gone")
        assertEquals(ReceiptRefusal.MISSING, controller.state.value.refusal)
        originals.files["content://picker/png"] = PNG
        controller.imported(controller.beginSelection(), "content://picker/png")
        assertEquals(ReceiptRefusal.UNSUPPORTED, controller.state.value.refusal)
        assertNull(controller.state.value.draft)
        originals.failRead = true
        originals.files["content://picker/denied"] = receiptJpeg()
        controller.imported(controller.beginSelection(), "content://picker/denied")
        assertEquals(ReceiptRefusal.MISSING, controller.state.value.refusal)
        // The picker's references are never deleted by the app.
        assertEquals(listOf(target), originals.discarded)
    }

    @Test
    fun aRefusedNewSelectionKeepsThePreviousPhotoAndANullPickIsACancel() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        originals.files["content://picker/a"] = receiptJpeg(fill = 3)
        controller.imported(controller.beginSelection(), "content://picker/a")
        val first = requireNotNull(controller.state.value.draft)
        originals.files["content://picker/png"] = PNG
        controller.imported(controller.beginSelection(), "content://picker/png")
        assertSame(first, controller.state.value.draft)
        assertEquals(ReceiptRefusal.UNSUPPORTED, controller.state.value.refusal)
        controller.imported(controller.beginSelection(), null)
        assertTrue(controller.state.value.cancelled)
        assertSame(first, controller.state.value.draft)
        controller.reset()
        assertEquals(ReceiptCaptureState(), controller.state.value)
    }

    @Test
    fun anUndecodableJpegIsNotCheckedButStillAPhoto() = runTest {
        val originals = FakeOriginals().apply { sample = null }
        val controller = ReceiptCaptureController(originals)
        originals.files["content://picker/raw"] = receiptJpeg()
        controller.imported(controller.beginSelection(), "content://picker/raw")
        assertEquals(LegibilityHint.NOT_CHECKED, controller.state.value.draft?.hint)
    }

    @Test
    fun aLateCameraResultFromAnAbandonedCaptureOnlyDeletesItsTemporaryFile() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val request = controller.beginSelection()
        val target = requireNotNull(controller.cameraTarget(request))
        originals.files[target] = receiptJpeg()
        controller.reset()

        controller.cameraResult(request, target, taken = true)

        assertEquals(ReceiptCaptureState(), controller.state.value)
        assertEquals(listOf(target), originals.discarded)
        assertTrue(originals.reads.isEmpty())
    }

    @Test
    fun anOldPickerCallbackCannotReadItsFileAfterAContextChange() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val old = controller.beginSelection()
        originals.files["content://picker/late"] = receiptJpeg()
        controller.reset()
        val current = controller.beginSelection()

        controller.cameraDenied(old)
        controller.imported(old, "content://picker/late")
        controller.imported(current, null)

        assertTrue(controller.state.value.cancelled)
        assertFalse(controller.state.value.cameraDenied)
        assertNull(controller.state.value.draft)
        assertTrue(originals.reads.isEmpty())
        assertTrue(originals.discarded.isEmpty())
    }

    @Test
    fun aLegibilityCheckFinishingAfterResetCannotReplaceTheNewSessionsPhoto() = runTest {
        val originals = FakeOriginals()
        val controller = ReceiptCaptureController(originals)
        val firstStarted = CompletableDeferred<Unit>()
        val releaseFirst = CompletableDeferred<Unit>()
        originals.inspectSample = {
            firstStarted.complete(Unit)
            releaseFirst.await()
        }
        originals.files["content://picker/old"] = receiptJpeg(fill = 1)
        originals.files["content://picker/new"] = receiptJpeg(fill = 2)
        val first = launch { controller.imported(controller.beginSelection(), "content://picker/old") }
        firstStarted.await()
        controller.reset()
        originals.inspectSample = {}
        controller.imported(controller.beginSelection(), "content://picker/new")
        releaseFirst.complete(Unit)
        first.join()

        assertArrayEquals(receiptJpeg(fill = 2), controller.state.value.draft?.original?.bytes())
        assertTrue(originals.discarded.isEmpty())
    }
}

/** Trips with EMP-DOC-004 photos of their readings over the EMP-OFFL-005 queue (B-BR-EMP-453/454, EDR-293). */
class EnterpriseTripReceiptTest {
    private class Fixture(val enabled: Boolean = true) {
        val trips = TripFlowFixture()
        class Sealed { var book: AttachmentBook? = null; val bytes = linkedMapOf<String, ByteArray>() }
        val sealed = mutableMapOf<EnterprisePartition, Sealed>()
        var failCapture = false
        var failRead = false
        private fun of(p: EnterprisePartition) = sealed.getOrPut(p) { Sealed() }
        val store = object : EnterpriseAttachmentStore {
            override suspend fun capture(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment, bytes: ByteArray) {
                attempt.assertCurrent()
                // The owner must be queued in the same partition (EMP-OFFL-005).
                if (attachment.ownerOperationId !in trips.disk.of(attempt.partition).entries)
                    throw AttachmentLocalException(AttachmentLocalFailure.UNKNOWN_OPERATION)
                val p = of(attempt.partition)
                val next = (p.book ?: AttachmentBook.empty(attempt.enrollment.target.membershipId, attempt.enrollment.enrollmentId)).withCapture(attachment)
                if (failCapture) throw EnterprisePersistenceUnavailable()
                p.book = next
                p.bytes[attachment.documentId] = bytes.copyOf()
            }
            override suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseAttachmentSnapshot {
                attempt.assertCurrent()
                if (failRead) throw EnterprisePersistenceUnavailable()
                val p = of(attempt.partition)
                val entries = trips.disk.of(attempt.partition).entries
                return EnterpriseAttachmentSnapshot(p.book, p.book?.items.orEmpty().mapNotNull { item -> entries[item.ownerOperationId]?.let { item.ownerOperationId to it } }.toMap())
            }
            override suspend fun bytes(attempt: EnterpriseQueueAttempt, documentId: String): ByteArray? = of(attempt.partition).bytes[documentId]?.copyOf()
            override suspend fun transition(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment): Boolean {
                val p = of(attempt.partition)
                p.book = p.book?.replace(expected, next) ?: return false
                if (next.state == AttachmentState.STORED) p.bytes.remove(next.documentId)
                return true
            }
        }
        val transport = object : EnterpriseAttachmentTransport {
            override suspend fun open(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment): AttachmentSessionResult =
                throw EnterpriseQueueDeliveryFailure()
            override suspend fun upload(attempt: EnterpriseQueueAttempt, documentId: String, grant: AttachmentUploadGrant, bytes: ByteArray): AttachmentTransferResult =
                throw EnterpriseQueueDeliveryFailure()
        }
        private var ids = 500
        val attachments = EnterpriseAttachmentService(trips.source, store, transport, trips.clock, { Instant.parse("2026-11-15T12:00:00Z") },
            entropy = { 0L }, newId = { rid(ids++) }, enabled = enabled)
        val receipts = EnterpriseReceipts(attachments, enabled)
        fun flow() = TripFlow(trips.source, trips.service, trips.fence, receipts) { account, partition -> trips.queueService.dispatch(account, partition) }
        fun draft(bytes: ByteArray = receiptJpeg()): ReceiptDraft =
            ReceiptDraft((ReceiptIntake.inspect(bytes, ReceiptSource.CAMERA) as ReceiptInspection.Accepted).original, LegibilityHint.of(emptySet()))
        fun sealedBytes(p: EnterprisePartition) = of(p).bytes
        suspend fun startTrip(flow: TripFlow): TripView {
            flow.open(trips.a, trips.fence.epoch)
            flow.start(trips.choice(), trips.label, trips.km("184320"), trips.fence.epoch, flow.state.value.generation)
            return flow.state.value.trips.single()
        }
    }

    private fun readingOf(view: TripView) = ((view.start.fact as TripOperationFact.Start).evidence as TripEvidence.Reading).readingId

    @Test
    fun aReadingPhotoIsSavedOnlyByTheProtectedWriteAndShownUnderItsRecord() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        val owner = trip.start.fact.operationId
        assertTrue(flow.state.value.attachments.isEmpty())
        val draft = x.draft()
        flow.attach(owner, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), draft, x.trips.fence.epoch, flow.state.value.generation)
        val state = flow.state.value
        assertEquals(owner, state.lastAttached)
        assertFalse(state.saveFailed)
        val photo = state.attachments.getValue(owner).single()
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, photo.label)
        assertEquals(AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), photo.attachment.target)
        assertArrayEquals(draft.original.bytes(), x.sealedBytes(x.trips.a.partition).getValue(photo.attachment.documentId))
        // The record keeps its own state: a photo neither sends nor accepts it.
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, state.trips.single().start.sync)
    }

    @Test
    fun aFailedWriteSavesNothingAndSaysSo() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        x.failCapture = true
        flow.attach(trip.start.fact.operationId, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), x.draft(),
            x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.saveFailed)
        assertNull(flow.state.value.lastAttached)
        assertTrue(flow.state.value.attachments.isEmpty())
        assertTrue(x.sealedBytes(x.trips.a.partition).isEmpty())
        assertEquals(1, flow.state.value.trips.size)
    }

    @Test
    fun localRefusalsCarryTheirReasonAndSaveNothing() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        flow.attach(rid(999), AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, flow.state.value.attachmentRefusal)
        assertTrue(x.sealedBytes(x.trips.a.partition).isEmpty())

        val off = Fixture(enabled = false)
        val disabled = off.flow()
        val other = off.startTrip(disabled)
        disabled.attach(other.start.fact.operationId, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(other)), off.draft(),
            off.trips.fence.epoch, disabled.state.value.generation)
        assertEquals(AttachmentLocalFailure.DISABLED, disabled.state.value.attachmentRefusal)
        assertTrue(off.sealedBytes(off.trips.a.partition).isEmpty())
    }

    @Test
    fun anotherOrganizationNeitherSeesNorOwnsThePhoto() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        val owner = trip.start.fact.operationId
        val target = AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip))
        flow.attach(owner, target, x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        val other = x.flow()
        other.open(x.trips.b, x.trips.fence.epoch)
        assertTrue(other.state.value.attachments.isEmpty())
        // BR-OFFL-003: A's record cannot own a photo in B's partition.
        other.attach(owner, target, x.draft(receiptJpeg(fill = 2)), x.trips.fence.epoch, other.state.value.generation)
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, other.state.value.attachmentRefusal)
        assertTrue(x.sealedBytes(x.trips.b.partition).isEmpty())
        assertEquals(1, x.sealedBytes(x.trips.a.partition).size)
    }

    @Test
    fun aRestartKeepsTheSealedPhotoWithItsOwnState() = runTest {
        val x = Fixture()
        val first = x.flow()
        val trip = x.startTrip(first)
        val owner = trip.start.fact.operationId
        first.attach(owner, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), x.draft(), x.trips.fence.epoch, first.state.value.generation)
        first.close()
        val restarted = x.flow()
        restarted.open(x.trips.a, x.trips.fence.epoch)
        assertEquals(AttachmentLabel.SAVED_ON_DEVICE, restarted.state.value.attachments.getValue(owner).single().label)
        assertNull(restarted.state.value.lastAttached)
    }

    @Test
    fun theRecordIsAcceptedWhileItsPhotoIsStillUnverifiedAndAMissingFileIsShown() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        val owner = trip.start.fact.operationId
        flow.attach(owner, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), x.draft(), x.trips.fence.epoch, flow.state.value.generation)
        flow.sendNow(x.trips.fence.epoch, flow.state.value.generation)
        val accepted = flow.state.value
        assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.trips.single().start.sync)
        assertEquals(AttachmentLabel.SENDING, accepted.attachments.getValue(owner).single().label)
        // The sealed original disappears: delivery marks it missing, never sends or verifies it (UC-OFFL-03).
        x.sealedBytes(x.trips.a.partition).clear()
        x.attachments.deliver(x.trips.source.account(), x.trips.a.partition)
        flow.reload(x.trips.fence.epoch, flow.state.value.generation)
        assertEquals(AttachmentLabel.FILE_MISSING, flow.state.value.attachments.getValue(owner).single().label)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, flow.state.value.trips.single().start.sync)
    }

    @Test
    fun anUnreadableAttachmentIndexKeepsTheTripsVisible() = runTest {
        val x = Fixture()
        val flow = x.flow()
        x.startTrip(flow)
        x.failRead = true
        flow.reload(x.trips.fence.epoch, flow.state.value.generation)
        assertTrue(flow.state.value.attachmentsUnavailable)
        assertTrue(flow.state.value.attachments.isEmpty())
        assertEquals(1, flow.state.value.trips.size)
        assertNull(flow.state.value.error)
    }

    @Test
    fun aStaleGestureSavesNothing() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        val generation = flow.state.value.generation
        flow.attach(trip.start.fact.operationId, AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip)), x.draft(),
            x.trips.fence.epoch, generation - 1)
        assertTrue(x.sealedBytes(x.trips.a.partition).isEmpty())
        assertNull(flow.state.value.lastAttached)
    }

    @Test
    fun receiptsGroupByOwnerAndMapLocalRefusals() = runTest {
        val x = Fixture()
        val flow = x.flow()
        val trip = x.startTrip(flow)
        val owner = trip.start.fact.operationId
        val target = AttachmentTarget(DocumentTargetKind.METER_READING, readingOf(trip))
        x.receipts.save(x.trips.a.partition, owner, target, x.draft(receiptJpeg(fill = 1)))
        x.receipts.save(x.trips.a.partition, owner, target, x.draft(receiptJpeg(fill = 2)))
        assertEquals(listOf(owner), x.receipts.byOwner(x.trips.a.partition).keys.toList())
        assertEquals(2, x.receipts.byOwner(x.trips.a.partition).getValue(owner).size)
        val refused = runCatching { x.receipts.save(x.trips.a.partition, rid(998), target, x.draft()) }.exceptionOrNull()
        assertEquals(AttachmentLocalFailure.UNKNOWN_OPERATION, (refused as ReceiptSaveRefused).failure)
    }
}
