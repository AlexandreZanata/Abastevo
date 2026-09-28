package com.anpfuel.app.mapper

import java.util.Locale
import kotlin.math.roundToInt

/**
 * UC-015 — Locale-aware distance label for the nearest station recommendation.
 */
object DistanceFormatter {

    private const val METERS_PER_KILOMETER = 1_000.0
    private const val KILOMETERS_PRECISION = "%.1f km"

    fun format(distanceMeters: Double, locale: Locale): String =
        if (distanceMeters < METERS_PER_KILOMETER) {
            String.format(locale, "%d m", distanceMeters.roundToInt())
        } else {
            String.format(locale, KILOMETERS_PRECISION, distanceMeters / METERS_PER_KILOMETER)
        }
}
