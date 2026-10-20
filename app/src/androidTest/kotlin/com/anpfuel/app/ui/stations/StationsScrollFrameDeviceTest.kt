package com.anpfuel.app.ui.stations

import android.os.Handler
import android.os.HandlerThread
import android.os.SystemClock
import android.util.Log
import android.view.InputDevice
import android.view.FrameMetrics
import android.view.MotionEvent
import android.view.Window
import androidx.activity.compose.setContent
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.app.MainActivity
import com.anpfuel.app.ui.theme.AnpFuelTheme
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.valueobject.BrazilianState
import java.util.concurrent.ConcurrentLinkedQueue
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Actual touch/Choreographer timing, separated from Compose's virtual test clock. */
@RunWith(AndroidJUnit4::class)
class StationsScrollFrameDeviceTest {
    @Test fun actualTouchScrollRecordsDebugFramesWithoutChangingUserData() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val rows = (0 until 5_000).map { ServerStation.create(
            "00000000-0000-4000-8000-%012d".format(it), "Synthetic station $it",
            StationLocationQuality.UNKNOWN, null, null, null, "3550308", "SP", null) }
        val state = StationsUiState(isLoading = false, municipality = "Synthetic City",
            state = BrazilianState.SAO_PAULO, serverStations = rows, serverFromCache = true)
        val frames = ConcurrentLinkedQueue<Long>()
        val thread = HandlerThread("pagination-real-frame-diagnostic").apply { start() }
        val listener = Window.OnFrameMetricsAvailableListener { _, metrics, _ ->
            if (metrics.getMetric(FrameMetrics.FIRST_DRAW_FRAME) == 0L) {
                frames.add(metrics.getMetric(FrameMetrics.TOTAL_DURATION))
            }
        }
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            var width = 0
            var height = 0
            scenario.onActivity { activity ->
                activity.setContent {
                    AnpFuelTheme {
                        StationsContent(state, onFuelProductSelected = {}, onSearchQueryChanged = {},
                            onFindNearestStation = {}, onDownloadStationDetail = {}, onRetry = {},
                            onWeekChanged = {}, onStationSelected = {}, onNavigateToStation = {},
                            onDetailDismissed = {}, onNavigateToUpdatePrice = {}, weekPickerAction = {})
                    }
                }
            }
            instrumentation.waitForIdleSync()
            SystemClock.sleep(500)
            scenario.onActivity { activity ->
                width = activity.window.decorView.width
                height = activity.window.decorView.height
                activity.window.addOnFrameMetricsAvailableListener(listener, Handler(thread.looper))
            }
            try {
                repeat(6) {
                    val downTime = SystemClock.uptimeMillis()
                    fun event(action: Int, y: Float) {
                        val event = MotionEvent.obtain(downTime, SystemClock.uptimeMillis(), action,
                            width * .5f, y, 0)
                        event.source = InputDevice.SOURCE_TOUCHSCREEN
                        try { assertTrue(instrumentation.uiAutomation.injectInputEvent(event, true)) }
                        finally { event.recycle() }
                    }
                    event(MotionEvent.ACTION_DOWN, height * .8f)
                    for (step in 1..20) {
                        SystemClock.sleep(16)
                        event(MotionEvent.ACTION_MOVE, height * (.8f - .5f * step / 20))
                    }
                    event(MotionEvent.ACTION_UP, height * .3f)
                    SystemClock.sleep(300)
                }
                instrumentation.waitForIdleSync()
            } finally {
                scenario.onActivity { it.window.removeOnFrameMetricsAvailableListener(listener) }
                thread.quitSafely()
                thread.join(2_000)
            }
        }
        val milliseconds = frames.map { it / 1_000_000.0 }.sorted()
        assertTrue("Actual rendered frames must be observed", milliseconds.isNotEmpty())
        Log.i("PaginationRealFrames", "synthetic_rows=5000 gestures=6 frames=${milliseconds.size} " +
            "p50_ms=${milliseconds[milliseconds.size / 2]} " +
            "p95_ms=${milliseconds[((milliseconds.size - 1) * .95).toInt()]}")
    }
}
