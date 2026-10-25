package com.anpfuel.app.ui.map

import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.R
import com.anpfuel.maps.engine.MapEngineHolder
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Opt-in local HTTP proof with admitted real MWM inputs, never synthetic map bytes. */
@RunWith(AndroidJUnit4::class)
class MapNativeDownloadInstrumentedTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()
    private fun label(id: Int) = compose.activity.getString(id)
    private fun waitText(text: String, timeout: Long = 60_000) {
        compose.waitUntil(timeout) {
            try { compose.onNodeWithText(text, substring = true).assertExists(); true }
            catch (_: AssertionError) { false }
        }
    }
    private fun openDownloads() {
        compose.onNodeWithTag("map-open-downloads").performClick()
    }
    private fun downloadCandidate(generation: String) {
        compose.waitUntil(30_000) {
            compose.onAllNodesWithTag("map-download-update").fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("map-download-update").performScrollTo().performClick()
        waitText(generation)
        compose.onNodeWithTag("map-download-start").performScrollTo().performClick()
        waitText(compose.activity.getString(R.string.map_downloads_active_generation, generation), 180_000)
        waitText(label(R.string.map_published), 180_000)
        compose.onNodeWithTag("map-download-back").performScrollTo().performClick()
    }
    @Test fun verifiedDownloadsRegisterInTheRunningEngineAndSurviveRecreation() {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("Needs configured local signed MWM distribution", args.getString("mapProofRun") == "true")
        val origin = requireNotNull(args.getString("mapProofOrigin"))
        fun serverGeneration(): String = org.json.JSONObject(
            URL("$origin/proof/requests").readText(),
        ).getString("generation")
        URL("$origin/proof/advance").readBytes()
        val firstGeneration = serverGeneration()
        waitText(label(R.string.nav_map))
        compose.onNodeWithContentDescription(label(R.string.nav_map), useUnmergedTree = true).performClick()
        val engine = MapEngineHolder.engine
        compose.waitUntil(45_000) { engine.isInitialized() }
        openDownloads()
        downloadCandidate(firstGeneration)
        compose.runOnIdle { engine.setViewport(-25.4284, -49.2733, 15) }
        compose.waitUntil(45_000) { engine.isDownloadedMapAtScreenCenter() }
        assertTrue("regional map must be registered", engine.downloadedMapsCount() > 0)
        val searched = CountDownLatch(1)
        var results = 0
        compose.runOnIdle {
            assertTrue(engine.search(compose.activity, "Curitiba") { results = it.size; searched.countDown() })
        }
        assertTrue("offline search must finish", searched.await(30, TimeUnit.SECONDS))
        assertTrue("offline search must return real features", results > 0)
        // Advance only the local proof server. The second generation has the same
        // admitted bytes; this exercises refresh while the native engine is open.
        val connection = URL("$origin/proof/advance").openConnection() as HttpURLConnection
        try { assertEquals(200, connection.responseCode) } finally { connection.disconnect() }
        openDownloads()
        downloadCandidate(serverGeneration())
        compose.runOnIdle { engine.setViewport(-25.4284, -49.2733, 15) }
        compose.waitUntil(45_000) { engine.isDownloadedMapAtScreenCenter() }
        compose.runOnUiThread { compose.activity.recreate() }
        compose.waitUntil(45_000) { engine.isDownloadedMapAtScreenCenter() }
        assertSame(engine, MapEngineHolder.engine)
        assertTrue(engine.writableDir().orEmpty().contains("com.anpfuel.app.debug/files"))
    }
}
