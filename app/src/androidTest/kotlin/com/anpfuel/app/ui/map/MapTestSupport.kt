package com.anpfuel.app.ui.map

import androidx.test.core.app.ApplicationProvider
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth

/**
 * MapScreen's production default is a Hilt ViewModel, which a plain `createComposeRule` host cannot
 * provide. Tests that render the shell without the Hilt activity pass this explicit, session-less one.
 */
internal fun testGuidance(): GuidanceViewModel = GuidanceViewModel(
    ApplicationProvider.getApplicationContext(),
    object : AuthSessionStore {
        override fun save(session: PortableAuth.Session) = Unit
        override fun load(): PortableAuth.Session? = null
        override fun clear() = Unit
    },
)
