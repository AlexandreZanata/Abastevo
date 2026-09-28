package com.anpfuel.domain.rule

import com.anpfuel.domain.model.UserPreferences

/**
 * BR-013 — Station detail retention uses configured rolling window; summary rows are unaffected.
 */
object StationDetailRetentionRule {

    fun retentionWeeks(preferences: UserPreferences): Int =
        preferences.stationDetailRetentionWeeks.coerceAtLeast(1)
}
