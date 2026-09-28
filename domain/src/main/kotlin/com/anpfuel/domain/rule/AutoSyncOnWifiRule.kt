package com.anpfuel.domain.rule

import com.anpfuel.domain.model.UserPreferences

/**
 * BR-014 — Background sync respects Wi-Fi-only preference via WorkManager constraints.
 */
object AutoSyncOnWifiRule {

    fun requiresUnmeteredNetwork(preferences: UserPreferences): Boolean =
        preferences.autoSyncOnWifi
}
