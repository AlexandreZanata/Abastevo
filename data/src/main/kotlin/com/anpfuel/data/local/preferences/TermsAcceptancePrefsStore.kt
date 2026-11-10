package com.anpfuel.data.local.preferences

import android.content.Context
import com.anpfuel.application.port.TermsAcceptanceStore
import com.anpfuel.domain.legal.TermsAcceptance
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import org.json.JSONObject

/**
 * Device-local first-use acceptance (B-BR-TA02): two document versions and the
 * instant, nothing about the person. Anything unreadable loads as no record
 * (B-BR-TA06). The value is never logged.
 */
@Singleton
class TermsAcceptancePrefsStore internal constructor(
    private val readValue: () -> String?,
    private val writeValue: (String?) -> Boolean,
) : TermsAcceptanceStore {
    @Inject constructor(@ApplicationContext context: Context) : this(
        readValue = { context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).getString(KEY_RECORD, null) },
        writeValue = { value ->
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).edit().apply {
                if (value == null) remove(KEY_RECORD) else putString(KEY_RECORD, value)
            }.commit()
        },
    )

    @Synchronized
    override fun load(): TermsAcceptance? = runCatching {
        val raw = readValue() ?: return null
        if (raw.length > MAX_LENGTH) return null
        val json = JSONObject(raw)
        if (json.length() != 3) return null
        val terms = json.get("terms_version") as? String ?: return null
        val privacy = json.get("privacy_version") as? String ?: return null
        val acceptedAt = json.get("accepted_at").let { it as? Long ?: (it as? Int)?.toLong() } ?: return null
        if (!version(terms) || !version(privacy) || acceptedAt <= 0) return null
        TermsAcceptance(terms, privacy, acceptedAt)
    }.getOrNull()

    @Synchronized
    override fun save(record: TermsAcceptance): Boolean {
        if (!version(record.termsVersion) || !version(record.privacyVersion) || record.acceptedAtMillis <= 0) return false
        val raw = JSONObject()
            .put("terms_version", record.termsVersion)
            .put("privacy_version", record.privacyVersion)
            .put("accepted_at", record.acceptedAtMillis)
            .toString()
        return runCatching { writeValue(raw) }.getOrDefault(false) && load() == record
    }

    @Synchronized
    override fun clear(): Boolean =
        runCatching { writeValue(null) }.getOrDefault(false) && runCatching { readValue() }.getOrNull() == null

    private fun version(value: String) = value.length in 1..32 && value.all { it in '!'..'~' }

    companion object {
        const val PREFS_NAME = "terms_acceptance"
        const val KEY_RECORD = "record"
        private const val MAX_LENGTH = 512
    }
}
