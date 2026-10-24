package com.anpfuel.data.local.dao

import com.anpfuel.data.local.entity.VehicleEntity
import com.anpfuel.domain.premium.AdmissionOutcome
import com.anpfuel.domain.premium.VehicleAdmissionRefused
import com.anpfuel.domain.premium.VehicleAdmissionResult
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class VehicleAdmissionDaoTest {

    private class FakeVehicleDao : VehicleDao {
        val rows = mutableMapOf<String, VehicleEntity>()

        override suspend fun listAll(): List<VehicleEntity> = rows.values.sortedBy { it.sortOrder }

        override suspend fun findById(id: String): VehicleEntity? = rows[id]

        override suspend fun upsert(vehicle: VehicleEntity) {
            rows[vehicle.id] = vehicle
        }

        override suspend fun deleteById(id: String) {
            rows.remove(id)
        }

        override suspend fun count(): Int = rows.size
    }

    private fun vehicle(id: String) = VehicleEntity(
        id = id, displayName = "V $id", tankCapacityLiters = 50.0,
        fuelProduct = "GASOLINE_REGULAR", priceSourceMode = "CHEAPEST_STATION",
        specificStationCnpj = null, priceDropAlertEnabled = false, sortOrder = 0,
    )

    @Test fun `first is free second needs capability fourth exceeds`() = runTest {
        val dao = FakeVehicleDao()
        assertEquals(VehicleAdmissionResult.AdmittedAsNew, dao.admit(vehicle("a"), 1, 3, false))
        val refused = try {
            dao.admit(vehicle("b"), 1, 3, false)
            null
        } catch (e: VehicleAdmissionRefused) {
            e.outcome
        }
        assertEquals(AdmissionOutcome.PREMIUM_REQUIRED, refused)
        assertEquals(1, dao.count())
        assertEquals(VehicleAdmissionResult.AdmittedAsNew, dao.admit(vehicle("b"), 1, 3, true))
        assertEquals(VehicleAdmissionResult.AdmittedAsNew, dao.admit(vehicle("c"), 1, 3, true))
        val ceiling = try {
            dao.admit(vehicle("d"), 1, 3, true)
            null
        } catch (e: VehicleAdmissionRefused) {
            e.outcome
        }
        assertEquals(AdmissionOutcome.CAPACITY_EXCEEDED, ceiling)
        assertEquals(3, dao.count())
    }

    @Test fun `edits replay without consuming slots even over ceiling`() = runTest {
        val dao = FakeVehicleDao()
        dao.upsert(vehicle("a"))
        dao.upsert(vehicle("b"))
        dao.upsert(vehicle("c"))
        dao.upsert(vehicle("legacy-fourth"))
        assertEquals(VehicleAdmissionResult.Replayed, dao.admit(vehicle("b").copy(displayName = "B2"), 1, 3, false))
        assertEquals("B2", dao.findById("b")?.displayName)
        assertEquals(4, dao.count())
    }

    @Test fun `serialized racers cannot both slip past the ceiling`() = runTest {
        val dao = FakeVehicleDao()
        dao.upsert(vehicle("a"))
        dao.upsert(vehicle("b"))
        // Two capability-holding racers serialized: the first lands, the
        // recount inside the transaction refuses the second.
        assertEquals(VehicleAdmissionResult.AdmittedAsNew, dao.admit(vehicle("c"), 1, 3, true))
        val second = try {
            dao.admit(vehicle("d"), 1, 3, true)
            null
        } catch (e: VehicleAdmissionRefused) {
            e.outcome
        }
        assertEquals(AdmissionOutcome.CAPACITY_EXCEEDED, second)
        assertTrue(dao.findById("d") == null)
    }
}
