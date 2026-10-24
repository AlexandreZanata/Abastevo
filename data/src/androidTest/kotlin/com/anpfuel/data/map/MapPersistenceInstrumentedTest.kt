package com.anpfuel.data.map

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.data.local.map.MapGenerationEntity
import com.anpfuel.data.local.map.MapPackageEntity
import java.security.KeyPairGenerator
import java.security.Signature
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MapPersistenceInstrumentedTest {
    @Test
    fun roomMapTablesAndEd25519ProviderWorkOnDevice() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val db = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java).build()
        try {
            val dao = db.mapPackageDao()
            dao.insertGeneration(MapGenerationEntity("g1", "hash", false, null))
            dao.insertPackages(listOf(
                MapPackageEntity("region", "g1", "REGION", "region.mwm", 10, "a".repeat(64), null, PACKAGE_PENDING, 0, null, 1),
            ))
            assertEquals(PACKAGE_PENDING, dao.packageById("g1", "region")?.status)
            dao.updatePackage("g1", "region", PACKAGE_READY, 10, "etag", null, 2)
            dao.activateGeneration("g1", 3)
            assertEquals("g1", dao.activeGeneration()?.generation)
            assertEquals(PACKAGE_READY, dao.packageById("g1", "region")?.status)
        } finally {
            db.close()
        }

        val keys = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()
        val message = "abastevo-map-instrumented".toByteArray()
        val signature = Signature.getInstance("Ed25519").run {
            initSign(keys.private)
            update(message)
            sign()
        }
        assertTrue(Signature.getInstance("Ed25519").run {
            initVerify(keys.public)
            update(message)
            verify(signature)
        })
    }
}
