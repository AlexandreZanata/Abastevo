package com.anpfuel.app.ui.stations

import android.graphics.Bitmap
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.R
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationAddressSource
import com.anpfuel.domain.discovery.StationLocationQuality
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ServerStationDetailSheetTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun lightSheetShowsAddressAndDistinctLegalName() {
        showStation(dark = false, located = true)
        compose.onNodeWithText("Posto Exemplo").assertIsDisplayed()
        compose.onNodeWithText("Comércio Exemplo LTDA").assertIsDisplayed()
        compose.onNodeWithText("Rua Exemplo, 10 – Centro, Curitiba/PR").assertIsDisplayed()
        compose.onNodeWithText(label(R.string.server_station_approximate_address)).assertIsDisplayed()
        compose.onNodeWithText("-25.42000, -49.27000").assertDoesNotExist()
        captureSheet("station-detail-light.png")
    }

    @Test
    fun darkSheetShowsMunicipalityWhenCoordinatesAreMissing() {
        showStation(dark = true, located = false)
        compose.onNodeWithText("Posto Exemplo").assertIsDisplayed()
        compose.onNodeWithText("Curitiba/PR").assertIsDisplayed()
        compose.onNodeWithText(label(R.string.server_station_unknown_location)).assertIsDisplayed()
        compose.onNodeWithText(label(R.string.server_station_copy_coordinates)).assertDoesNotExist()
        compose.onNodeWithText("4106902/PR").assertDoesNotExist()
        captureSheet("station-detail-dark.png")
    }

    private fun captureSheet(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        // Synchronize with the dialog's rendered frame before capturing the screen.
        compose.onNodeWithTag("station-detail").captureToImage()
        instrumentation.uiAutomation.waitForIdle(250, 5_000)
        val bitmap = checkNotNull(instrumentation.uiAutomation.takeScreenshot())
        File(context.cacheDir, name).outputStream().use {
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
        }
    }

    private fun label(id: Int): String =
        InstrumentationRegistry.getInstrumentation().targetContext.getString(id)

    private fun showStation(dark: Boolean, located: Boolean) {
        val station = ServerStation.create(
            stationId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            displayName = "Posto Exemplo",
            legalName = if (located) "Comércio Exemplo LTDA" else "Posto Exemplo",
            locationQuality = if (located) StationLocationQuality.REVIEWED else StationLocationQuality.UNKNOWN,
            latitude = if (located) -25.42 else null,
            longitude = if (located) -49.27 else null,
            cnpjNormalized = null,
            municipalityCode = "4106902",
            state = "PR",
            currentRevisionId = null,
            address = if (located) {
                mapOf("street" to "Rua Exemplo", "number" to "10", "district" to "Centro", "city" to "Curitiba")
            } else {
                mapOf("city" to "Curitiba")
            },
            addressSource = if (located) StationAddressSource.OSM else StationAddressSource.MUNICIPALITY,
        )
        compose.setContent {
            AnpFuelTheme(darkTheme = dark, dynamicColor = false) {
                ServerStationDetailSheet(
                    station = station,
                    fromCache = false,
                    onDismiss = {},
                    onRoute = {},
                    onUpdatePrice = {},
                    modifier = Modifier.testTag("station-detail"),
                )
            }
        }
    }
}
