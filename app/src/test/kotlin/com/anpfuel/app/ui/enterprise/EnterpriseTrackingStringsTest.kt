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
 * system stopped or never started). EMP-GPS-015 (B-BR-EMP-550/551, EDR-359): the transparency copy joins the check, the only
 * remote-erasure wording in any tracking text is the negation, and the indicator names the organization and the phone.
 */
class EnterpriseTrackingStringsTest {
    private val locales = listOf("values", "values-pt-rBR", "values-es", "values-fr", "values-de", "values-ja", "values-ru", "values-zh-rCN")
    private val files = listOf("enterprise_tracking.xml", "enterprise_tracking_interruptions.xml", "enterprise_tracking_transparency.xml")

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

    /** Per locale: the one allowed negation about remote erasure, and the stems that would promise any other. */
    private val remote = mapOf(
        "values" to ("a phone without a connection is not erased remotely" to Regex("remote|wipe", RegexOption.IGNORE_CASE)),
        "values-pt-rBR" to ("um celular sem conexão não é apagado a distância" to Regex("remot|\\ba distância|limpeza", RegexOption.IGNORE_CASE)),
        "values-es" to ("un teléfono sin conexión no se borra a distancia" to Regex("remot|\\ba distancia", RegexOption.IGNORE_CASE)),
        "values-fr" to ("un téléphone sans connexion n’est pas effacé à distance" to Regex("à distance|distant", RegexOption.IGNORE_CASE)),
        "values-de" to ("ein Telefon ohne Verbindung wird nicht aus der Ferne gelöscht" to Regex("Ferne|Fernlösch|remote", RegexOption.IGNORE_CASE)),
        "values-ja" to ("接続していないスマートフォンが遠隔で削除されることはなく" to Regex("遠隔|リモート")),
        "values-ru" to ("телефон без подключения не очищается удалённо" to Regex("удалённо|дистанционно", RegexOption.IGNORE_CASE)),
        "values-zh-rCN" to ("未联网的手机不会被远程删除" to Regex("远程|遥控")),
    )

    @Test fun `false remote-wipe promise - the only remote erasure wording is the negation, in every locale`() {
        for (locale in locales) {
            val (negation, stems) = remote.getValue(locale)
            val device = strings(locale, "enterprise_tracking_transparency.xml").getValue("enterprise_tracking_mine_device")
            assertTrue(device.contains(negation), "$locale: the device text states that no remote erasure happens")
            for (file in files) for ((key, text) in strings(locale, file)) {
                val rest = text.replace(negation, "")
                assertTrue(stems.find(rest) == null, "$locale/$key promises a remote erasure: ${stems.find(rest)?.value}")
            }
        }
    }

    @Test fun `personal session displayed as corporate - the indicator names the organization and the phone, never the vehicle`() {
        val phone = mapOf("values" to "this phone", "values-pt-rBR" to "deste celular", "values-es" to "de este teléfono",
            "values-fr" to "de ce téléphone", "values-de" to "dieses Telefons", "values-ja" to "このスマートフォン", "values-ru" to "этого телефона",
            "values-zh-rCN" to "此手机")
        for (locale in locales) {
            val strings = strings(locale, "enterprise_tracking_transparency.xml")
            assertTrue(strings.getValue("enterprise_tracking_indicator_active").contains("%1\$s"), "$locale: the organization is named")
            assertTrue(strings.getValue("enterprise_tracking_indicator_detail").contains(phone.getValue(locale)), "$locale: the phone is named")
        }
        // The personal recorder's own notification text is not the organization's.
        val personal = strings("values-pt-rBR", "map25.xml").filterKeys { it.startsWith("map25_recording") }
        assertTrue(personal.isNotEmpty())
        val organization = strings("values-pt-rBR", "enterprise_tracking.xml").getValue("enterprise_tracking_notification_title")
        assertTrue(personal.values.none { it == organization })
    }
}
