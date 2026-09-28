package com.anpfuel.app.ui.components

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
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
