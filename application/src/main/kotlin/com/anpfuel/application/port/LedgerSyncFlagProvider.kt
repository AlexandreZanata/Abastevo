package com.anpfuel.application.port

/**
 * MEX-14 explicit opt-in for private ledger sync. Default off: pending
 * work stays durable locally and nothing is uploaded until the owner
 * opts in. Disabling later never deletes held commands.
 */
interface LedgerSyncFlagProvider {
    fun isEnabled(): Boolean
}
