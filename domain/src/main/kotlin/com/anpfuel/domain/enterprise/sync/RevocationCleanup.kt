package com.anpfuel.domain.enterprise.sync

/**
 * EMP-OFFL-011 (B-BR-EMP-536..539, EDR-348..350): what a device does with one partition after the exact
 * signed contact named its revocation class, or after its offline validity expired (BR-PRIV-018).
 */
enum class EnterpriseRevocationClass { LOST_OR_COMPROMISED, ACCESS_ENDED, OFFLINE_EXPIRED }

/** Counts read from one partition's sealed queue under its vault lock; never a payload or an identifier. */
class RevocationQueueFacts(val unsent: Int, val windowClosed: Boolean) {
    init { require(unsent >= 0) }
    override fun toString(): String = "RevocationQueueFacts(unsent=$unsent,windowClosed=$windowClosed)"
}

enum class RevocationCleanupAction { DELIVER_INTAKE_ONLY, ERASE }

/** What the device may say about a revoked partition; CLEANED only after verified disposal. */
enum class RevocationCleanupState { INTAKE_ONLY, CLEANUP_PENDING, CLEANED }

object RevocationCleanup {
    /** Late-intake acknowledgement codes (EMP-OFFL-009) that ask for one exact contact; never a verdict. */
    val HINT_CODES: Set<String> = setOf("ACCESS_ENDED_BEFORE_RECEIPT", "MEMBERSHIP_ENDED", "INTAKE_WINDOW_CLOSED", "DEVICE_REVOKED")

    /** True when this final or retry acknowledgement is a late-intake answer (EDR-348). */
    fun hint(acknowledgement: OperationAcknowledgement): Boolean = acknowledgement.code in HINT_CODES

    /** A whole-response status of the signed delivery plane that asks for one exact contact (EDR-348). */
    fun hint(httpStatus: Int): Boolean = httpStatus == 403 || httpStatus == 404

    /** Facts of one partition's queue: unsent entries and whether the server closed its intake window. */
    fun facts(entries: List<EnterpriseQueueEntry>): RevocationQueueFacts = RevocationQueueFacts(
        entries.count { it.acknowledgement == null },
        entries.any { entry -> entry.acknowledgement?.let { it.result == SyncResult.REJECTED && it.code == WINDOW_CLOSED } == true },
    )

    /** ACCESS_ENDED delivers until nothing is unsent or the window closed; the other classes erase at once. */
    fun decide(revocation: EnterpriseRevocationClass, facts: RevocationQueueFacts?): RevocationCleanupAction = when (revocation) {
        EnterpriseRevocationClass.LOST_OR_COMPROMISED, EnterpriseRevocationClass.OFFLINE_EXPIRED -> RevocationCleanupAction.ERASE
        EnterpriseRevocationClass.ACCESS_ENDED -> {
            val known = requireNotNull(facts) { "An ended partition is never erased without its own queue facts" }
            // The window only closes (EDR-197): after one INTAKE_WINDOW_CLOSED the rest would be refused too.
            if (known.unsent == 0 || known.windowClosed) RevocationCleanupAction.ERASE else RevocationCleanupAction.DELIVER_INTAKE_ONLY
        }
    }

    /** The only states the device may report; [erasureVerified] is the erasure's own verified result. */
    fun state(action: RevocationCleanupAction, erasureVerified: Boolean): RevocationCleanupState = when (action) {
        RevocationCleanupAction.DELIVER_INTAKE_ONLY -> {
            require(!erasureVerified) { "A partition still delivering cannot be erased" }
            RevocationCleanupState.INTAKE_ONLY
        }
        RevocationCleanupAction.ERASE -> if (erasureVerified) RevocationCleanupState.CLEANED else RevocationCleanupState.CLEANUP_PENDING
    }

    private const val WINDOW_CLOSED = "INTAKE_WINDOW_CLOSED"
}
