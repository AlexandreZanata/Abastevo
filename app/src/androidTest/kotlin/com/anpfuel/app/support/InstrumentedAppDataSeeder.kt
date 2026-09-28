package com.anpfuel.app.support

import android.content.Context
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.work.WorkManager
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.AnpFuelDatabaseMigrations
import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.SurveyWeekEntity
import com.anpfuel.data.mapper.VehicleMapper
import com.anpfuel.data.local.preferences.SyncStateDataStore
import com.anpfuel.data.local.preferences.UserPreferencesDataStore
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import java.io.File
import kotlinx.coroutines.runBlocking

/**
 * Seeds on-device preferences and Room data for instrumented app flows.
 *
 * Always closes the database instance so Hilt can open the production singleton safely.
 */
object InstrumentedAppDataSeeder {

    private const val DATABASE_NAME = "anp_fuel.db"

    suspend fun seedReturningUserHomeState(context: Context) {
        clearAppStorage(context)
        seedPostSyncState(
            context = context,
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
        )
    }

    suspend fun seedReturningUserHomeStateWithVehicle(context: Context) {
        seedReturningUserHomeState(context)
        seedSampleVehicle(context)
    }

    suspend fun seedAppendixA2PostSyncState(context: Context) {
        clearAppStorage(context)
        seedPostSyncState(
            context = context,
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13"),
        )
    }

    private suspend fun seedPostSyncState(
        context: Context,
        municipality: String,
        state: BrazilianState,
        surveyWeek: SurveyWeek,
    ) {
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value

        UserPreferencesDataStore(context).write(
            UserPreferences(
                onboardingCompleted = true,
                activeSurveyWeek = surveyWeek,
                preferredMunicipality = municipality,
                preferredState = state,
            ),
        )

        val database = Room.databaseBuilder(context, AnpFuelDatabase::class.java, DATABASE_NAME)
            .setJournalMode(RoomDatabase.JournalMode.WRITE_AHEAD_LOGGING)
            .addMigrations(
                AnpFuelDatabaseMigrations.MIGRATION_1_2,
                AnpFuelDatabaseMigrations.MIGRATION_2_3,
                AnpFuelDatabaseMigrations.MIGRATION_3_4,
            )
            .allowMainThreadQueries()
            .build()

        try {
            database.surveyWeekDao().insert(
                SurveyWeekEntity(
                    id = surveyWeekId,
                    startDate = surveyWeek.startDate.toString(),
                    endDate = surveyWeek.endDate.toString(),
                    summaryImportedAt = 1_718_284_800_000L,
                    stationImportedAt = null,
                ),
            )
            database.averagePriceDao().insertAll(
                listOf(
                    AveragePriceEntity(
                        id = "avg-$municipality-ethanol",
                        surveyWeekId = surveyWeekId,
                        state = state.abbreviation,
                        municipality = municipality.uppercase(),
                        fuelProduct = "ETHANOL",
                        stationCount = 42,
                        unit = "R$/l",
                        avgPrice = 3.42,
                        minPrice = 3.10,
                        maxPrice = 3.80,
                        stdDev = 0.12,
                    ),
                    AveragePriceEntity(
                        id = "avg-$municipality-gasoline",
                        surveyWeekId = surveyWeekId,
                        state = state.abbreviation,
                        municipality = municipality.uppercase(),
                        fuelProduct = "GASOLINE_REGULAR",
                        stationCount = 38,
                        unit = "R$/l",
                        avgPrice = 5.89,
                        minPrice = 5.50,
                        maxPrice = 6.20,
                        stdDev = 0.15,
                    ),
                ),
            )
        } finally {
            database.close()
        }

        SyncStateDataStore(context).writeState(SyncJobState.COMPLETED)
    }

    private suspend fun seedSampleVehicle(context: Context) {
        val vehicle = Vehicle.create(
            displayName = "Gol",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            sortOrder = 0,
        )
        val database = Room.databaseBuilder(context, AnpFuelDatabase::class.java, DATABASE_NAME)
            .setJournalMode(RoomDatabase.JournalMode.WRITE_AHEAD_LOGGING)
            .addMigrations(
                AnpFuelDatabaseMigrations.MIGRATION_1_2,
                AnpFuelDatabaseMigrations.MIGRATION_2_3,
                AnpFuelDatabaseMigrations.MIGRATION_3_4,
            )
            .allowMainThreadQueries()
            .build()
        try {
            database.vehicleDao().upsert(VehicleMapper.toEntity(vehicle))
        } finally {
            database.close()
        }
    }

    fun clearAppStorage(context: Context) {
        context.deleteDatabase(DATABASE_NAME)
        deleteDatastoreFiles(context, "user_preferences")
        deleteDatastoreFiles(context, "sync_state")
        context.getDatabasePath(DATABASE_NAME).parentFile
            ?.listFiles()
            ?.filter { it.name.startsWith(DATABASE_NAME) }
            ?.forEach(File::delete)
        runCatching { WorkManager.getInstance(context).cancelAllWork() }
        runBlocking {
            SyncStateDataStore(context).writeState(SyncJobState.IDLE)
        }
    }

    private fun deleteDatastoreFiles(context: Context, storeName: String) {
        val datastoreDir = context.filesDir.resolve("datastore")
        datastoreDir.listFiles()
            ?.filter { it.name.startsWith(storeName) }
            ?.forEach(File::delete)
    }
}
