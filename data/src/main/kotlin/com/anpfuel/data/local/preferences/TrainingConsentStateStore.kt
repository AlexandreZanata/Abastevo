package com.anpfuel.data.local.preferences

import android.content.Context
import com.anpfuel.application.port.TrainingConsentLocalStore
import com.anpfuel.domain.model.ContributionScope
import com.anpfuel.domain.repository.TrainingConsentCommand
import com.anpfuel.domain.repository.TrainingConsentException
import com.anpfuel.domain.repository.TrainingConsentLocalState
import com.anpfuel.domain.repository.TrainingConsentRejectKind
import dagger.hilt.android.qualifiers.ApplicationContext
import java.security.MessageDigest
import javax.inject.Inject
import javax.inject.Singleton
import org.json.JSONObject

/** Legacy global preferences stay quarantined: never grant or dispatch them as a new owner. */
@Singleton
class TrainingConsentStateStore internal constructor(
    private val readFile: () -> String?,
    private val writeFile: (String) -> Boolean,
) : TrainingConsentLocalStore {
    @Inject constructor(@ApplicationContext context: Context) : this(
        readFile = { context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).getString(KEY_JOURNAL, null) },
        writeFile = { context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).edit().putString(KEY_JOURNAL, it).commit() },
    )

    private var poisoned = false

    @Synchronized
    override fun load(scope: ContributionScope): TrainingConsentLocalState = guarded {
        state(document(), scopeKey(scope))
    }

    @Synchronized
    override fun update(scope: ContributionScope, change: (TrainingConsentLocalState) -> TrainingConsentLocalState): TrainingConsentLocalState = guarded {
        val document = document()
        val key = scopeKey(scope)
        if (!document.has(key) && document.length() >= MAX_SCOPES) fail()
        val next = change(state(document, key))
        document.put(key, encode(next))
        val raw = document.toString()
        if (raw.toByteArray(Charsets.UTF_8).size > MAX_BYTES) fail()
        if (!writeFile(raw)) {
            // SharedPreferences may update RAM even when disk commit fails.
            // Refuse this instance thereafter; never acknowledge durable OFF/ON.
            poisoned = true
            fail()
        }
        next
    }

    private fun document(): JSONObject {
        if (poisoned) fail()
        val raw = readFile() ?: return JSONObject()
        if (raw.toByteArray(Charsets.UTF_8).size > MAX_BYTES) fail()
        return JSONObject(raw).also { document ->
            if (document.length() > MAX_SCOPES) fail()
            document.keys().forEach { if (!it.matches(Regex("[a-f0-9]{64}"))) fail() }
        }
    }

    private fun state(document: JSONObject, key: String): TrainingConsentLocalState {
        if (!document.has(key)) return TrainingConsentLocalState()
        return decode(document.getJSONObject(key))
    }

    private fun number(doc: JSONObject, name: String): Long {
        val value = doc.get(name)
        if (value !is Int && value !is Long) fail()
        return (value as Number).toLong()
    }

    private fun boolean(doc: JSONObject, name: String): Boolean = (doc.get(name) as? Boolean) ?: fail()

    private fun decode(doc: JSONObject): TrainingConsentLocalState {
        if (!doc.has("pending") || (!doc.isNull("pending") && doc.get("pending") !is JSONObject)) fail()
        val pending = doc.optJSONObject("pending")?.let {
            if (!it.has("revision")) fail()
            TrainingConsentCommand(it.getString("id"), boolean(it, "grant"),
                if (it.isNull("revision")) null else number(it, "revision"),
                it.getString("terms_version"), it.getString("terms_digest"))
        }
        return TrainingConsentLocalState(number(doc, "revision_floor"), boolean(doc, "granted"), pending,
            number(doc, "retry_at"), number(doc, "attempts").also { if (it !in 0..16) fail() }.toInt())
    }

    private fun encode(state: TrainingConsentLocalState): JSONObject = JSONObject()
        .put("revision_floor", state.revisionFloor).put("granted", state.granted)
        .put("retry_at", state.retryAtMillis).put("attempts", state.attempts)
        .put("pending", state.pending?.let {
            JSONObject().put("id", it.id).put("grant", it.grant).put("revision", it.revision ?: JSONObject.NULL)
                .put("terms_version", it.termsVersion).put("terms_digest", it.termsDigest)
        } ?: JSONObject.NULL)

    private fun scopeKey(scope: ContributionScope): String = MessageDigest.getInstance("SHA-256")
        .digest((scope.origin + "\n" + scope.ownerScope).toByteArray(Charsets.UTF_8))
        .joinToString("") { "%02x".format(it) }

    private fun <T> guarded(block: () -> T): T = try {
        block()
    } catch (e: TrainingConsentException) {
        throw e
    } catch (_: Exception) {
        fail()
    }

    private fun fail(): Nothing = throw TrainingConsentException(TrainingConsentRejectKind.TRANSPORT, "privacy storage unavailable")

    companion object {
        const val PREFS_NAME = "training_consent_owned_v2"
        const val KEY_JOURNAL = "journal"
        const val MAX_SCOPES = 32
        const val MAX_BYTES = 65536
    }
}
