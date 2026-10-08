package com.anpfuel.data.ocr

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Matrix
import android.graphics.Paint
import androidx.exifinterface.media.ExifInterface
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.ByteArrayOutputStream
import java.io.File
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Generated pixels only: no camera permission, account, contribution or private fixture. */
@RunWith(AndroidJUnit4::class)
class OcrFormatMatrixDeviceTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun board(price: String, dark: Boolean = false, vertical: Boolean = false): Bitmap {
        val bitmap = Bitmap.createBitmap(1200, 550, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        canvas.drawColor(if (dark) Color.rgb(10, 60, 25) else Color.WHITE)
        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            color = if (dark) Color.WHITE else Color.BLACK
            textSize = 72f
        }
        canvas.drawText("ETANOL", 40f, 120f, paint)
        canvas.drawText(price, if (vertical) 40f else 800f, if (vertical) 230f else 120f, paint)
        canvas.drawText("GASOLINA", if (vertical) 650f else 40f, if (vertical) 120f else 400f, paint)
        canvas.drawText("6,59", if (vertical) 650f else 800f, if (vertical) 230f else 400f, paint)
        return bitmap
    }

    private fun encoded(bitmap: Bitmap, jpeg: Boolean = false): ByteArray =
        ByteArrayOutputStream().also {
            assertTrue(bitmap.compress(if (jpeg) Bitmap.CompressFormat.JPEG else Bitmap.CompressFormat.PNG, 95, it))
        }.toByteArray()

    @Test fun decimalSeparatorCurrencyAndLayoutPixels() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        val formats = listOf("4,32" to 4320L, "4.32" to 4320L, "4,321" to 4321L,
            "432" to 4320L, "4 32" to 4320L, "R$ 4,32" to 4320L)
        for ((price, amount) in formats) for ((dark, vertical) in listOf(false to false, true to false, false to true)) {
            val bitmap = board(price, dark, vertical)
            val bytes = try { encoded(bitmap) } finally { bitmap.recycle() }
            val result = withTimeout(15000) { engine.recognize(bytes) }
            assertEquals("$price dark=$dark vertical=$vertical",
                mapOf(FuelProduct.ETHANOL to amount, FuelProduct.GASOLINE_REGULAR to 6590L),
                result.rows.associate { it.product to it.amountMilli })
        }
    }

    @Test fun allExifOrientationsRecoverUprightPrices() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        val upright = board("4,321")
        try {
            for (orientation in 1..8) {
                val transform = Matrix().apply {
                    when (orientation) {
                        2 -> setScale(-1f, 1f)
                        3 -> setRotate(180f)
                        4 -> setScale(1f, -1f)
                        5 -> { setRotate(90f); postScale(-1f, 1f) }
                        6 -> setRotate(90f)
                        7 -> { setRotate(270f); postScale(-1f, 1f) }
                        8 -> setRotate(270f)
                    }
                }
                val inverse = Matrix()
                assertTrue(transform.invert(inverse))
                val raw = Bitmap.createBitmap(upright, 0, 0, upright.width, upright.height, inverse, true)
                val file = File(context.cacheDir, "generated-ocr-exif.jpeg")
                try {
                    file.writeBytes(encoded(raw, jpeg = true))
                    ExifInterface(file).apply {
                        setAttribute(ExifInterface.TAG_ORIENTATION, orientation.toString())
                        saveAttributes()
                    }
                    val result = withTimeout(15000) { engine.recognize(file.readBytes()) }
                    assertEquals("EXIF=$orientation", mapOf(FuelProduct.ETHANOL to 4321L,
                        FuelProduct.GASOLINE_REGULAR to 6590L), result.rows.associate { it.product to it.amountMilli })
                } finally {
                    file.delete()
                    if (raw !== upright) raw.recycle()
                }
            }
        } finally { upright.recycle() }
    }

    @Test fun nonPricePixelsAndBoundedInvalidInputsNeverBecomeFuelRows() = runBlocking {
        val engine = MlKitImagePriceOcr(context)
        assertTrue(engine.prepareModel())
        for (bytes in listOf(byteArrayOf(), byteArrayOf(1, 2, 3), ByteArray(32 * 1024 * 1024 + 1))) {
            assertTrue(runCatching { engine.recognize(bytes) }.isFailure)
        }
        val bitmap = Bitmap.createBitmap(1200, 550, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        canvas.drawColor(Color.WHITE)
        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = 64f }
        canvas.drawText("TOTAL R$ 549,60", 40f, 100f, paint)
        canvas.drawText("VOLUME 74,270 LITROS", 40f, 230f, paint)
        canvas.drawText("08/10/2026", 40f, 360f, paint)
        canvas.drawText("DIESEL S500 0,00", 40f, 490f, paint)
        val bytes = try { encoded(bitmap) } finally { bitmap.recycle() }
        val result = withTimeout(15000) { engine.recognize(bytes) }
        assertTrue(result.rows.isEmpty())
        assertTrue(result.orphans.isEmpty())
    }
}
