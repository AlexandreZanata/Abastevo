package com.anpfuel.app.ui.safety

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Every community safety string exists in all eight locales with the same placeholders. */
class CommunitySafetyStringsTest {

    private val localeDirs = listOf(
        "values",
        "values-pt-rBR",
        "values-es",
        "values-de",
        "values-fr",
        "values-ru",
        "values-ja",
        "values-zh-rCN",
    )

    private val requiredKeys = setOf(
        "safety_more_actions",
        "safety_report_topic",
        "safety_report_comment",
        "safety_report_user",
        "safety_block_user",
        "safety_report_user_title",
        "safety_report_user_prompt",
        "safety_reason_spam",
        "safety_reason_harassment",
        "safety_reason_hate",
        "safety_reason_impersonation",
        "safety_reason_illegal",
        "safety_reason_other",
        "safety_report_detail_hint",
        "safety_report_detail_counter",
        "safety_report_send",
        "safety_report_user_received",
        "safety_report_user_replayed",
        "safety_block_title",
        "safety_block_message",
        "safety_block_confirm",
        "safety_block_done",
        "safety_error_self_target",
        "safety_error_block_limit",
        "safety_error_rate_limited",
        "safety_error_offline",
        "safety_error_not_found",
        "safety_error_generic",
        "safety_topic_hidden_title",
        "safety_topic_hidden_message",
        "blocked_users_title",
        "blocked_users_entry_subtitle",
        "blocked_users_intro",
        "blocked_users_empty_title",
        "blocked_users_empty_message",
        "blocked_users_fallback_forum",
        "blocked_users_fallback_station",
        "blocked_users_fallback_generic",
        "blocked_users_since",
        "blocked_users_unblock",
        "blocked_users_unblock_title",
        "blocked_users_unblock_message",
        "blocked_users_unblocked",
        "blocked_users_load_error",
        "blocked_users_sign_in",
    )

    private val placeholder = Regex("%\\d+\\$[sd]")

    private fun strings(dir: String): Map<String, String> {
        val file = File("src/main/res/$dir/strings_safety.xml")
        assertTrue(file.exists(), "strings_safety.xml must exist for $dir")
        val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("string")
        return (0 until nodes.length).associate { i ->
            val node = nodes.item(i)
            node.attributes.getNamedItem("name").nodeValue to node.textContent
        }
    }

    @Test
    fun `all safety strings exist in every supported locale`() {
        for (dir in localeDirs) {
            assertEquals(requiredKeys, strings(dir).keys, "key set of $dir")
        }
    }

    @Test
    fun `placeholders match the default locale`() {
        val default = strings("values")
        for (dir in localeDirs) {
            val translated = strings(dir)
            for ((key, text) in default) {
                assertEquals(
                    placeholder.findAll(text).map { it.value }.sorted().toList(),
                    placeholder.findAll(translated.getValue(key)).map { it.value }.sorted().toList(),
                    "placeholders of $key in $dir",
                )
                assertTrue(translated.getValue(key).isNotBlank(), "$key is blank in $dir")
            }
        }
    }
}
