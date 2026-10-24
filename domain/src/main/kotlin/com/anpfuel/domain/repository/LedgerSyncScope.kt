package com.anpfuel.domain.repository

/**
 * MEX-15 sync scope controls (device + per-dataset participation).
 * Revoking a dataset stops its commands from dispatching and from
 * restore pulls without deleting held work. Device off pauses all
 * sync on this device; the master opt-in stays separate.
 */
interface LedgerSyncScope {

    fun isDatasetEnabled(dataset: String): Boolean

    fun setDatasetEnabled(dataset: String, enabled: Boolean)

    fun isDeviceEnabled(): Boolean

    fun setDeviceEnabled(enabled: Boolean)
}
