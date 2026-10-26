package com.anpfuel.app.ui.map

import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapEngineHolder
import org.junit.Rule
import org.junit.Test
import org.junit.Assert.assertTrue
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeRouteScreenInstrumentedTest {
    @get:Rule val compose=createAndroidComposeRule<MainActivity>()
    private fun text(id:Int)=compose.activity.getString(id)
    @Test fun actualMapTabPlansOfflineWithoutLocationAndDisplaysNativePreview() {
        com.anpfuel.maps.engine.SharedPreferencesMapCameraStore(compose.activity).save(com.anpfuel.maps.engine.MapCamera(-25.43,-49.27,13))
        compose.onNodeWithContentDescription(text(R.string.nav_map),useUnmergedTree=true).performClick()
        compose.waitUntil(45_000){MapEngineHolder.engine.isInitialized() && MapEngineHolder.engine.isRenderingReady()}
        compose.waitUntil(30_000) {
            val camera=MapEngineHolder.engine.currentCamera()
            camera!=null && camera.zoom>=13 && kotlin.math.abs(camera.latitude+25.43)<0.1 && kotlin.math.abs(camera.longitude+49.27)<0.1
        }
        compose.onNodeWithTag("native-route-open").performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_origin),substring=false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextInput("-25.4295")
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextInput("-49.2719")
        compose.onNodeWithText(text(R.string.map22_coordinates),substring=false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_destination),substring=false).performScrollTo().performClick()
        compose.onNodeWithText(text(R.string.map22_latitude)).performTextReplacement("-25.4222")
        compose.onNodeWithText(text(R.string.map22_longitude)).performTextReplacement("-49.2640")
        compose.onNodeWithText(text(R.string.map22_coordinates),substring=false).performScrollTo().performClick()
        compose.onNodeWithTag("native-route-build").performScrollTo().performClick()
        compose.waitUntil(60_000) {compose.onAllNodesWithTag("native-route-preview").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithTag("native-route-preview").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.map22_show)).performScrollTo().performClick()
        compose.waitForIdle()
        compose.waitUntil(30_000) {
            val camera=MapEngineHolder.engine.currentCamera()
            camera!=null && camera.zoom>=13 && kotlin.math.abs(camera.latitude+25.43)<0.1
        }
        var image: android.graphics.Bitmap? = null
        compose.waitUntil(30_000) {
            image=androidx.test.platform.app.InstrumentationRegistry.getInstrumentation().uiAutomation.takeScreenshot()
            val bitmap=image ?: return@waitUntil false
            val colors=mutableSetOf<Int>()
            for(x in bitmap.width/5 until bitmap.width*4/5 step 20)
                for(y in bitmap.height*2/5 until bitmap.height*3/5 step 20) colors+=bitmap.getPixel(x,y)
            colors.size>20
        }
        assertTrue(image!=null)
        java.io.File(compose.activity.cacheDir,"map22-native-route.png").outputStream().use {image!!.compress(android.graphics.Bitmap.CompressFormat.PNG,100,it)}
        compose.runOnUiThread {MapEngineHolder.engine.cancelRoute()}
    }
}
