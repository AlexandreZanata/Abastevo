package com.anpfuel.app.ui.enterprise

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * EMP-GPS-010 (B-BR-EMP-544, EDR-354): every organization tracking string exists in the eight locales with the same
 * placeholders, and the process-stopped cause no longer claims that the app was closed (it also names a collection the
 * system stopped or never started).
 */
class EnterpriseTrackingStringsTest {
    private val locales = listOf("values", "values-pt-rBR", "values-es", "values-fr", "values-de", "values-ja", "values-ru", "values-zh-rCN")
    private val files = listOf("enterprise_tracking.xml", "enterprise_tracking_interruptions.xml")

    private fun strings(locale: String, file: String): Map<String, String> {
        val document = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File("src/main/res/$locale/$file"))
        val nodes = document.getElementsByTagName("string")
        return (0 until nodes.length).associate { i -> nodes.item(i).attributes.getNamedItem("name").nodeValue to nodes.item(i).textContent }
    }
    private fun placeholders(text: String) = Regex("%\\d+\\$[sd]").findAll(text).map { it.value }.sorted().toList()

    @Test fun `every tracking string exists in all eight locales with the same placeholders`() {
        for (file in files) {
            val base = strings("values", file)
            assertTrue(base.isNotEmpty(), file)
            for (locale in locales) {
                val translated = strings(locale, file)
                assertEquals(base.keys, translated.keys, "$locale/$file")
                for ((key, text) in base) {
                    assertTrue(translated.getValue(key).isNotBlank(), "$locale/$key")
                    assertEquals(placeholders(text), placeholders(translated.getValue(key)), "$locale/$key")
                }
            }
        }
    }

    @Test fun `missing gap - the interruption lines carry state, start, end and cause and the stop cause makes no false claim`() {
        val base = strings("values", "enterprise_tracking_interruptions.xml")
        assertEquals(listOf("%1\$s", "%2\$s", "%3\$s"), placeholders(base.getValue("enterprise_tracking_interruption_open")))
        assertEquals(listOf("%1\$s", "%2\$s", "%3\$s", "%4\$s"), placeholders(base.getValue("enterprise_tracking_interruption_closed")))
        val stopped = strings("values-pt-rBR", "enterprise_tracking.xml").getValue("enterprise_tracking_cause_process_stopped")
        assertFalse(stopped.contains("O app foi fechado durante"), "the cause also names a collection the system stopped")
        assertTrue(stopped.contains("sem o seu comando"))
    }
}
