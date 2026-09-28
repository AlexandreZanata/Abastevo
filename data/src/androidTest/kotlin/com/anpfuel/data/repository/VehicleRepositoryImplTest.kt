package com.anpfuel.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.dao.VehicleDao
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class VehicleRepositoryImplTest {

    private lateinit var database: AnpFuelDatabase
    private lateinit var vehicleDao: VehicleDao
    private lateinit var repository: VehicleRepositoryImpl

    @Before
    fun setUp() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        database = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java)
            .allowMainThreadQueries()
            .build()
        vehicleDao = database.vehicleDao()
        repository = VehicleRepositoryImpl(vehicleDao)
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun saveAndListVehiclesInSortOrder() = runTest {
        val first = sampleVehicle(
            id = DomainId.from("vehicle-1"),
            displayName = "Gol",
            sortOrder = 1,
        )
        val second = sampleVehicle(
            id = DomainId.from("vehicle-2"),
            displayName = "Onix",
            sortOrder = 0,
        )

        repository.save(first)
        repository.save(second)

        assertEquals(listOf(second, first), repository.listAll())
        assertEquals(2, repository.count())
    }

    @Test
    fun findByIdReturnsSavedVehicle() = runTest {
        val vehicle = sampleVehicle(id = DomainId.from("vehicle-3"))
        repository.save(vehicle)

        assertEquals(vehicle, repository.findById(vehicle.id))
    }

    @Test
    fun deleteRemovesVehicle() = runTest {
        val vehicle = sampleVehicle(id = DomainId.from("vehicle-4"))
        repository.save(vehicle)

        repository.delete(vehicle.id)

        assertNull(repository.findById(vehicle.id))
        assertEquals(0, repository.count())
    }

    private fun sampleVehicle(
        id: DomainId,
        displayName: String = "Civic",
        sortOrder: Int = 0,
    ): Vehicle = Vehicle.create(
        id = id,
        displayName = displayName,
        tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(),
        sortOrder = sortOrder,
    )
}
