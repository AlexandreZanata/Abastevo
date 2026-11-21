package com.anpfuel.app.ui.enterprise

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Ensures all F6 field management strings exist across all eight locales with matching placeholders. */
class EnterpriseManagementStringsTest {
    private val locales = listOf("values", "values-pt-rBR", "values-es", "values-fr", "values-de", "values-ja", "values-ru", "values-zh-rCN")
    private val file = "enterprise_management_f6.xml"

    private fun strings(locale: String): Map<String, String> {
        val document = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File("src/main/res/$locale/$file"))
        val nodes = document.getElementsByTagName("string")
        return (0 until nodes.length).associate { i ->
            nodes.item(i).attributes.getNamedItem("name").nodeValue to nodes.item(i).textContent
        }
    }

    private fun placeholders(text: String) = Regex("%\\d+\\$[sd]").findAll(text).map { it.value }.sorted().toList()

    @Test
    fun `every management string exists in all eight locales with identical placeholders`() {
        val base = strings("values")
        assertTrue(base.isNotEmpty())
        for (locale in locales) {
            val localized = strings(locale)
            assertEquals(base.keys, localized.keys, "$locale/$file")
            for ((key, text) in base) {
                assertTrue(localized.getValue(key).isNotBlank(), "$locale/$key")
                assertEquals(placeholders(text), placeholders(localized.getValue(key)), "$locale/$key")
            }
        }
    }
}
