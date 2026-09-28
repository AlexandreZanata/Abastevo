package com.anpfuel.application.usecase.alert

import com.anpfuel.application.format.BrlPriceFormatter
import com.anpfuel.domain.model.PriceDropAlertNotification
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.model.UserPreferences
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.repository.AveragePriceRepository
import com.anpfuel.domain.repository.PriceDropNotificationRepository
import com.anpfuel.domain.repository.PriceTableRepository
import com.anpfuel.domain.repository.StationPriceRepository
import com.anpfuel.domain.repository.UserPreferencesRepository
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class EvaluatePriceDropAlertsUseCaseTest {

    private val vehicleRepository = mockk<VehicleRepository>()
    private val averagePriceRepository = mockk<AveragePriceRepository>()
    private val stationPriceRepository = mockk<StationPriceRepository>()
    private val priceTableRepository = mockk<PriceTableRepository>()
    private val userPreferencesRepository = mockk<UserPreferencesRepository>()
    private val priceDropNotificationRepository = mockk<PriceDropNotificationRepository>(relaxed = true)

    private lateinit var useCase: EvaluatePriceDropAlertsUseCase

    private val currentWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val previousWeek = SurveyWeek.fromIsoDates("2026-05-31", "2026-06-06")
    private val currentSurveyId = DomainId.forSurveyWeek(currentWeek)
    private val previousSurveyId = DomainId.forSurveyWeek(previousWeek)
    private val state = BrazilianState.PARANA
    private val municipality = "Curitiba"

    @BeforeEach
    fun setUp() {
        useCase = EvaluatePriceDropAlertsUseCase(
            vehicleRepository = vehicleRepository,
            averagePriceRepository = averagePriceRepository,
            stationPriceRepository = stationPriceRepository,
            priceTableRepository = priceTableRepository,
            userPreferencesRepository = userPreferencesRepository,
            priceDropNotificationRepository = priceDropNotificationRepository,
        )

        coEvery { priceDropNotificationRepository.hasPostNotificationsPermission() } returns true
        coEvery { userPreferencesRepository.getPreferences() } returns UserPreferences(
            preferredState = state,
            preferredMunicipality = municipality,
            activeSurveyWeek = currentWeek,
        )
        coEvery { priceTableRepository.getImportedPriceSurveys() } returns listOf(
            survey(currentWeek),
            survey(previousWeek),
        )
        coEvery {
            stationPriceRepository.hasStationData(currentWeek, state, municipality)
        } returns true
        coEvery {
            stationPriceRepository.hasStationData(previousWeek, state, municipality)
        } returns true
        coEvery {
            averagePriceRepository.getPricesByMunicipality(state, municipality, any())
        } returns emptyList()
    }

    @Test
    fun skipsWhenNotificationPermissionDenied() = runTest {
        coEvery { priceDropNotificationRepository.hasPostNotificationsPermission() } returns false

        val result = useCase()

        assertEquals(0, result.notificationsShown)
        assertEquals(PriceDropAlertSkipReason.PERMISSION_DENIED, result.skipReason)
        coVerify(exactly = 0) { vehicleRepository.listAll() }
    }

    @Test
    fun skipsWhenNoAlertsEnabled() = runTest {
        coEvery { vehicleRepository.listAll() } returns listOf(
            vehicle("Gol", priceDropAlertEnabled = false),
        )

        val result = useCase()

        assertEquals(0, result.notificationsShown)
        assertEquals(PriceDropAlertSkipReason.NO_ALERTS_ENABLED, result.skipReason)
    }

    @Test
    fun notifiesForMultipleVehiclesWithPriceDrop() = runTest {
        val vehicleOne = vehicle("Gol")
        val vehicleTwo = vehicle("Onix")
        coEvery { vehicleRepository.listAll() } returns listOf(vehicleOne, vehicleTwo)
        stubStationPrices(
            currentPrices = listOf("5.40", "4.90"),
            previousPrices = listOf("5.60", "5.10"),
        )

        val result = useCase()

        assertEquals(2, result.notificationsShown)
        assertNull(result.skipReason)
        coVerify(exactly = 2) { priceDropNotificationRepository.showPriceDropAlert(any()) }
    }

    @Test
    fun doesNotNotifyWhenPriceDidNotDrop() = runTest {
        coEvery { vehicleRepository.listAll() } returns listOf(vehicle("Gol"))
        stubStationPrices(
            currentPrices = listOf("5.60"),
            previousPrices = listOf("5.40"),
        )

        val result = useCase()

        assertEquals(0, result.notificationsShown)
        assertNull(result.skipReason)
        coVerify(exactly = 0) { priceDropNotificationRepository.showPriceDropAlert(any()) }
    }

    @Test
    fun notificationIncludesVehicleNameAndFormattedPrice() = runTest {
        val trackedVehicle = vehicle("Gol")
        coEvery { vehicleRepository.listAll() } returns listOf(trackedVehicle)
        stubStationPrices(
            currentPrices = listOf("5.40"),
            previousPrices = listOf("5.60"),
        )
        val notificationSlot = slot<PriceDropAlertNotification>()

        useCase()

        coVerify(exactly = 1) {
            priceDropNotificationRepository.showPriceDropAlert(capture(notificationSlot))
        }
        assertEquals(trackedVehicle.id, notificationSlot.captured.vehicleId)
        assertEquals("Gol", notificationSlot.captured.vehicleDisplayName)
        assertEquals(
            BrlPriceFormatter.format(PriceAmount.of("5.40")),
            notificationSlot.captured.currentPriceFormatted,
        )
    }

    private fun stubStationPrices(
        currentPrices: List<String>,
        previousPrices: List<String>,
    ) {
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                surveyWeek = currentWeek,
            )
        } returns currentPrices.map { price ->
            station(currentSurveyId, currentWeek, price)
        }
        coEvery {
            stationPriceRepository.getStationPrices(
                state = state,
                municipality = municipality,
                fuelProduct = FuelProduct.GASOLINE_REGULAR,
                surveyWeek = previousWeek,
            )
        } returns previousPrices.map { price ->
            station(previousSurveyId, previousWeek, price)
        }
    }

    private fun vehicle(name: String, priceDropAlertEnabled: Boolean = true): Vehicle =
        Vehicle.create(
            displayName = name,
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
            priceDropAlertEnabled = priceDropAlertEnabled,
        )

    private fun survey(surveyWeek: SurveyWeek): PriceSurvey =
        PriceSurvey.restore(
            id = DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = java.time.Instant.parse("2026-06-14T10:00:00Z"),
            stationImportedAt = java.time.Instant.parse("2026-06-14T11:00:00Z"),
        )

    private fun station(
        surveyId: DomainId,
        surveyWeek: SurveyWeek,
        price: String,
    ): StationPrice = StationPrice.create(
        priceSurveyId = surveyId,
        surveyWeek = surveyWeek,
        station = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "POSTO",
            tradeName = "POSTO",
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(price),
    )
}
