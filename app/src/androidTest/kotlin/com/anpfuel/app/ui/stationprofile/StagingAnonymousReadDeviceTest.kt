package com.anpfuel.app.ui.stationprofile

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.DirectoryStationHttpClient
import com.anpfuel.data.remote.OkHttpClientFactory
import com.anpfuel.data.remote.profile.StationProfileHttpClient
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Explicitly selected live read-only smoke. No accounts, proofs, GPS or writes. */
@RunWith(AndroidJUnit4::class)
class StagingAnonymousReadDeviceTest {
    @Test fun stagingDirectoryAndProfileKeepAnonymousContract() = runBlocking {
        assumeTrue("Live environment is selected explicitly", InstrumentationRegistry.getArguments().getString("p32LiveStaging") == "true")
        val client = OkHttpClientFactory.create(maxRetries = 0)
        val origin = ApiEnvironment.STAGING.origin
        val page = JSONObject(DirectoryStationHttpClient(client, origin).list(1, null))
        val items = page.getJSONArray("items")
        android.util.Log.i("P32LiveRead", "anonymous_directory_count=${items.length()}")
        if (items.length() > 0) {
            val id = items.getJSONObject(0).getString("station_id")
            val profile = StationProfileHttpClient(client, origin).getProfile(id)
            assertNotNull("Profile endpoint must be deployed", profile)
            assertEquals(id, profile!!.stationId)
        }
    }
}
