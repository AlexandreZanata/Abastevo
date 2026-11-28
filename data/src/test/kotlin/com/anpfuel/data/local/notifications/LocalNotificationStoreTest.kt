package com.anpfuel.data.local.notifications

import android.content.SharedPreferences
import com.anpfuel.domain.notification.AppNotification
import com.anpfuel.domain.notification.AppNotificationContent
import com.anpfuel.domain.notification.AppNotificationSource
import com.anpfuel.domain.notification.FuelDrop
import com.anpfuel.domain.notification.NotificationFeedRule
import com.anpfuel.domain.valueobject.FuelProduct
import kotlinx.coroutines.test.runTest
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-NOTIF-03 decoding and B-BR-NOTIF-01/04/05 device storage. */
class LocalNotificationStoreTest {

    private class MemoryPrefs : SharedPreferences {
        val map = mutableMapOf<String, String?>()
        override fun getAll(): MutableMap<String, *> = map
        override fun getString(key: String, defValue: String?): String? = map[key] ?: defValue
        override fun getStringSet(key: String, defValues: MutableSet<String>?): MutableSet<String>? = defValues
        override fun getInt(key: String, defValue: Int) = defValue
        override fun getLong(key: String, defValue: Long) = defValue
        override fun getFloat(key: String, defValue: Float) = defValue
        override fun getBoolean(key: String, defValue: Boolean) = defValue
        override fun contains(key: String) = key in map
        override fun edit(): SharedPreferences.Editor = Editor(map)
        override fun registerOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) = Unit
        override fun unregisterOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) = Unit
    }

    private class Editor(private val map: MutableMap<String, String?>) : SharedPreferences.Editor {
        private val pending = mutableMapOf<String, String?>()
        private val removed = mutableSetOf<String>()
        override fun putString(key: String, value: String?) = apply { pending[key] = value }
        override fun putStringSet(key: String, values: MutableSet<String>?) = this
        override fun putInt(key: String, value: Int) = this
        override fun putLong(key: String, value: Long) = this
        override fun putFloat(key: String, value: Float) = this
        override fun putBoolean(key: String, value: Boolean) = this
        override fun remove(key: String) = apply { removed += key }
        override fun clear() = apply { removed += map.keys }
        override fun commit(): Boolean { apply(); return true }
        override fun apply() { removed.forEach(map::remove); map.putAll(pending) }
    }

    private val now = 10L * 24 * 60 * 60 * 1000
    private val prefs = MemoryPrefs()
    private val store = LocalNotificationStore(prefs) { now }

    private fun local(id: String, at: Long = now, content: AppNotificationContent = AppNotificationContent.NewSurveyWeek("2026-11-22", "2026-11-28")) =
        AppNotification(id, AppNotificationSource.LOCAL, content, at, read = false)

    @Test
    fun `every local kind survives a round trip`() = runTest {
        val kinds = listOf(
            AppNotificationContent.CityAverageDrop("SORRISO", "MT", listOf(FuelDrop(FuelProduct.GASOLINE_REGULAR, 6_470, 120))),
            AppNotificationContent.VehiclePriceDrop("Gol", "R$ 6,47"),
            AppNotificationContent.NewSurveyWeek("2026-11-22", "2026-11-28"),
            AppNotificationContent.ForumReply("7b0e2a54-0000-4000-8000-000000000001", "Topic", "Ana"),
            AppNotificationContent.ContributionDecided("Posto Vaca", FuelProduct.ETHANOL, validated = false),
        )
        kinds.forEachIndexed { index, content -> store.add(local("n$index", now - index, content)) }

        assertEquals(kinds, store.list().sortedBy { it.id }.map { it.content })
    }

    @Test
    fun `adding the same id replaces it and the inbox stays bounded`() = runTest {
        store.add(local("same"))
        store.add(local("same"))
        repeat(NotificationFeedRule.LOCAL_MAX_ITEMS + 5) { store.add(local("n$it", now - it - 1)) }

        val items = store.list()
        assertEquals(NotificationFeedRule.LOCAL_MAX_ITEMS, items.size)
        assertEquals(1, items.count { it.id == "same" })
    }

    @Test
    fun `marks read by id and all`() = runTest {
        store.add(local("a"))
        store.add(local("b", now - 1))
        store.markRead(setOf("a"))
        assertEquals(setOf("a"), store.list().filter { it.read }.map { it.id }.toSet())
        store.markAllRead()
        assertTrue(store.list().all { it.read })
    }

    @Test
    fun `account cache and shown ids are separate and cleared together`() = runTest {
        store.save(listOf(local("acc")))
        store.remember(listOf("x", "y"))
        assertEquals(setOf("x", "y"), store.shownIds())
        assertEquals(listOf("acc"), store.load().map { it.id })

        store.clear()
        assertTrue(store.load().isEmpty())
        assertTrue(store.shownIds().isEmpty())
    }

    @Test
    fun `shown ids keep only the most recent ones`() = runTest {
        store.remember((1..NotificationFeedRule.SHOWN_IDS_MAX + 10).map { "id$it" })
        val ids = store.shownIds()
        assertEquals(NotificationFeedRule.SHOWN_IDS_MAX, ids.size)
        assertTrue("id${NotificationFeedRule.SHOWN_IDS_MAX + 10}" in ids)
    }

    @Test
    fun `corrupt storage reads as empty`() = runTest {
        prefs.map["local_inbox"] = "{not json"
        assertTrue(store.list().isEmpty())
    }

    @Test
    fun `decodes the server kinds and skips unknown or malformed items`() {
        fun item(kind: String, payload: JSONObject, id: String = "7b0e2a54-0000-4000-8000-0000000000aa") = JSONObject()
            .put("notification_id", id).put("kind", kind).put("created_at", "2026-11-28T12:00:00Z").put("read", false).put("payload", payload)

        val reply = NotificationJsonCodec.decodeServerItem(item("forum.reply", JSONObject()
            .put("topic_id", "7b0e2a54-0000-4000-8000-000000000001").put("topic_title", "Preço").put("comment_id", "7b0e2a54-0000-4000-8000-000000000002").put("author_name", "Ana")))!!
        assertEquals(AppNotificationContent.ForumReply("7b0e2a54-0000-4000-8000-000000000001", "Preço", "Ana"), reply.content)
        assertEquals(AppNotificationSource.ACCOUNT, reply.source)

        val rejected = NotificationJsonCodec.decodeServerItem(item("contribution.rejected", JSONObject()
            .put("observation_id", "7b0e2a54-0000-4000-8000-000000000003").put("station_id", "7b0e2a54-0000-4000-8000-000000000004")
            .put("station_name", "Posto Vaca").put("fuel", "ETHANOL").put("reason", "photo_unreadable")))!!
        assertEquals(false, (rejected.content as AppNotificationContent.ContributionDecided).validated)

        assertNull(NotificationJsonCodec.decodeServerItem(item("promotion.new", JSONObject())))
        assertNull(NotificationJsonCodec.decodeServerItem(item("forum.reply", JSONObject().put("topic_id", "not-a-uuid"))))
        assertNull(NotificationJsonCodec.decodeServerItem(item("forum.reply", JSONObject(), id = "bad")))
    }
}
