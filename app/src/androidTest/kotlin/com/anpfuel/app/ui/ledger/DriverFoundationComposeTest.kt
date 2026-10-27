package com.anpfuel.app.ui.ledger

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.monthly.MonthlyIndicatorsContent
import com.anpfuel.app.ui.monthly.MonthlyIndicatorsUiState
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.application.usecase.ledger.LedgerEntryRow
import com.anpfuel.application.usecase.ledger.LedgerEntryState
import com.anpfuel.application.usecase.ledger.LedgerEntrySummary
import com.anpfuel.application.usecase.ledger.MonthlyVehicleIndicators
import com.anpfuel.application.usecase.ledger.MonthlyWindowNote
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.OdometerReviewChoice
import com.anpfuel.domain.premium.PremiumConsumptionResult
import com.anpfuel.domain.valueobject.DomainId
import java.time.Instant
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * DRV-01/DRV-02 rendered proof on a real device: synthetic, clearly identified
 * rows only (no real user data). Pure state in, actual Compose nodes out.
 */
@RunWith(AndroidJUnit4::class)
class DriverFoundationComposeTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val ptBr = Locale.forLanguageTag("pt-BR")
    private val vehicle = DomainId.from("22222222-2222-4222-8222-222222222222")
    private fun string(id: Int, vararg args: Any) =
        InstrumentationRegistry.getInstrumentation().targetContext.getString(id, *args)

    private fun indicators(
        name: String,
        measured: PremiumConsumptionResult,
        costPerKm: String,
        unverified: Boolean,
    ) = MonthlyVehicleIndicators(
        vehicleId = vehicle, vehicleName = name, year = 2026, month = 3, timezone = "America/Cuiaba",
        purchaseCentavos = 26000, expenseCentavos = 0, chargeCostCentavos = 0, chargeEnergyWh = 0,
        chargeFeesCentavos = 0, byCategory = emptyMap(), recordCount = 2,
        measured = measured, costPerKm = costPerKm,
        costPerKmMethod = if (costPerKm.isEmpty()) "" else "fuel-purchase-interval",
        combinedCentavosPerKm = "", combinedMethod = "", combinedUnknown = "odometer-unit-unverified".takeIf { unverified } ?: "",
        estimatedKmPerL = null, comparisonChangePercent = "", comparisonUnknown = "no-baseline",
        windowNote = MonthlyWindowNote(2, 2, false, if (unverified) 0 else 500000, false, unverified),
    )

    private fun showMonthly(vararg rows: MonthlyVehicleIndicators) {
        composeTestRule.setContent {
            AnpFuelTheme {
                MonthlyIndicatorsContent(
                    uiState = MonthlyIndicatorsUiState(isLoading = false, vehicleId = vehicle, vehicleName = "DRV synthetic",
                        year = 2026, month = 3, rows = rows.toList()),
                    locale = ptBr, onNavigateBack = {}, onPreviousMonth = {}, onNextMonth = {},
                    onRetry = {}, onExport = {}, onDismissError = {},
                )
            }
        }
    }

    @Test
    fun verifiedAnchorsRenderTwelvePointFiveKmPerLitreAndFiftyTwoCentavosPerKm() {
        showMonthly(
            indicators(
                "DRV-VERIFIED",
                PremiumConsumptionResult(40000, 26000, 500000, "25/2", "52"),
                costPerKm = "52", unverified = false,
            ),
        )
        composeTestRule.onNodeWithText("12,5 km/L", substring = true).assertIsDisplayed()
        composeTestRule.onNodeWithText("0,52", substring = true).assertIsDisplayed()
        // Purchase line and measured-cost line both carry the exact BRL 260,00.
        assertEquals(2, composeTestRule.onAllNodesWithText("260,00", substring = true).fetchSemanticsNodes().size)
        assertEquals(0, composeTestRule.onAllNodesWithText(string(R.string.monthly_review_required)).fetchSemanticsNodes().size)
    }

    @Test
    fun unverifiedLegacyUnitBlocksIndicatorsButKeepsPurchasesAndOffersReview() {
        showMonthly(
            indicators(
                "DRV-LEGACY",
                PremiumConsumptionResult(unknown = "odometer-unit-unverified"),
                costPerKm = "", unverified = true,
            ),
        )
        composeTestRule.onNodeWithText(string(R.string.monthly_review_required)).assertIsDisplayed()
        // The real purchase total remains readable.
        assertEquals(1, composeTestRule.onAllNodesWithText("260,00", substring = true).fetchSemanticsNodes().size)
        composeTestRule.onNodeWithText("260,00", substring = true).assertIsDisplayed()
        // No ratio is rendered as if it had been measured.
        assertEquals(0, composeTestRule.onAllNodesWithText("km/L", substring = true).fetchSemanticsNodes().size)
    }

    private fun entriesContent(
        form: LedgerEntryFormState, entries: List<LedgerEntryRow> = emptyList(),
        onReview: (OdometerReviewChoice) -> Unit = {}, onSave: () -> Unit = {},
        showForm: Boolean = true,
    ) {
        composeTestRule.setContent {
            AnpFuelTheme {
                LedgerEntriesContent(
                    uiState = LedgerEntriesUiState(isLoading = false, vehicleId = vehicle, vehicleName = "DRV synthetic",
                        entries = entries, showForm = showForm, form = form),
                    locale = ptBr, vehicleIsElectric = false, simulatedTotal = null,
                    onNavigateBack = {}, onAddRefuel = {}, onAddExpense = {}, onEdit = {}, onQueue = {},
                    onRequestDelete = {}, onConfirmDelete = {}, onDismissDelete = {}, onOpenGap = {}, onDismissGap = {},
                    onGapBeforeChanged = {}, onGapNoteChanged = {}, onConfirmGap = {}, onRequestReset = {},
                    onConfirmReset = {}, onDismissReset = {}, onDismissForm = {}, onDatasetChanged = {},
                    onDateChanged = {}, onTimeChanged = {}, onQuantityChanged = {}, onTotalChanged = {},
                    onUnitPriceChanged = {}, onRateChanged = {}, onFeesChanged = {}, onSocStartChanged = {}, onSocEndChanged = {},
                    onTariffIdChanged = {}, onTariffRevisionChanged = {}, onContextChanged = {}, onFillChanged = {},
                    onCategoryChanged = {}, onOdometerChanged = {}, onStationChanged = {}, onNoteChanged = {},
                    onSaveDraft = onSave, onEstimateChanged = {}, onSaveEstimate = {}, onClearEstimate = {},
                    onDismissError = {}, onOpenMonthly = {}, onReviewOdometer = onReview,
                )
            }
        }
    }

    @Test
    fun legacyReviewFormShowsPreviewsLocksOdometerAndRequiresExplicitChoice() {
        val chosen = mutableListOf<OdometerReviewChoice>()
        var saves = 0
        entriesContent(
            LedgerEntryFormState(
                entryId = "synthetic-legacy-1", dataset = LedgerDataset.REFUEL, fill = LedgerFill.FULL,
                odometerNeedsReview = true, originalOdometerValue = 10500L, reviewBaseHash = "0".repeat(64),
                metresReviewKm = "10,5", kilometresReviewKm = "10500",
            ),
            onReview = { chosen += it }, onSave = { saves++ },
        )
        composeTestRule.onNodeWithText(string(R.string.ledger_review_required)).assertIsDisplayed()
        composeTestRule.onNodeWithText(string(R.string.ledger_review_saved, 10500L)).assertIsDisplayed()
        val asMetres = string(R.string.ledger_review_metres, "10,5")
        val asKm = string(R.string.ledger_review_kilometres, "10500")
        composeTestRule.onNodeWithText(asMetres).assertIsDisplayed()
        composeTestRule.onNodeWithText(asKm).assertIsDisplayed()
        // Selecting neither option has written nothing yet.
        assertTrue(chosen.isEmpty())
        composeTestRule.onNodeWithText(asKm).performClick()
        assertEquals(listOf(OdometerReviewChoice.AS_KILOMETRES), chosen)
        composeTestRule.onNodeWithText(asMetres).performClick()
        assertEquals(listOf(OdometerReviewChoice.AS_KILOMETRES, OdometerReviewChoice.AS_METRES), chosen)
        assertEquals(0, saves)
    }

    @Test
    fun legacyEntryRowAdvertisesUnitReview() {
        val row = LedgerEntryRow(
            "synthetic-legacy-2", LedgerDataset.REFUEL, vehicle.value, "DRV synthetic",
            Instant.parse("2026-03-01T15:00:00Z"), "America/Cuiaba",
            LedgerEntrySummary.Refuel(40000, 26000, LedgerFill.FULL), 0, null, LedgerEntryState.LOCAL, null,
            odometerNeedsReview = true,
        )
        entriesContent(LedgerEntryFormState(), entries = listOf(row), showForm = false)
        composeTestRule.onNodeWithText(string(R.string.ledger_review_entry)).assertIsDisplayed()
    }

    @Test
    fun reviewFailureIsShownWithoutSilentSave() {
        entriesContent(
            LedgerEntryFormState(
                entryId = "synthetic-legacy-3", dataset = LedgerDataset.EXPENSE,
                odometerNeedsReview = true, originalOdometerValue = 7L, reviewBaseHash = "1".repeat(64),
                metresReviewKm = "0,007", kilometresReviewKm = "7",
                validationErrorKey = LedgerEntryValidationError.ODOMETER_REVIEW_FAILED,
            ),
        )
        composeTestRule.onNodeWithText(string(R.string.ledger_review_failed)).assertIsDisplayed()
    }
}
