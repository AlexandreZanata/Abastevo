package com.anpfuel.data.local.preferences

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.application.enterprise.EnterpriseMembershipHints
import dagger.hilt.android.qualifiers.ApplicationContext
import java.security.MessageDigest
import javax.inject.Inject
import javax.inject.Singleton

/**
 * B-BR-EMP-ENTRY-01 device hint: one boolean per signed account, keyed by a
 * SHA-256 of origin and account id, so neither the account nor any company
 * data is stored. It only opens or closes the Perfil entry.
 */
@Singleton
class EnterpriseMembershipHintStore @Inject constructor(
    @ApplicationContext context: Context,
) : EnterpriseMembershipHints {

    private val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    override fun linked(accountId: String, origin: String): Boolean? {
        val key = key(accountId, origin)
        return if (prefs.contains(key)) prefs.getBoolean(key, false) else null
    }

    override fun record(accountId: String, origin: String, linked: Boolean) {
        prefs.edit().putBoolean(key(accountId, origin), linked).apply()
    }

    private fun key(accountId: String, origin: String): String =
        MessageDigest.getInstance("SHA-256").digest("$origin|$accountId".toByteArray())
            .joinToString("") { "%02x".format(it) }

    companion object {
        const val PREFS_NAME = "enterprise_membership_hint"
    }
}
