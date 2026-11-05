package com.anpfuel.app.ui.forum

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumStringsTest {

    private val requiredKeys = listOf(
        "forum_card_title",
        "forum_card_description",
        "forum_card_cta",
        "forum_screen_title",
        "forum_detail_title",
        "forum_category_all",
        "forum_kind_editorial",
        "forum_kind_news",
        "forum_kind_topic",
        "forum_kind_bulletin",
        "forum_feed_empty_title",
        "forum_feed_empty_hint",
        "forum_load_more",
        "forum_retry",
        "forum_offline_notice",
        "forum_unavailable_title",
        "forum_unavailable_hint",
        "forum_post_not_found_title",
        "forum_post_not_found_hint",
        "forum_read_source",
        "forum_external_link_dialog_title",
        "forum_external_link_dialog_message",
        "forum_external_link_dialog_confirm",
        "forum_external_link_dialog_cancel",
        "forum_external_link_invalid",
        "forum_updated_label",
    )

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

    @Test
    fun `all forum strings are defined in all supported locales`() {
        val resDir = File("src/main/res")
        val factory = DocumentBuilderFactory.newInstance()
        val builder = factory.newDocumentBuilder()

        for (dirName in localeDirs) {
            val stringsFile = File(resDir, "$dirName/strings.xml")
            assertTrue(stringsFile.exists(), "strings.xml must exist for locale $dirName")

            val doc = builder.parse(stringsFile)
            val stringNodes = doc.getElementsByTagName("string")
            val foundKeys = mutableSetOf<String>()

            for (i in 0 until stringNodes.length) {
                val node = stringNodes.item(i)
                val nameAttr = node.attributes.getNamedItem("name")?.nodeValue
                if (nameAttr != null) {
                    foundKeys.add(nameAttr)
                }
            }

            for (key in requiredKeys) {
                assertTrue(
                    foundKeys.contains(key),
                    "Key '$key' missing in $dirName/strings.xml",
                )
            }
        }
    }
}
