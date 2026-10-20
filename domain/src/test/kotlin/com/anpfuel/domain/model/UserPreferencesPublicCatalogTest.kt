package com.anpfuel.domain.model

import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class UserPreferencesPublicCatalogTest {

    @Test
    fun `public catalog sync is disabled and unstamped by default`() {
        val preferences = UserPreferences()

        assertFalse(preferences.syncPublicCityCatalog)
        assertNull(preferences.publicCityCatalogLastSyncedAtMillis)
    }
}
