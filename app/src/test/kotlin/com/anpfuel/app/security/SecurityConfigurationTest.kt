package com.anpfuel.app.security

import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.nio.file.Path
import kotlin.io.path.readText

class SecurityConfigurationTest {

    @Test
    fun manifestDisablesCleartextTraffic() {
        val manifest = readProjectFile("src/main/AndroidManifest.xml")

        assertTrue(
            manifest.contains("android:usesCleartextTraffic=\"false\""),
            "AndroidManifest must set usesCleartextTraffic to false",
        )
        assertTrue(
            manifest.contains("android:networkSecurityConfig=\"@xml/network_security_config\""),
            "AndroidManifest must reference network_security_config",
        )
    }

    @Test
    fun networkSecurityConfigBlocksCleartext() {
        val config = readProjectFile("src/main/res/xml/network_security_config.xml")

        assertTrue(
            config.contains("cleartextTrafficPermitted=\"false\""),
            "network_security_config must disallow cleartext traffic",
        )
    }

    @Test
    fun releaseProguardRulesExist() {
        val rules = readProjectFile("proguard-rules.pro")

        assertTrue(rules.contains("-keep class dagger.hilt.**"), "Hilt keep rules required")
        assertTrue(rules.contains("-keep @androidx.room.Entity"), "Room keep rules required")
    }

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        return moduleRoot.resolve(relativePath).readText()
    }
}
