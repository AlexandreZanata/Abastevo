package com.anpfuel.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class FuelProductIconScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun ethanolLeafRemainsOpaqueWhiteOnLightAndDarkSurfaces() {
        composeTestRule.setContent {
            Row {
                listOf(false, true).forEach { dark ->
                    AnpFuelTheme(darkTheme = dark, dynamicColor = false) {
                        Box(modifier = Modifier.background(MaterialTheme.colorScheme.background)) {
                            FuelProductIcon(
                                product = FuelProduct.ETHANOL,
                                size = 96.dp,
                                modifier = Modifier.testTag("ethanol_$dark"),
                            )
                        }
                    }
                }
            }
        }
        listOf(false, true).forEach { dark ->
            val image = composeTestRule.onNodeWithTag("ethanol_$dark").captureToImage()
            val pixel = image.toPixelMap()[(image.width * 0.68f).toInt(), (image.height * 0.65f).toInt()]
            assertTrue("Ethanol leaf must be opaque white (dark=$dark)",
                pixel.red > 0.98f && pixel.green > 0.98f && pixel.blue > 0.98f && pixel.alpha > 0.98f)
        }
    }

    @Test
    fun allFuelIcons_lightThemeLayoutCapture() {
        composeTestRule.setContent {
            AnpFuelTheme(darkTheme = false, dynamicColor = false) {
                Column(modifier = Modifier.padding(16.dp)) {
                    FuelProduct.entries.forEach { product ->
                        FuelProductLabel(product = product)
                    }
                }
            }
        }

        composeTestRule.onRoot().captureToImage().apply {
            assertTrue(width > 0)
            assertTrue(height > 0)
        }
    }

    @Test
    fun allFuelIcons_darkThemeLayoutCapture() {
        composeTestRule.setContent {
            AnpFuelTheme(darkTheme = true, dynamicColor = false) {
                Column(modifier = Modifier.padding(16.dp)) {
                    FuelProduct.entries.forEach { product ->
                        FuelProductLabel(product = product)
                    }
                }
            }
        }

        composeTestRule.onRoot().captureToImage().apply {
            assertTrue(width > 0)
            assertTrue(height > 0)
        }
    }
}
