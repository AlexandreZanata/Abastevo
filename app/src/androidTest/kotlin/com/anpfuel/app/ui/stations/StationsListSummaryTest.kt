package com.anpfuel.app.ui.stations

import androidx.compose.ui.platform.testTag
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.test.platform.app.InstrumentationRegistry
import android.graphics.Bitmap
import java.io.File
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class StationsListSummaryTest {
    @get:Rule val compose = createComposeRule()

    @Test fun summaryReadsAsInformationAndHasNoClickAction() {
        compose.setContent {
            AnpFuelTheme(darkTheme = true, dynamicColor = false) {
                StationsListSummary("SORRISO", BrazilianState.MATO_GROSSO, FuelProduct.GASOLINE_REGULAR,
                    Modifier.testTag("summary"))
            }
        }
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        File(context.getExternalFilesDir(null), "stations-list-summary.png").outputStream().use {
            compose.onNodeWithTag("summary").captureToImage().asAndroidBitmap().compress(Bitmap.CompressFormat.PNG, 100, it)
        }
        compose.onNodeWithTag("summary").assertHasNoClickAction()
        compose.onNodeWithText("SORRISO, MT", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithTag("summary").assert(SemanticsMatcher.keyNotDefined(
            androidx.compose.ui.semantics.SemanticsProperties.EditableText,
        ))
    }
}
