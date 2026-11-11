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
        val legacy = parseProjectXml("src/main/res/xml/backup_rules.xml").documentElement
        val modern = parseProjectXml("src/main/res/xml/data_extraction_rules.xml")
        val channels = mapOf(
            "full-backup-content" to legacy,
            "cloud-backup" to modern.getElementsByTagName("cloud-backup").item(0) as org.w3c.dom.Element,
            "device-transfer" to modern.getElementsByTagName("device-transfer").item(0) as org.w3c.dom.Element,
        )
        // Every channel must exclude credentials, private Room metadata and the
        // sealed enterprise vault, wherever each rule sits in the section.
        val required = setOf(
            "sharedpref" to ".",
            "database" to ".",
            "root" to "no_backup/enterprise-sealed-v1/",
        )
        for ((channel, rules) in channels) {
            val nodes = rules.getElementsByTagName("exclude")
            val excluded = (0 until nodes.length).map {
                val node = nodes.item(it) as org.w3c.dom.Element
                node.getAttribute("domain") to node.getAttribute("path")
            }.toSet()
            for (rule in required) {
                assertTrue(rule in excluded, "$channel must exclude ${rule.first} ${rule.second}")
            }
        }
    }

    private fun parseProjectXml(relativePath: String): org.w3c.dom.Document =
        javax.xml.parsers.DocumentBuilderFactory.newInstance().newDocumentBuilder()
            .parse(readProjectFile(relativePath).byteInputStream())

    private fun readProjectFile(relativePath: String): String {
        val moduleRoot = Path.of(System.getProperty("user.dir"))
        return moduleRoot.resolve(relativePath).readText()
    }
}
