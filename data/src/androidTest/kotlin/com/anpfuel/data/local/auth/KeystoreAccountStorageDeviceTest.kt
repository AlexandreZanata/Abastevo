package com.anpfuel.data.local.auth

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.portable.AuthFlow
import com.anpfuel.domain.portable.PortableAuth
import java.security.KeyStore
import java.util.UUID
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Real AndroidKeyStore regression: software AES keys do not reject caller IVs. */
@RunWith(AndroidJUnit4::class)
class KeystoreAccountStorageDeviceTest {
    @Test
    fun randomizedNonExportableKeysPersistAcrossStoreRecreation() {
        val scope = "auth-regression-" + UUID.randomUUID()
        val context = ApplicationProvider.getApplicationContext<Context>()
        val prefs = context.getSharedPreferences(scope, Context.MODE_PRIVATE)
        val keys = AndroidKeystoreKeys()
        val keyAlias = "$scope-key"
        val sessionAlias = "$scope-session"
        val backup = AuthFlow.KeyBackup("testuser", "synthetic-account-key")
        val session = PortableAuth.Session("family-test", "account-test", "access-test", "refresh-test", 900L, 1800L)
        try {
            assertNull(keys.getOrCreateKey(keyAlias)!!.encoded)
            KeystoreAccountKey(prefs, keys, keyAlias).saveKey(backup)
            KeystoreSessionStore(prefs, keys, sessionAlias).save(session)
            // A fresh preferences handle and fresh adapters model process restart.
            val reopened = context.getSharedPreferences(scope, Context.MODE_PRIVATE)
            val recreatedKeys = AndroidKeystoreKeys()
            assertEquals(backup, KeystoreAccountKey(reopened, recreatedKeys, keyAlias).loadKey())
            assertEquals(session, KeystoreSessionStore(reopened, recreatedKeys, sessionAlias).load())
            val first = reopened.getString(KeystoreAccountKey.PREF_KEY, null)
            KeystoreAccountKey(reopened, recreatedKeys, keyAlias).saveKey(backup)
            assertNotEquals(first, reopened.getString(KeystoreAccountKey.PREF_KEY, null))
            KeystoreAccountKey(reopened, recreatedKeys, keyAlias).clearKey()
            KeystoreSessionStore(reopened, recreatedKeys, sessionAlias).clear()
            assertNull(KeystoreAccountKey(reopened, recreatedKeys, keyAlias).loadKey())
            assertNull(KeystoreSessionStore(reopened, recreatedKeys, sessionAlias).load())
        } finally {
            prefs.edit().clear().commit()
            KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.also {
                it.deleteEntry(keyAlias)
                it.deleteEntry(sessionAlias)
            }
        }
    }
}
