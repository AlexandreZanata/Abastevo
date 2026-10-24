package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class VehicleEnergyTest {

    private fun ice() = Vehicle.create(
        displayName = "Gol",
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(),
    )

    @Test
    fun `legacy ICE needs tank and fuel and refuses energy fields`() {
        val kept = ice()
        assertEquals(Propulsion.ICE, kept.propulsion)
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                priceSource = VehiclePriceSource.cheapest(),
                batteryCapacity = BatteryCapacity.ofWh(10000),
            )
        }
    }

    @Test
    fun `BEV carries battery without tank or fuel`() {
        val bev = Vehicle.create(
            displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
            priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.BEV,
            batteryCapacity = BatteryCapacity.ofWh(38000),
            chargeConnector = com.anpfuel.domain.charging.ChargingConnector.CCS2,
        )
        assertEquals(38000L, bev.batteryCapacity?.wattHours)
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = "Dolphin", tankCapacity = TankCapacity.of(50.0), fuelProduct = null,
                priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.BEV,
                batteryCapacity = BatteryCapacity.ofWh(38000),
            )
        }
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
                priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.BEV,
            )
        }
    }

    @Test
    fun `PHEV carries both HEV and MHEV carry tank only`() {
        val phev = Vehicle.create(
            displayName = "GWM", tankCapacity = TankCapacity.of(55.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.PHEV,
            batteryCapacity = BatteryCapacity.ofWh(20000),
        )
        assertEquals(20000L, phev.batteryCapacity?.wattHours)
        for (propulsion in listOf(Propulsion.HEV, Propulsion.MHEV)) {
            val hybrid = Vehicle.create(
                displayName = "Hrv", tankCapacity = TankCapacity.of(40.0),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                priceSource = VehiclePriceSource.cheapest(), propulsion = propulsion,
            )
            assertEquals(null, hybrid.batteryCapacity)
            assertThrows(DomainException::class.java) {
                Vehicle.create(
                    displayName = "Hrv", tankCapacity = TankCapacity.of(40.0),
                    fuelProduct = FuelProduct.GASOLINE_REGULAR,
                    priceSource = VehiclePriceSource.cheapest(), propulsion = propulsion,
                    batteryCapacity = BatteryCapacity.ofWh(5000),
                )
            }
        }
        assertThrows(DomainException::class.java) {
            Vehicle.create(
                displayName = "X", tankCapacity = TankCapacity.of(40.0),
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.PHEV,
            )
        }
    }

    @Test
    fun `variant edits apply atomically without impossible intermediates`() {
        val ice = ice()
        // Chained single-field copies would strand an ICE with a null
        // tank; the atomic transition lands directly on BEV.
        val bev = ice.withEnergyVariant(
            propulsion = Propulsion.BEV, tankCapacity = null, fuelProduct = null,
            batteryCapacity = BatteryCapacity.ofWh(38000),
            chargeConnector = com.anpfuel.domain.charging.ChargingConnector.TYPE2,
        )
        assertEquals(Propulsion.BEV, bev.propulsion)
        assertEquals(ice.id, bev.id)
        assertThrows(DomainException::class.java) {
            bev.withTankCapacity(TankCapacity.of(10.0))
        }
    }

    @Test
    fun `legacy restore preserves identity and merges energy explicitly`() {        val legacy = Vehicle.restore(
            id = DomainId.generate(), displayName = "Gol",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            priceDropAlertEnabled = false, sortOrder = 0,
            propulsion = Propulsion.ICE, batteryCapacity = null, chargeConnector = null,
        )
        val edited = legacy.withDisplayName("Gol G2").withBatteryCapacity(null)
        assertEquals("Gol G2", edited.displayName)
        assertEquals(legacy.id, edited.id)
        assertEquals(null, edited.batteryCapacity)
    }
}
