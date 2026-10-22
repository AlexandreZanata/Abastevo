package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.VehicleEntity
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.BatteryCapacity
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.Propulsion
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class VehicleEnergyMapperTest {

    private fun iceEntity() = VehicleEntity(
        id = "11111111-1111-4111-8111-111111111111", displayName = "Gol",
        tankCapacityLiters = 50.0, fuelProduct = "GASOLINE_REGULAR",
        priceSourceMode = "CHEAPEST_STATION", specificStationCnpj = null,
        priceDropAlertEnabled = false, sortOrder = 0, photoPath = null,
        propulsion = "ICE", batteryWh = null, chargeConnector = null,
    )

    @Test
    fun `legacy ICE restores without energy and round-trips`() {
        val restored = VehicleMapper.toDomain(iceEntity())
        assertEquals(Propulsion.ICE, restored.propulsion)
        assertEquals(null, restored.batteryCapacity)
        val back = VehicleMapper.toEntity(restored)
        assertEquals("ICE", back.propulsion)
        assertEquals("GASOLINE_REGULAR", back.fuelProduct)
    }

    @Test
    fun `BEV PHEV HEV round-trip with exact variant rules`() {
        val bev = Vehicle.create(
            displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
            priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.BEV,
            batteryCapacity = BatteryCapacity.ofWh(38000), chargeConnector = ChargingConnector.CCS2,
        )
        val bevBack = VehicleMapper.toDomain(VehicleMapper.toEntity(bev))
        assertEquals(null, bevBack.tankCapacity)
        assertEquals(38000L, bevBack.batteryCapacity?.wattHours)
        assertEquals(ChargingConnector.CCS2, bevBack.chargeConnector)
        val vin = DomainId.from("22222222-2222-4222-8222-222222222222")
        val phev = Vehicle.create(
            displayName = "GWM", tankCapacity = TankCapacity.of(55.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(), propulsion = Propulsion.PHEV,
            batteryCapacity = BatteryCapacity.ofWh(20000), id = vin,
        )
        assertEquals(Propulsion.PHEV, VehicleMapper.toDomain(VehicleMapper.toEntity(phev)).propulsion)
    }

    @Test
    fun `unknown propulsion connector fuel and corrupt battery refuse`() {
        assertThrows(DomainException::class.java) {
            VehicleMapper.toDomain(iceEntity().copy(propulsion = "HYDROGEN"))
        }
        assertThrows(DomainException::class.java) {
            VehicleMapper.toDomain(iceEntity().copy(propulsion = "BEV", batteryWh = 38000, chargeConnector = "WARP"))
        }
        assertThrows(DomainException::class.java) {
            VehicleMapper.toDomain(iceEntity().copy(fuelProduct = "FUTURE_FUEL"))
        }
        assertThrows(DomainException::class.java) {
            VehicleMapper.toDomain(iceEntity().copy(propulsion = "BEV", batteryWh = 0))
        }
        // BEV row with a tank is corrupt: no silent repair.
        assertThrows(DomainException::class.java) {
            VehicleMapper.toDomain(iceEntity().copy(propulsion = "BEV", batteryWh = 38000, tankCapacityLiters = null, fuelProduct = null).copy(tankCapacityLiters = 50.0))
        }
    }
}
