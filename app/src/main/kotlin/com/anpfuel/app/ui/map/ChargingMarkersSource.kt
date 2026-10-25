package com.anpfuel.app.ui.map

import com.anpfuel.domain.charging.ChargingPlace

/**
 * Narrow source for charging markers. The product implementation reads the
 * offline EE-18 snapshot; tests inject a fake. Failures return empty and
 * never block fuel pins.
 */
fun interface ChargingMarkersSource {
    suspend fun list(limit: Int): List<ChargingPlace>
}
