package com.anpfuel.app.backup

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.w3c.dom.Element

/**
 * SEC05-D backup assessment pins (AUD privacyAssessment): private Room
 * metadata and auth preferences stay out of OS backup (Auto Backup
 * rules) and out of cloud backup plus device transfer (extraction
 * rules). Sealed photos live under cacheDir (never backed up by the
 * platform) and need no rule. Public offline packs re-download after
 * a restore; the app boots fresh logged-out without a database.
 */
class BackupRulesTest {

    private fun res(name: String): Element {
        val file = File("src/main/res/xml/$name")
        require(file.isFile) { "missing res xml: ${file.absolutePath}" }
        return DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).documentElement
    }

    private fun sectionExcludes(root: Element, section: String, domain: String): Boolean {
        val sections = root.getElementsByTagName(section)
        for (i in 0 until sections.length) {
            val excludes = (sections.item(i) as Element).getElementsByTagName("exclude")
            for (j in 0 until excludes.length) {
                val exclude = excludes.item(j) as Element
                if (exclude.getAttribute("domain") == domain && exclude.getAttribute("path") == ".") {
                    return true
                }
            }
        }
        return false
    }

    private fun excludesAnywhere(root: Element, domain: String): Boolean {
        val nodes = root.getElementsByTagName("exclude")
        for (i in 0 until nodes.length) {
            val exclude = nodes.item(i) as Element
            if (exclude.getAttribute("domain") == domain && exclude.getAttribute("path") == ".") {
                return true
            }
        }
        return false
    }

    @Test
    fun autoBackupExcludesPrefsAndDatabase() {
        val root = res("backup_rules.xml")
        assertTrue(excludesAnywhere(root, "sharedpref"), "auth prefs must stay out of Auto Backup")
        assertTrue(excludesAnywhere(root, "database"), "Room metadata must stay out of Auto Backup")
    }

    @Test
    fun extractionRulesExcludePrefsAndDatabaseEverywhere() {
        val root = res("data_extraction_rules.xml")
        for (section in listOf("cloud-backup", "device-transfer")) {
            assertTrue(sectionExcludes(root, section, "sharedpref"), "$section must exclude auth prefs")
            assertTrue(sectionExcludes(root, section, "database"), "$section must exclude Room metadata")
        }
    }
}
