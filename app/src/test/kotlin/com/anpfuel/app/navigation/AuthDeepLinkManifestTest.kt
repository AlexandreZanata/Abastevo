package com.anpfuel.app.navigation

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Path
import kotlin.io.path.readText

/**
 * P13-T05C deep-link surface: the provider callback must resolve to
 * MainActivity without user choice fan-out. Manifest XML is asserted
 * as text (SecurityConfigurationTest precedent); routing behavior
 * itself is device-gated.
 */
class AuthDeepLinkManifestTest {

    @Test
    fun manifestRoutesProviderCallbackToMainActivity() {
        val manifest = readProjectFile("src/main/AndroidManifest.xml")

        assertTrue(
            manifest.contains("android:scheme=\"anpfuel\""),
            "manifest must declare the anpfuel callback scheme",
        )
        assertTrue(
            manifest.contains("android:host=\"auth\""),
            "manifest must declare the auth callback host",
        )
        assertTrue(
            manifest.contains("android:pathPrefix=\"/callback\""),
            "manifest must declare the /callback path",
        )
        assertTrue(
            manifest.contains("android.intent.category.BROWSABLE"),
            "callback intent-filter must be browsable",
        )
    }

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        return moduleRoot.resolve(relativePath).readText()
    }
}
