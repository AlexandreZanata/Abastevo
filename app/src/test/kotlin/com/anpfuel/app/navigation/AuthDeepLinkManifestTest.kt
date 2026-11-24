package com.anpfuel.app.navigation

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Test
import java.nio.file.Path
import kotlin.io.path.readText

class AuthDeepLinkManifestTest {
    @Test
    fun manifestRetiresExternalAuthCallbackAndPreservesMapLinks() {
        val manifest = readProjectFile("src/main/AndroidManifest.xml")
        assertFalse(manifest.contains("android:host=\"auth\""))
        assertFalse(manifest.contains("android:pathPrefix=\"/callback\""))
        assertTrue(manifest.contains("android:scheme=\"anpfuel\""))
        assertTrue(manifest.contains("android:host=\"map\""))
        assertTrue(manifest.contains("android.intent.category.BROWSABLE"))
    }

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        return moduleRoot.resolve(relativePath).readText()
    }
}
