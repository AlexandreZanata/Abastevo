package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-OFFL-011 frozen rule table (B-BR-EMP-536..539, EDR-348..350). Synthetic identifiers only. */
class RevocationCleanupTest {
    private fun id(n: Int) = "7f000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun op(n: Int) = EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${id(n)}","owner_account_id":"${id(1)}","api_origin":"https://cleanup.example.invalid","org_id":"${id(2)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","type":"validation.cleanup","operation_schema":1,"entity":{"type":"record","id":"${id(8)}","expected_revision":null},"occurred_at":"2026-11-14T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"synthetic"}}""".toByteArray(), SyncBinding.of(id(1), "https://cleanup.example.invalid", id(2)))
    private fun ack(operation: EnterpriseOperation, result: String, code: String?): OperationAcknowledgement {
        val final = result != "RETRY"
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(6)}\"" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":${if (final) "\"2026-11-14T00:01:00Z\"" else "null"},"review_item_id":$review,"warnings":[]}""".toByteArray(), operation)
    }
    private fun entry(n: Int, result: String? = null, code: String? = null): EnterpriseQueueEntry {
        val operation = op(n)
        return EnterpriseQueueEntry(operation, acknowledgement = result?.let { ack(operation, it, code) })
    }

    @Test fun lostAndExpiredClassesEraseAtOnceWhateverIsUnsent() {
        for (revocation in listOf(EnterpriseRevocationClass.LOST_OR_COMPROMISED, EnterpriseRevocationClass.OFFLINE_EXPIRED)) {
            assertEquals(RevocationCleanupAction.ERASE, RevocationCleanup.decide(revocation, null))
            for (unsent in listOf(0, 1, 50)) for (closed in listOf(false, true)) {
                assertEquals(RevocationCleanupAction.ERASE, RevocationCleanup.decide(revocation, RevocationQueueFacts(unsent, closed)),
                    "$revocation must never keep sending ($unsent unsent, window closed $closed)")
            }
        }
    }

    @Test fun eligibleIntakeIsNeverDiscardedSilentlyWhileTheWindowIsOpen() {
        for (unsent in listOf(1, 2, 10_000)) {
            assertEquals(RevocationCleanupAction.DELIVER_INTAKE_ONLY,
                RevocationCleanup.decide(EnterpriseRevocationClass.ACCESS_ENDED, RevocationQueueFacts(unsent, false)))
        }
        // Without the sealed queue's own facts an ended partition is never erased blindly.
        assertThrows(IllegalArgumentException::class.java) { RevocationCleanup.decide(EnterpriseRevocationClass.ACCESS_ENDED, null) }
        assertThrows(IllegalArgumentException::class.java) { RevocationQueueFacts(-1, false) }
    }

    @Test fun accessEndedErasesOnlyWhenEverythingIsFinalOrTheWindowClosed() {
        assertEquals(RevocationCleanupAction.ERASE, RevocationCleanup.decide(EnterpriseRevocationClass.ACCESS_ENDED, RevocationQueueFacts(0, false)))
        assertEquals(RevocationCleanupAction.ERASE, RevocationCleanup.decide(EnterpriseRevocationClass.ACCESS_ENDED, RevocationQueueFacts(0, true)))
        assertEquals(RevocationCleanupAction.ERASE, RevocationCleanup.decide(EnterpriseRevocationClass.ACCESS_ENDED, RevocationQueueFacts(7, true)))
    }

    @Test fun factsCountUnsentEntriesAndOnlyAFinalWindowRefusalClosesTheWindow() {
        val open = listOf(entry(10), entry(11, "ACCEPTED_FOR_REVIEW", "ACCESS_ENDED_BEFORE_RECEIPT"), entry(12, "REJECTED", "MEMBERSHIP_ENDED"),
            entry(13, "REJECTED", "DEVICE_REVOKED"), entry(14, "ACCEPTED"), entry(15))
        val facts = RevocationCleanup.facts(open)
        assertEquals(2, facts.unsent); assertFalse(facts.windowClosed)
        val closed = RevocationCleanup.facts(open + entry(16, "REJECTED", "INTAKE_WINDOW_CLOSED"))
        assertEquals(2, closed.unsent); assertTrue(closed.windowClosed)
        val empty = RevocationCleanup.facts(emptyList())
        assertEquals(0, empty.unsent); assertFalse(empty.windowClosed)
    }

    @Test fun hintsAreLateIntakeCodesAndWholeResponseRefusalsNeverAVerdict() {
        val operation = op(20)
        for (code in RevocationCleanup.HINT_CODES) {
            val result = if (code == "ACCESS_ENDED_BEFORE_RECEIPT") "ACCEPTED_FOR_REVIEW" else "REJECTED"
            assertTrue(RevocationCleanup.hint(ack(operation, result, code)), code)
        }
        for ((result, code) in listOf("ACCEPTED" to null, "ACCEPTED_FOR_REVIEW" to "STALE_REVISION", "REJECTED" to "FORBIDDEN",
            "REJECTED" to "MALFORMED", "REJECTED" to "ORG_SUSPENDED", "RETRY" to "UNAVAILABLE", "RETRY" to "DEPENDENCY_PENDING")) {
            assertFalse(RevocationCleanup.hint(ack(operation, result, code)), "$result $code")
        }
        assertTrue(RevocationCleanup.hint(403)); assertTrue(RevocationCleanup.hint(404))
        for (status in listOf(200, 400, 401, 409, 413, 429, 500, 503)) assertFalse(RevocationCleanup.hint(status), status.toString())
    }

    @Test fun offlineCleanupOverclaimRemovedIsSaidOnlyAfterVerifiedDisposal() {
        assertEquals(RevocationCleanupState.INTAKE_ONLY, RevocationCleanup.state(RevocationCleanupAction.DELIVER_INTAKE_ONLY, false))
        assertEquals(RevocationCleanupState.CLEANUP_PENDING, RevocationCleanup.state(RevocationCleanupAction.ERASE, false))
        assertEquals(RevocationCleanupState.CLEANED, RevocationCleanup.state(RevocationCleanupAction.ERASE, true))
        // A partition still delivering can never be reported as erased.
        assertThrows(IllegalArgumentException::class.java) { RevocationCleanup.state(RevocationCleanupAction.DELIVER_INTAKE_ONLY, true) }
        for (revocation in EnterpriseRevocationClass.entries) for (unsent in listOf(0, 3)) for (closed in listOf(false, true)) {
            val action = RevocationCleanup.decide(revocation, RevocationQueueFacts(unsent, closed))
            assertNotEquals(RevocationCleanupState.CLEANED, RevocationCleanup.state(action, false), "$revocation claimed removal before verification")
        }
    }
}
