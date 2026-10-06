package com.anpfuel.data.local.auth

import android.content.SharedPreferences
import com.anpfuel.domain.portable.PortableAuth
import java.security.SecureRandom
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

private class FakePrefs : SharedPreferences {
    val map = mutableMapOf<String, String?>()
    var commitSuccess = true

    override fun getString(key: String, defValue: String?): String? =
        if (map.containsKey(key)) map[key] else defValue

    override fun edit(): SharedPreferences.Editor = FakeEditor(map, commitSuccess)

    override fun contains(key: String): Boolean = map.containsKey(key)

    override fun getAll(): Map<String, *> = map.toMap()

    override fun getStringSet(key: String, defValues: MutableSet<String>?): MutableSet<String>? = defValues

    override fun getInt(key: String, defValue: Int): Int = defValue

    override fun getLong(key: String, defValue: Long): Long = defValue

    override fun getFloat(key: String, defValue: Float): Float = defValue

    override fun getBoolean(key: String, defValue: Boolean): Boolean = defValue

    override fun registerOnSharedPreferenceChangeListener(
        listener: SharedPreferences.OnSharedPreferenceChangeListener?,
    ) = Unit

    override fun unregisterOnSharedPreferenceChangeListener(
        listener: SharedPreferences.OnSharedPreferenceChangeListener?,
    ) = Unit
}

private class FakeEditor(private val map: MutableMap<String, String?>, private val commitSuccess: Boolean) : SharedPreferences.Editor {
    override fun putString(key: String, value: String?): SharedPreferences.Editor {
        map[key] = value
        return this
    }

    override fun remove(key: String): SharedPreferences.Editor {
        map.remove(key)
        return this
    }

    override fun clear(): SharedPreferences.Editor {
        map.clear()
        return this
    }

    override fun commit(): Boolean = commitSuccess

    override fun apply() = Unit

    override fun putStringSet(key: String, values: MutableSet<String>?): SharedPreferences.Editor = this

    override fun putInt(key: String, value: Int): SharedPreferences.Editor = this

    override fun putLong(key: String, value: Long): SharedPreferences.Editor = this

    override fun putFloat(key: String, value: Float): SharedPreferences.Editor = this

    override fun putBoolean(key: String, value: Boolean): SharedPreferences.Editor = this
}

private class FakeKeys : SessionKeyProvider {
    var key: SecretKey = freshKey()
    var unavailable: Boolean = false

    override fun getOrCreateKey(alias: String): SecretKey? =
        if (unavailable) null else key

    companion object {
        fun freshKey(): SecretKey {
            val gen = KeyGenerator.getInstance("AES")
            gen.init(256, SecureRandom())
            return gen.generateKey()
        }
    }
}

class KeystoreSessionStoreTest {

    private fun session() = PortableAuth.Session(
        familyId = "family-1", accountId = "account-1",
        accessToken = "access-1", refreshToken = "refresh-1",
        accessExpiresAt = 1_000_900L, absoluteExpiresAt = 4_259_200L,
    )

    @Test
    fun failedUpdateRestoresPreviousCredentialInMemory() {
        val prefs = FakePrefs()
        val store = KeystoreSessionStore(prefs, FakeKeys())
        store.save(session())
        prefs.commitSuccess = false
        assertThrows(IllegalStateException::class.java) { store.save(session().copy(refreshToken = "replacement-test-refresh")) }
        assertEquals(session(), store.load())
    }

    @Test
    fun diskWriteFailureIsReportedEvenWhenPreferencesMemoryChanges() {
        val prefs = FakePrefs().apply { commitSuccess = false }
        val store = KeystoreSessionStore(prefs, FakeKeys())
        assertThrows(IllegalStateException::class.java) { store.save(session()) }
        assertNull(store.load())
    }

    @Test
    fun savesAndLoadsRoundTrip() {
        val store = KeystoreSessionStore(FakePrefs(), FakeKeys())
        store.save(session())
        assertEquals(session(), store.load())
    }

    @Test
    fun clearEmpties() {
        val prefs = FakePrefs()
        val store = KeystoreSessionStore(prefs, FakeKeys())
        store.save(session())
        store.clear()
        assertNull(store.load())
    }

    @Test
    fun rotatedKeyCannotReadOldBlob() {
        val prefs = FakePrefs()
        val keys = FakeKeys()
        val writer = KeystoreSessionStore(prefs, keys)
        writer.save(session())
        keys.key = FakeKeys.freshKey()
        assertNull(KeystoreSessionStore(prefs, keys).load())
    }

    @Test
    fun corruptBlobLoadsNull() {
        val prefs = FakePrefs()
        val store = KeystoreSessionStore(prefs, FakeKeys())
        store.save(session())
        prefs.map[KeystoreSessionStore.PREF_SESSION] = "corrupted"
        assertNull(store.load())
    }

    @Test
    fun unavailableKeystoreFailsClosed() {
        val keys = FakeKeys().apply { unavailable = true }
        val store = KeystoreSessionStore(FakePrefs(), keys)
        store.save(session())
        assertNull(store.load())
    }
}
