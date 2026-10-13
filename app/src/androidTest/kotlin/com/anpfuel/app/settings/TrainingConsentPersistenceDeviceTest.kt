package com.anpfuel.app.settings

import android.content.Context
import android.content.ContextWrapper
import android.content.SharedPreferences
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.preferences.TrainingConsentStateStore
import com.anpfuel.domain.model.ContributionScope
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.UUID

/** Real committed SharedPreferences, isolated to this test's owned filename. */
@RunWith(AndroidJUnit4::class)
class TrainingConsentPersistenceDeviceTest {
    @Test fun originalOwnerOffIsOnDiskAndAnotherAccountCannotConsumeIt() {
        val context=ApplicationProvider.getApplicationContext<Context>()
        val name="consent-owned-test-"+UUID.randomUUID()
        val scoped=object:ContextWrapper(context) {
            override fun getSharedPreferences(requested:String,mode:Int):SharedPreferences =
                context.getSharedPreferences(name,mode)
        }
        val a=ContributionScope("account:synthetic-a:key","https://test.invalid")
        try {
            val first=TrainingConsentStateStore(scoped)
            val off=first.update(a) {it.revoke("synthetic-off").bindRevocation("synthetic-off",100)}
            val file=File(context.applicationInfo.dataDir,"shared_prefs/$name.xml")
            assertTrue(file.isFile)
            assertTrue(file.readText().contains("synthetic-off"))
            assertFalse(file.readText().contains("account:synthetic-a:key"))
            val reopened=TrainingConsentStateStore(scoped)
            assertEquals(off,reopened.load(a))
            assertNull(reopened.load(a.copy(ownerScope="account:synthetic-b:key")).pending)
            assertFalse(reopened.load(a).granted)
            assertEquals(101L,reopened.load(a).pending!!.revision)
        } finally {context.deleteSharedPreferences(name)}
    }
}
