package com.anpfuel.data.ocr

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.local.ocr.MlKitImagePriceOcr
import com.anpfuel.domain.portable.FuelBoardOcr
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.ByteArrayOutputStream
import java.io.File
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Runtime-only private inputs via ADB; no photos, raw text or GPS in APK assets/logs. */
@RunWith(AndroidJUnit4::class)
class RealImageOcrDeviceTest {
    @Test fun privateCorpusPixels() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val directory = File(context.filesDir, "ocr-evaluation")
        val files = directory.listFiles()?.filter { it.extension.lowercase() in setOf("jpg","jpeg","png") }?.sortedBy { it.name }.orEmpty()
        assertTrue("Push the private evaluation corpus via run-as before executing", files.isNotEmpty())
        val engine=MlKitImagePriceOcr(context)
        assertTrue("Latin model is unavailable",engine.prepareModel())
        val output = JSONArray()
        for (file in files.take(1250)) {
            val start = android.os.SystemClock.elapsedRealtime()
            val record = JSONObject().put("file", file.name)
            try {
                val tokens = withTimeout(45000) { engine.recognizeTokens(file.readBytes()) }
                val result = FuelBoardOcr.associate(tokens)
                record.put("status", "recognized").put("conditional",result.conditional).put("unresolved",result.unresolved)
                    .put("rows",JSONArray(result.rows.map { JSONObject().put("fuel",it.product.name).put("milli_brl",it.amountMilli) }))
                // Diagnostics remain in the private, ignored evaluation artifact only.
                record.put("tokens",JSONArray(tokens.map { JSONObject().put("text",it.text).put("box",JSONArray(listOf(it.left,it.top,it.right,it.bottom))) }))
            } catch (error: Exception) {
                record.put("status","failed").put("error",error.javaClass.simpleName)
            }
            record.put("elapsed_ms",android.os.SystemClock.elapsedRealtime()-start)
            output.put(record)
            File(directory,"results.json").writeText(output.toString(2))
        }
        assertEquals(files.take(1250).size,output.length())
        assertTrue((0 until output.length()).any { output.getJSONObject(it).optString("status")=="recognized" })
    }

    @Test fun boundedInvalidInputAndSyntheticPixels() = runBlocking {
        val engine=MlKitImagePriceOcr(InstrumentationRegistry.getInstrumentation().targetContext)
        assertTrue("Latin model is unavailable",engine.prepareModel())
        assertTrue(runCatching { engine.recognize(byteArrayOf(1,2,3)) }.isFailure)
        val bitmap=Bitmap.createBitmap(1000,400,Bitmap.Config.ARGB_8888)
        val canvas=Canvas(bitmap);canvas.drawColor(Color.WHITE)
        val paint=Paint().apply{color=Color.BLACK;textSize=70f;isAntiAlias=true}
        canvas.drawText("ETANOL",40f,110f,paint);canvas.drawText("4,32",650f,110f,paint)
        canvas.drawText("GASOLINA",40f,260f,paint);canvas.drawText("6,59",650f,260f,paint)
        val bytes=ByteArrayOutputStream().also{bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}.toByteArray();bitmap.recycle()
        val result=withTimeout(45000){engine.recognize(bytes)}
        assertEquals(mapOf(FuelProduct.ETHANOL to 4320L,FuelProduct.GASOLINE_REGULAR to 6590L),result.rows.associate{it.product to it.amountMilli})
    }
}
