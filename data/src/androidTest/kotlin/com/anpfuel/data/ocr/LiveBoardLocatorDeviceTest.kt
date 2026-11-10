package com.anpfuel.data.ocr

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ImageFormat
import android.graphics.Matrix
import android.graphics.Paint
import android.media.Image
import android.media.ImageReader
import android.media.ImageWriter
import android.os.SystemClock
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.data.local.ocr.MlKitLiveBoardLocator
import com.anpfuel.domain.portable.LiveBoardZoom
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Live preview framing on generated YUV frames only, delivered as real [Image]s like the
 * camera's: a board seen upright, the same board in a sensor-rotated frame, and an empty
 * scene. No camera, account, contribution or private fixture.
 */
@RunWith(AndroidJUnit4::class)
class LiveBoardLocatorDeviceTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    /** Portrait 960 x 1280 scene with a four-row board whose box is (200, 500)-(880, 905). */
    private fun uprightScene(board: Boolean): Bitmap {
        val bitmap = Bitmap.createBitmap(960, 1280, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        canvas.drawColor(Color.rgb(120, 150, 170))
        if (board) {
            val size = 40f
            val row = size * 2.2f
            val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.WHITE }
            canvas.drawRect(200f, 500f, 200f + size * 17f, 500f + row * 4.6f, paint)
            paint.color = Color.BLACK
            paint.textSize = size
            listOf("ETANOL" to "4,32", "GASOLINA COMUM" to "6,59", "GASOLINA ADITIVADA" to "6,79", "DIESEL S10" to "6,19")
                .forEachIndexed { index, (name, price) ->
                    val baseline = 500f + row * (index + 1)
                    canvas.drawText(name, 200f + size * 0.6f, baseline, paint)
                    canvas.drawText(price, 200f + size * 13.5f, baseline, paint)
                }
        }
        return bitmap
    }

    /** What a sensor mounted a quarter turn away delivers: the scene turned 90° counter-clockwise. */
    private fun sensorRotated(upright: Bitmap): Bitmap {
        val matrix = Matrix().apply { setRotate(-90f); postTranslate(0f, upright.width.toFloat()) }
        val bitmap = Bitmap.createBitmap(upright.height, upright.width, Bitmap.Config.ARGB_8888)
        Canvas(bitmap).drawBitmap(upright, matrix, null)
        return bitmap
    }

    /** Runs [block] with [bitmap] as a YUV_420_888 [Image] from an [ImageReader]. */
    private fun <T> withYuvImage(bitmap: Bitmap, block: (Image) -> T): T {
        val reader = ImageReader.newInstance(bitmap.width, bitmap.height, ImageFormat.YUV_420_888, 2)
        val writer = ImageWriter.newInstance(reader.surface, 2, ImageFormat.YUV_420_888)
        try {
            val input = writer.dequeueInputImage()
            val pixels = IntArray(bitmap.width * bitmap.height).also { bitmap.getPixels(it, 0, bitmap.width, 0, 0, bitmap.width, bitmap.height) }
            val (y, u, v) = input.planes
            for (row in 0 until bitmap.height) for (column in 0 until bitmap.width) {
                val color = pixels[row * bitmap.width + column]
                val r = Color.red(color); val g = Color.green(color); val b = Color.blue(color)
                y.buffer.put(row * y.rowStride + column * y.pixelStride, ((66 * r + 129 * g + 25 * b + 128 shr 8) + 16).toByte())
                if (row % 2 == 0 && column % 2 == 0) {
                    val chroma = (row / 2) * u.rowStride + (column / 2) * u.pixelStride
                    u.buffer.put(chroma, ((-38 * r - 74 * g + 112 * b + 128 shr 8) + 128).toByte())
                    v.buffer.put((row / 2) * v.rowStride + (column / 2) * v.pixelStride, ((112 * r - 94 * g - 18 * b + 128 shr 8) + 128).toByte())
                }
            }
            writer.queueInputImage(input)
            val deadline = SystemClock.elapsedRealtime() + 3000
            var image: Image? = null
            while (image == null && SystemClock.elapsedRealtime() < deadline) {
                image = reader.acquireLatestImage() ?: run { SystemClock.sleep(20); null }
            }
            return checkNotNull(image) { "no YUV frame" }.use(block)
        } finally {
            writer.close()
            reader.close()
        }
    }

    private fun locate(session: MlKitLiveBoardLocator.Session, image: Image, rotation: Int): LiveBoardZoom.Board? {
        val done = CountDownLatch(1)
        var found: LiveBoardZoom.Board? = null
        session.locate(image, rotation) { found = it; done.countDown() }
        assertTrue("reading did not finish", done.await(10, TimeUnit.SECONDS))
        return found
    }

    private fun assertBoard(board: LiveBoardZoom.Board?) {
        assertNotNull("board not found", board)
        board!!
        // Drawn box as fractions: 0.21..0.92 wide, 0.39..0.71 high; the frame adds a margin.
        assertTrue("left ${board.left}", board.left in 0.10..0.26)
        assertTrue("right ${board.right}", board.right in 0.85..1.0)
        assertTrue("top ${board.top}", board.top in 0.30..0.45)
        assertTrue("bottom ${board.bottom}", board.bottom in 0.62..0.80)
    }

    @Test fun boardIsLocatedInUprightAndSensorRotatedFramesAndNotInAnEmptyScene() = runBlocking {
        assertTrue(MlKitImagePriceOcr(context).prepareModel())
        MlKitLiveBoardLocator.open().use { session ->
            val upright = uprightScene(board = true)
            val rotated = sensorRotated(upright)
            val empty = uprightScene(board = false)
            try {
                assertBoard(withYuvImage(upright) { locate(session, it, 0) })
                assertBoard(withYuvImage(rotated) { locate(session, it, 90) })
                assertNull(withYuvImage(empty) { locate(session, it, 0) })
            } finally {
                upright.recycle(); rotated.recycle(); empty.recycle()
            }
        }
    }

    @Test fun aClosedSessionReportsNoBoard() {
        val session = MlKitLiveBoardLocator.open()
        session.close()
        val upright = uprightScene(board = true)
        try {
            assertNull(withYuvImage(upright) { locate(session, it, 0) })
        } finally { upright.recycle() }
    }
}
