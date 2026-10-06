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

    @Test
    fun credentialsExcludedFromBothBackupAndDeviceTransfer() {
        val manifest = readProjectFile("src/main/AndroidManifest.xml")
        assertTrue(manifest.contains("android:fullBackupContent=\"@xml/backup_rules\""))
        assertTrue(manifest.contains("android:dataExtractionRules=\"@xml/data_extraction_rules\""))
        val legacy = readProjectFile("src/main/res/xml/backup_rules.xml")
        assertTrue(legacy.contains("<exclude domain=\"sharedpref\" path=\".\" />"))
        val modern = readProjectFile("src/main/res/xml/data_extraction_rules.xml")
        val doc = javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder()
            .parse(modern.byteInputStream())
        for (type in listOf("cloud-backup", "device-transfer")) {
            val node = doc.getElementsByTagName(type).item(0) as org.w3c.dom.Element
            val excluded = node.getElementsByTagName("exclude").item(0) as org.w3c.dom.Element
            assertTrue(excluded.getAttribute("domain") == "sharedpref" && excluded.getAttribute("path") == ".")
        }
    }

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        return moduleRoot.resolve(relativePath).readText()
    }
}
