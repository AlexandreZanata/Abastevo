package com.anpfuel.data.local.auth

import android.content.SharedPreferences
import com.anpfuel.application.portable.AuthFlow
import java.security.SecureRandom
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

private class KeyPrefs : SharedPreferences {
    val map = mutableMapOf<String, String?>()
    var commitSuccess = true

    override fun getString(key: String, defValue: String?): String? =
        if (map.containsKey(key)) map[key] else defValue

    override fun edit(): SharedPreferences.Editor = KeyEditor(map, commitSuccess)

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

private class KeyEditor(private val map: MutableMap<String, String?>, private val commitSuccess: Boolean) : SharedPreferences.Editor {
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

private class KeyKeys : SessionKeyProvider {
    var key: SecretKey = fresh()
    var unavailable: Boolean = false

    override fun getOrCreateKey(alias: String): SecretKey? =
        if (unavailable) null else key

    companion object {
        fun fresh(): SecretKey {
            val gen = KeyGenerator.getInstance("AES")
            gen.init(256, SecureRandom())
            return gen.generateKey()
        }
    }
}

class KeystoreAccountKeyTest {

    private fun backup() = AuthFlow.KeyBackup("ana123", "key-for-ana123")

    @Test
    fun failedUpdateRestoresPreviousCredentialInMemory() {
        val prefs = KeyPrefs()
        val store = KeystoreAccountKey(prefs, KeyKeys())
        store.saveKey(backup())
        prefs.commitSuccess = false
        assertThrows(IllegalStateException::class.java) { store.saveKey(backup().copy(accountKey = "replacement-test-key")) }
        assertEquals(backup(), store.loadKey())
    }

    @Test
    fun diskWriteFailureIsReportedEvenWhenPreferencesMemoryChanges() {
        val prefs = KeyPrefs().apply { commitSuccess = false }
        val store = KeystoreAccountKey(prefs, KeyKeys())
        assertThrows(IllegalStateException::class.java) { store.saveKey(backup()) }
        assertNull(store.loadKey())
    }

    @Test
    fun savesAndLoadsRoundTrip() {
        val store = KeystoreAccountKey(KeyPrefs(), KeyKeys())
        store.saveKey(backup())
        assertEquals(backup(), store.loadKey())
    }

    @Test
    fun clearEmpties() {
        val store = KeystoreAccountKey(KeyPrefs(), KeyKeys())
        store.saveKey(backup())
        store.clearKey()
        assertNull(store.loadKey())
    }

    @Test
    fun foreignKeyRefuses() {
        val prefs = KeyPrefs()
        KeystoreAccountKey(prefs, KeyKeys()).saveKey(backup())
        assertNull(KeystoreAccountKey(prefs, KeyKeys()).loadKey())
    }

    @Test
    fun tamperedBlobRefuses() {
        val prefs = KeyPrefs()
        KeystoreAccountKey(prefs, KeyKeys()).saveKey(backup())
        prefs.map[KeystoreAccountKey.PREF_KEY] = "tampered"
        assertNull(KeystoreAccountKey(prefs, KeyKeys()).loadKey())
    }

    @Test
    fun emptyBackupNeverPersists() {
        val prefs = KeyPrefs()
        val store = KeystoreAccountKey(prefs, KeyKeys())
        store.saveKey(AuthFlow.KeyBackup("", ""))
        assertNull(store.loadKey())
        assertNull(prefs.map[KeystoreAccountKey.PREF_KEY])
    }
}
