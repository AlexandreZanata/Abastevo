package com.anpfuel.app.ui.stationprofile

import android.net.Uri
import android.os.SystemClock
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.profile.SignedClaimDocument
import java.io.IOException
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SignedClaimDocumentDeviceTest {
    private val resolver = InstrumentationRegistry.getInstrumentation().context.contentResolver
    @Test fun originalBoundedPDFReadsUnknownSizeProviderWithoutParser() = runBlocking {
        val start = SystemClock.elapsedRealtime()
        val runtime = Runtime.getRuntime()
        val before = runtime.totalMemory() - runtime.freeMemory()
        val body = SignedClaimDocument.read(resolver, Uri.parse("content://com.anpfuel.profile.test.documents/valid"))
        assertEquals(639 * 8192 + 9, body.size)
        assertEquals("%PDF-1.7\n", body.copyOfRange(0, 9).toString(Charsets.UTF_8))
        assertTrue(body.copyOfRange(9, body.size).all { it == 65.toByte() })
        val used = runtime.totalMemory() - runtime.freeMemory()
        android.util.Log.i("P32DocumentMeasurement", "bounded_read_ms=${SystemClock.elapsedRealtime() - start} bytes=${body.size} heap_before=$before heap_after=$used heap_max=${runtime.maxMemory()}")
        body.fill(0)
    }
    @Test fun expiredURIAndOversizeProviderRequireSelectionAgain() = runBlocking {
        for (path in listOf("expired", "oversize")) {
            try {
                SignedClaimDocument.read(resolver, Uri.parse("content://com.anpfuel.profile.test.documents/$path"))
                fail("$path unexpectedly read")
            } catch (error: IOException) {
                if (path == "oversize") assertEquals("Document exceeds limit", error.message)
                else assertTrue(error is java.io.FileNotFoundException)
            }
        }
    }
}
