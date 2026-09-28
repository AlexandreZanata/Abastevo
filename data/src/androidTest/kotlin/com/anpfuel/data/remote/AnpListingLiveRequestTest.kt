package com.anpfuel.data.remote

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.remote.OkHttpClientFactory
import okhttp3.Request
import org.junit.Assert.assertEquals
import org.junit.Ignore
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AnpListingLiveRequestTest {

    @Test
    @Ignore("Requires network access to gov.br — enable manually for live POC validation")
    fun getAnpListingPageReturns200() {
        val client = OkHttpClientFactory.create()
        val request = Request.Builder()
            .url(AnpEndpoints.LISTING_PAGE_URL)
            .get()
            .build()

        client.newCall(request).execute().use { response ->
            assertEquals(200, response.code)
        }
    }
}
