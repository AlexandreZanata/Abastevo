package com.anpfuel.data.ocr

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Matrix
import android.graphics.Paint
import android.os.Bundle
import androidx.exifinterface.media.ExifInterface
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.domain.portable.FuelBoardOcr
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.ByteArrayOutputStream
import java.io.File
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Automatic framing on generated pixels only: a far board in a large photo, a
 * tilted board and a dark board. No camera, account, contribution or private fixture.
 */
@RunWith(AndroidJUnit4::class)
class OcrFramingDeviceTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val expected = mapOf(
        FuelProduct.ETHANOL to 4320L, FuelProduct.GASOLINE_REGULAR to 6590L,
        FuelProduct.GASOLINE_PREMIUM to 6790L, FuelProduct.DIESEL_S10 to 6190L)

    /** A price board drawn at [textSize] with its top-left corner at ([x], [y]). */
    private fun drawBoard(canvas: Canvas, x: Float, y: Float, textSize: Float, paper: Int = Color.WHITE, ink: Int = Color.BLACK) {
        val row = textSize * 2.2f
        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = paper }
        canvas.drawRect(x, y, x + textSize * 17f, y + row * 4.6f, paint)
        paint.color = ink
        paint.textSize = textSize
        listOf("ETANOL" to "4,32", "GASOLINA COMUM" to "6,59", "GASOLINA ADITIVADA" to "6,79", "DIESEL S10" to "6,19")
            .forEachIndexed { index, (name, price) ->
                val baseline = y + row * (index + 1)
                canvas.drawText(name, x + textSize * 0.6f, baseline, paint)
                canvas.drawText(price, x + textSize * 13.5f, baseline, paint)
            }
    }

    private fun jpeg(bitmap: Bitmap): ByteArray =
        ByteArrayOutputStream().also { assertTrue(bitmap.compress(Bitmap.CompressFormat.JPEG, 92, it)) }.toByteArray()

    private fun rows(result: FuelBoardOcr.Result) = result.rows.associate { it.product to it.amountMilli }

    private fun report(name: String, value: String) =
        InstrumentationRegistry.getInstrumentation().sendStatus(0, Bundle().apply { putString(name, value) })

    @Test fun farBoardInLargePhotoIsReadFromTheCameraDetail() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        // 12 MP scenes with a far board; bounding the photo to 2048 px halves these digit heights.
        fun scene(textSize: Float): ByteArray {
            val bitmap = Bitmap.createBitmap(4000, 3000, Bitmap.Config.ARGB_8888)
            return try {
                Canvas(bitmap).apply { drawColor(Color.rgb(120, 150, 170)); drawBoard(this, 2550f, 1700f, textSize) }
                jpeg(bitmap)
            } finally { bitmap.recycle() }
        }
        val summary = StringBuilder()
        for (textSize in listOf(16f, 18f, 20f)) {
            val bytes = scene(textSize)
            val detailed = rows(withTimeout(20000) { engine.recognize(bytes) })
            val bounded = rows(withTimeout(20000) { FuelBoardOcr.reconcile(engine.analyze(bytes, cameraDetail = false).views) })
            val right = detailed.count { expected[it.key] == it.value }
            val rightBounded = bounded.count { expected[it.key] == it.value }
            summary.append("text${textSize.toInt()}px camera_detail=$right bounded_only=$rightBounded; ")
            assertTrue("camera detail must not read fewer rows at $textSize", right >= rightBounded)
            assertTrue("no wrong row at $textSize", detailed.all { expected[it.key] == it.value })
        }
        report("far_board_rows_of_${expected.size}", summary.toString())
        // At 18 px text the board is invisible in the bounded photo and is found in the camera's own pixels.
        val far = scene(18f)
        assertEquals(expected, rows(withTimeout(20000) { engine.recognize(far) }))
        assertTrue(withTimeout(20000) { FuelBoardOcr.reconcile(engine.analyze(far, cameraDetail = false).views) }.rows.isEmpty())
        val upright = scene(22f)
        val framed = withTimeout(20000) { engine.recognizeFramed(upright) }
        assertEquals(expected, rows(framed.result))
        val frame = assertNotNullFrame(framed.frame)
        assertTrue("frame must sit on the board", frame.left > 0.55f && frame.top > 0.5f && frame.right < 0.8f && frame.bottom < 0.75f)

        // The same scene stored rotated with EXIF orientation 6 must give the same upright result.
        val file = File(context.cacheDir, "generated-ocr-far.jpeg")
        val rotated = Bitmap.createBitmap(4000, 3000, Bitmap.Config.ARGB_8888)
        try {
            Canvas(rotated).apply { drawColor(Color.rgb(120, 150, 170)); drawBoard(this, 2550f, 1700f, 22f) }
            val inverse = Matrix().apply { setRotate(-90f) }
            val stored = Bitmap.createBitmap(rotated, 0, 0, rotated.width, rotated.height, inverse, true)
            try { file.writeBytes(jpeg(stored)) } finally { if (stored !== rotated) stored.recycle() }
            ExifInterface(file).apply { setAttribute(ExifInterface.TAG_ORIENTATION, "6"); saveAttributes() }
            val turned = withTimeout(20000) { engine.recognizeFramed(file.readBytes()) }
            assertEquals("EXIF=6", expected, rows(turned.result))
            val turnedFrame = assertNotNullFrame(turned.frame)
            assertEquals(frame.left, turnedFrame.left, 0.03f)
            assertEquals(frame.top, turnedFrame.top, 0.03f)
        } finally { rotated.recycle(); file.delete() }
    }

    @Test fun tiltedBoardIsStraightenedBeforeAssociation() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        for (degrees in listOf(-14f, 9f)) {
            val scene = Bitmap.createBitmap(1600, 1200, Bitmap.Config.ARGB_8888)
            val bytes = try {
                Canvas(scene).apply {
                    drawColor(Color.rgb(90, 110, 90))
                    rotate(degrees, 800f, 600f)
                    drawBoard(this, 330f, 380f, 54f)
                }
                jpeg(scene)
            } finally { scene.recycle() }
            val result = withTimeout(20000) { engine.recognize(bytes) }
            assertEquals("tilt=$degrees", expected, rows(result))
        }
    }

    @Test fun darkBoardIsReadAfterContrastStretch() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        val scene = Bitmap.createBitmap(1600, 1200, Bitmap.Config.ARGB_8888)
        val bytes = try {
            Canvas(scene).apply {
                drawColor(Color.rgb(8, 8, 10))
                drawBoard(this, 330f, 380f, 54f, paper = Color.rgb(38, 38, 40), ink = Color.rgb(14, 14, 16))
            }
            jpeg(scene)
        } finally { scene.recycle() }
        val result = withTimeout(20000) { engine.recognize(bytes) }
        report("dark_board_rows", "${rows(result).size} of ${expected.size}")
        assertEquals(expected, rows(result))
    }

    private fun assertNotNullFrame(frame: com.anpfuel.application.port.ImagePriceOcr.BoardFrame?): com.anpfuel.application.port.ImagePriceOcr.BoardFrame {
        assertNotNull("board frame", frame)
        return frame!!
    }
}
